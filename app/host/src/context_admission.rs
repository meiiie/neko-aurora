use crate::acp::Binding;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::collections::HashMap;

const SAFE: i64 = 9_007_199_254_740_991;
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ContextError {
    Protocol,
    Manifest,
    Receipt,
    Binding,
    Bound,
}
type Result<T> = std::result::Result<T, ContextError>;

#[derive(Clone, Debug)]
enum Mode {
    Fresh {
        manifest: Value,
        pin: String,
        digest: String,
    },
    Resume {
        expected: Value,
    },
}
#[derive(Clone, Debug)]
pub struct LaunchContext {
    mode: Mode,
}
#[derive(Clone, Debug)]
pub struct AdmittedContext {
    receipt: Value,
}

impl LaunchContext {
    pub fn parse(spec: &Value) -> Result<Option<Self>> {
        if !spec.is_object() {
            return Err(ContextError::Protocol);
        }
        let fields = [
            "contextProtocol",
            "contextManifest",
            "contextManifestSHA256",
            "contextPinSHA256",
            "contextExpected",
        ];
        if fields.iter().all(|key| spec.get(key).is_none()) {
            return Ok(None);
        }
        if integer(&spec["contextProtocol"], 1, 1).is_err() {
            return Err(ContextError::Protocol);
        }
        let mode = if let Some(saved) = spec.get("resume") {
            if !saved.is_object()
                || [
                    "contextManifest",
                    "contextManifestSHA256",
                    "contextPinSHA256",
                ]
                .iter()
                .any(|key| spec.get(key).is_some())
            {
                return Err(ContextError::Protocol);
            }
            let expected = spec.get("contextExpected").ok_or(ContextError::Receipt)?;
            validate_receipt(expected)?;
            if expected["sessionId"] != saved["sessionId"]
                || expected["task"] != saved["task"]
                || expected["execution"] != saved["execution"]
            {
                return Err(ContextError::Binding);
            }
            Mode::Resume {
                expected: expected.clone(),
            }
        } else {
            if spec.get("contextExpected").is_some() {
                return Err(ContextError::Protocol);
            }
            let m = spec.get("contextManifest").ok_or(ContextError::Manifest)?;
            validate_manifest(m)?;
            let digest = manifest_digest(m)?;
            let pin = hex(&spec["contextPinSHA256"], 64)?.to_owned();
            if hex(&spec["contextManifestSHA256"], 64)? != digest || m["contextPinSHA256"] != pin {
                return Err(ContextError::Manifest);
            }
            Mode::Fresh {
                manifest: m.clone(),
                pin,
                digest,
            }
        };
        Ok(Some(Self { mode }))
    }
    pub fn session_meta(&self) -> Value {
        match &self.mode {
            Mode::Fresh { manifest, .. } => {
                json!({"neko.contextProtocol":{"version":1},"neko.contextManifest":manifest})
            }
            Mode::Resume { expected } => {
                json!({"neko.contextProtocol":{"version":1},"neko.contextExpected":expected})
            }
        }
    }
    pub fn admit(&self, admission: &Value, binding: &Binding) -> Result<AdmittedContext> {
        let r = admission["_meta"]
            .get("neko.contextAdmission")
            .ok_or(ContextError::Receipt)?;
        validate_receipt(r)?;
        receipt_binding(r, binding)?;
        match &self.mode {
            Mode::Fresh { pin, digest, .. }
                if r["contextPinSHA256"] != *pin || r["manifestSHA256"] != *digest =>
            {
                return Err(ContextError::Receipt)
            }
            Mode::Resume { expected } => {
                for key in [
                    "sessionId",
                    "contextPinSHA256",
                    "manifestSHA256",
                    "sourceIds",
                ] {
                    if r[key] != expected[key] {
                        return Err(ContextError::Receipt);
                    }
                }
                if binding.task["id"] != expected["task"]["id"]
                    || binding.task["root"] != expected["task"]["root"]
                    || binding.execution["authorityId"] != expected["execution"]["authorityId"]
                    || binding.execution["policyId"] != expected["execution"]["policyId"]
                {
                    return Err(ContextError::Binding);
                }
            }
            _ => {}
        }
        Ok(AdmittedContext { receipt: r.clone() })
    }
}
impl AdmittedContext {
    pub fn receipt(&self) -> &Value {
        &self.receipt
    }
    pub fn meta(&self, binding: &Binding) -> Result<Value> {
        receipt_binding(&self.receipt, binding)?;
        let mut meta = binding.meta();
        meta["neko.contextAdmission"] = self.receipt.clone();
        Ok(meta)
    }
    pub fn echo(&self, binding: &Binding, params: &Value) -> Result<()> {
        receipt_binding(&self.receipt, binding)?;
        binding.echo(params).map_err(|_| ContextError::Binding)?;
        let meta = params["_meta"].as_object().ok_or(ContextError::Receipt)?;
        if meta.get("neko.contextAdmission") != Some(&self.receipt)
            || meta
                .keys()
                .any(|key| key.starts_with("neko.context") && key != "neko.contextAdmission")
        {
            return Err(ContextError::Receipt);
        }
        Ok(())
    }
}
fn receipt_binding(r: &Value, b: &Binding) -> Result<()> {
    if r["sessionId"] != b.session || r["task"] != b.task || r["execution"] != b.execution {
        return Err(ContextError::Binding);
    }
    Ok(())
}
fn validate_receipt(v: &Value) -> Result<()> {
    fields(
        v,
        &[
            "version",
            "sessionId",
            "contextPinSHA256",
            "manifestSHA256",
            "sourceIds",
            "task",
            "execution",
        ],
    )?;
    integer(&v["version"], 1, 1)?;
    hex(&v["sessionId"], 32)?;
    hex(&v["contextPinSHA256"], 64)?;
    hex(&v["manifestSHA256"], 64)?;
    let ids = v["sourceIds"].as_array().ok_or(ContextError::Receipt)?;
    if ids.len() != 1 {
        return Err(ContextError::Receipt);
    }
    hex(&ids[0], 64)?;
    if !v["task"].is_object() || !v["execution"].is_object() {
        return Err(ContextError::Receipt);
    }
    hex(&v["execution"]["authorityId"], 64)?;
    hex(&v["execution"]["policyId"], 64)?;
    let root = v["task"]["root"]
        .as_str()
        .filter(|s| !s.is_empty())
        .ok_or(ContextError::Binding)?;
    Binding::admit(&json!({"sessionId":v["sessionId"],"_meta":{"neko.task":v["task"],"neko.execution":v["execution"]}}),root).map_err(|_|ContextError::Binding)?;
    canonical_bytes(v, 65536)?;
    Ok(())
}

fn validate_manifest(m: &Value) -> Result<()> {
    let version = integer(&m["version"], 1, 2)?;
    fields(
        m,
        if version == 1 {
            &[
                "version",
                "contextPinSHA256",
                "scope",
                "primaryMessage",
                "sources",
                "factCandidates",
            ]
        } else {
            &[
                "version",
                "contextPinSHA256",
                "scope",
                "primaryMessage",
                "sources",
                "factCandidates",
                "requester",
            ]
        },
    )?;
    hex(&m["contextPinSHA256"], 64)?;
    scope(&m["scope"])?;
    fields(&m["primaryMessage"], &["id", "senderId", "text"])?;
    id(&m["primaryMessage"]["id"])?;
    id(&m["primaryMessage"]["senderId"])?;
    let primary = text(&m["primaryMessage"]["text"], 32768)?;
    if primary.is_empty() {
        return Err(ContextError::Manifest);
    }
    if version == 1 {
        if m["primaryMessage"]["senderId"] != m["scope"]["ownerUserId"] {
            return Err(ContextError::Manifest);
        }
    } else {
        let r = &m["requester"];
        fields(
            r,
            &[
                "version",
                "actorId",
                "spaceId",
                "authorityId",
                "deviceId",
                "deviceRevision",
                "sessionId",
                "sessionRevision",
                "issuerEpoch",
                "expiresAtUnixMs",
            ],
        )?;
        integer(&r["version"], 1, 1)?;
        for key in ["actorId", "spaceId", "authorityId", "deviceId", "sessionId"] {
            id(&r[key])?;
        }
        for key in [
            "deviceRevision",
            "sessionRevision",
            "issuerEpoch",
            "expiresAtUnixMs",
        ] {
            integer(&r[key], 1, SAFE)?;
        }
        if r["spaceId"] != m["scope"]["spaceId"] || r["actorId"] != m["primaryMessage"]["senderId"]
        {
            return Err(ContextError::Manifest);
        }
    }
    let sources = m["sources"]
        .as_array()
        .filter(|s| s.len() <= 16)
        .ok_or(ContextError::Manifest)?;
    let candidates = m["factCandidates"]
        .as_array()
        .filter(|s| s.len() <= 64)
        .ok_or(ContextError::Manifest)?;
    let mut by_id = HashMap::new();
    for source in sources {
        fields(
            source,
            &[
                "id",
                "kind",
                "version",
                "sha256",
                "bytes",
                "scope",
                "accessRevision",
                "observedAt",
                "effectiveFrom",
                "effectiveTo",
                "entities",
                "span",
            ],
        )?;
        let source_id = id(&source["id"])?;
        if by_id.insert(source_id, source).is_some() {
            return Err(ContextError::Manifest);
        }
        let kind = source["kind"].as_str().ok_or(ContextError::Manifest)?;
        if !["original-user", "retrieved", "derived"].contains(&kind) {
            return Err(ContextError::Manifest);
        }
        scope(&source["scope"])?;
        if canonical_bytes(&source["scope"], 4096)? != canonical_bytes(&m["scope"], 4096)? {
            return Err(ContextError::Manifest);
        }
        integer(&source["version"], 1, SAFE)?;
        integer(&source["accessRevision"], 0, SAFE)?;
        let bytes = integer(&source["bytes"], 1, 1048576)? as usize;
        let digest = hex(&source["sha256"], 64)?;
        let selected = span(&source["span"])?;
        if selected.end > bytes {
            return Err(ContextError::Manifest);
        }
        if kind == "original-user"
            && (source["id"] != m["primaryMessage"]["id"]
                || bytes != primary.len()
                || digest != hash(primary.as_bytes())
                || primary.get(selected.offset..selected.end) != Some(selected.text))
        {
            return Err(ContextError::Manifest);
        }
        date(&source["observedAt"], true)?;
        date(&source["effectiveFrom"], false)?;
        date(&source["effectiveTo"], false)?;
        if let (Some(a), Some(z)) = (
            source["effectiveFrom"].as_str(),
            source["effectiveTo"].as_str(),
        ) {
            if z < a {
                return Err(ContextError::Manifest);
            }
        }
        let entities = source["entities"]
            .as_array()
            .filter(|e| e.len() <= 32)
            .ok_or(ContextError::Manifest)?;
        for entity in entities {
            fields(entity, &["id", "label"])?;
            nullable_id(&entity["id"])?;
            let label = text(&entity["label"], 256)?;
            if js_whitespace_only(label) {
                return Err(ContextError::Manifest);
            }
        }
    }
    for candidate in candidates {
        fields(candidate, &["key", "sourceId", "entityId", "state", "span"])?;
        id(&candidate["key"])?;
        nullable_id(&candidate["entityId"])?;
        let source = by_id
            .get(id(&candidate["sourceId"])?)
            .ok_or(ContextError::Manifest)?;
        let selected = span(&candidate["span"])?;
        let original = span(&source["span"])?;
        if candidate["state"] != "unresolved"
            || selected.offset < original.offset
            || selected.end > original.end
            || original
                .text
                .get(selected.offset - original.offset..selected.end - original.offset)
                != Some(selected.text)
        {
            return Err(ContextError::Manifest);
        }
        if !candidate["entityId"].is_null()
            && !source["entities"]
                .as_array()
                .unwrap()
                .iter()
                .any(|e| e["id"] == candidate["entityId"])
        {
            return Err(ContextError::Manifest);
        }
    }
    canonical_bytes(m, 49152)?;
    Ok(())
}
struct Span<'a> {
    offset: usize,
    end: usize,
    text: &'a str,
}
fn span(v: &Value) -> Result<Span<'_>> {
    fields(v, &["offset", "end", "sha256", "text"])?;
    let offset =
        usize::try_from(integer(&v["offset"], 0, SAFE)?).map_err(|_| ContextError::Manifest)?;
    let end = usize::try_from(integer(&v["end"], 0, SAFE)?).map_err(|_| ContextError::Manifest)?;
    let s = text(&v["text"], 8192)?;
    if end <= offset || end - offset != s.len() || hex(&v["sha256"], 64)? != hash(s.as_bytes()) {
        return Err(ContextError::Manifest);
    }
    Ok(Span {
        offset,
        end,
        text: s,
    })
}
fn fields(v: &Value, keys: &[&str]) -> Result<()> {
    let obj = v.as_object().ok_or(ContextError::Manifest)?;
    if obj.len() != keys.len() || keys.iter().any(|k| !obj.contains_key(*k)) {
        return Err(ContextError::Manifest);
    }
    Ok(())
}
fn text(v: &Value, max: usize) -> Result<&str> {
    v.as_str()
        .filter(|s| s.len() <= max)
        .ok_or(ContextError::Manifest)
}
fn id(v: &Value) -> Result<&str> {
    let s = text(v, 128)?;
    if s.is_empty()
        || !s.as_bytes()[0].is_ascii_alphanumeric()
        || !s
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"_.:-".contains(&b))
    {
        return Err(ContextError::Manifest);
    }
    Ok(s)
}
fn nullable_id(v: &Value) -> Result<()> {
    if !v.is_null() {
        id(v)?;
    }
    Ok(())
}
fn hex(v: &Value, size: usize) -> Result<&str> {
    v.as_str()
        .filter(|s| {
            s.len() == size
                && s.bytes()
                    .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
        })
        .ok_or(ContextError::Manifest)
}
fn number(v: &Value) -> Result<i64> {
    if let Some(n) = v.as_i64() {
        if (-SAFE..=SAFE).contains(&n) {
            return Ok(n);
        }
    }
    if let Some(n) = v.as_f64() {
        if n.is_finite() && n.fract() == 0.0 && n >= -(SAFE as f64) && n <= SAFE as f64 {
            return Ok(n as i64);
        }
    }
    Err(ContextError::Manifest)
}
fn integer(v: &Value, min: i64, max: i64) -> Result<i64> {
    let n = number(v)?;
    if n < min || n > max {
        Err(ContextError::Manifest)
    } else {
        Ok(n)
    }
}
fn scope(v: &Value) -> Result<()> {
    fields(
        v,
        &[
            "ownerUserId",
            "spaceId",
            "botId",
            "projectId",
            "channelId",
            "audienceRevision",
        ],
    )?;
    for k in ["ownerUserId", "spaceId", "botId", "channelId"] {
        id(&v[k])?;
    }
    nullable_id(&v["projectId"])?;
    integer(&v["audienceRevision"], 1, SAFE)?;
    Ok(())
}
fn js_whitespace_only(s: &str) -> bool {
    s.chars().all(|c| {
        matches!(
            c,
            '\t' | '\n' | '\r' | '\x0b' | '\x0c' | ' ' | '\u{a0}' | '\u{1680}' | '\u{2000}'
                ..='\u{200a}'
                    | '\u{2028}'
                    | '\u{2029}'
                    | '\u{202f}'
                    | '\u{205f}'
                    | '\u{3000}'
                    | '\u{feff}'
        )
    })
}
fn date(v: &Value, timestamp: bool) -> Result<()> {
    if v.is_null() {
        return Ok(());
    }
    let s = v.as_str().ok_or(ContextError::Manifest)?;
    let b = s.as_bytes();
    if b.len() != if timestamp { 24 } else { 10 } || b[4] != b'-' || b[7] != b'-' {
        return Err(ContextError::Manifest);
    }
    let digits = |a: usize, z: usize| -> Result<u32> {
        if !b[a..z].iter().all(u8::is_ascii_digit) {
            return Err(ContextError::Manifest);
        }
        Ok(b[a..z].iter().fold(0, |n, c| n * 10 + u32::from(c - b'0')))
    };
    let y = digits(0, 4)?;
    let m = digits(5, 7)?;
    let d = digits(8, 10)?;
    let days = match m {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if y % 4 == 0 && (y % 100 != 0 || y % 400 == 0) {
                29
            } else {
                28
            }
        }
        _ => 0,
    };
    if d == 0 || d > days {
        return Err(ContextError::Manifest);
    }
    if timestamp
        && (b[10] != b'T'
            || b[13] != b':'
            || b[16] != b':'
            || b[19] != b'.'
            || b[23] != b'Z'
            || digits(11, 13)? > 23
            || digits(14, 16)? > 59
            || digits(17, 19)? > 59)
    {
        return Err(ContextError::Manifest);
    }
    if timestamp {
        digits(20, 23)?;
    }
    Ok(())
}
fn hash(b: &[u8]) -> String {
    format!("{:x}", Sha256::digest(b))
}
pub fn manifest_digest(v: &Value) -> Result<String> {
    Ok(hash(&canonical_bytes(v, 49152)?))
}
fn canonical_bytes(v: &Value, max: usize) -> Result<Vec<u8>> {
    fn append(out: &mut Vec<u8>, bytes: &[u8], max: usize) -> Result<()> {
        if bytes.len() > max.saturating_sub(out.len()) {
            return Err(ContextError::Bound);
        }
        out.extend_from_slice(bytes);
        Ok(())
    }
    fn string(out: &mut Vec<u8>, s: &str, max: usize) -> Result<()> {
        append(out, b"\"", max)?;
        for c in s.chars() {
            match c {
                '"' => append(out, b"\\\"", max)?,
                '\\' => append(out, b"\\\\", max)?,
                '\u{8}' => append(out, b"\\b", max)?,
                '\u{c}' => append(out, b"\\f", max)?,
                '\n' => append(out, b"\\n", max)?,
                '\r' => append(out, b"\\r", max)?,
                '\t' => append(out, b"\\t", max)?,
                c if c < '\u{20}' => {
                    let n = c as usize;
                    append(
                        out,
                        &[
                            b'\\',
                            b'u',
                            b'0',
                            b'0',
                            b"0123456789abcdef"[n >> 4],
                            b"0123456789abcdef"[n & 15],
                        ],
                        max,
                    )?;
                }
                _ => {
                    let mut buf = [0u8; 4];
                    append(out, c.encode_utf8(&mut buf).as_bytes(), max)?;
                }
            }
        }
        append(out, b"\"", max)
    }
    fn write(out: &mut Vec<u8>, v: &Value, max: usize, depth: usize) -> Result<()> {
        if depth > 32 {
            return Err(ContextError::Bound);
        }
        match v {
            Value::Null => append(out, b"null", max)?,
            Value::Bool(b) => append(out, if *b { b"true" } else { b"false" }, max)?,
            Value::Number(_) => append(out, number(v)?.to_string().as_bytes(), max)?,
            Value::String(s) => string(out, s, max)?,
            Value::Array(a) => {
                append(out, b"[", max)?;
                for (i, item) in a.iter().enumerate() {
                    if i > 0 {
                        append(out, b",", max)?;
                    }
                    write(out, item, max, depth + 1)?;
                }
                append(out, b"]", max)?;
            }
            Value::Object(o) => {
                let mut keys: Vec<_> = o.keys().collect();
                keys.sort_by(|a, z| a.encode_utf16().cmp(z.encode_utf16()));
                append(out, b"{", max)?;
                for (i, key) in keys.iter().enumerate() {
                    if i > 0 {
                        append(out, b",", max)?;
                    }
                    string(out, key, max)?;
                    append(out, b":", max)?;
                    write(out, &o[*key], max, depth + 1)?;
                }
                append(out, b"}", max)?;
            }
        }
        Ok(())
    }
    let mut out = Vec::new();
    write(&mut out, v, max, 0)?;
    Ok(out)
}
