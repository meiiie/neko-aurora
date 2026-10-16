use neko_host::acp::{self, Binding};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
#[path = "../src/context_admission.rs"]
mod context_admission;
use context_admission::{manifest_digest, LaunchContext};

fn hash(s: &str) -> String {
    format!("{:x}", Sha256::digest(s.as_bytes()))
}
fn manifest() -> Value {
    let text = "Owner <>& Tiếng Việt \u{2028}\u{2029} literal \\u2028\n\t\"\\";
    let scope = json!({"ownerUserId":"local-owner","spaceId":"local-space","botId":"bot-meiiie","projectId":null,"channelId":"conversation-bot-meiiie","audienceRevision":1});
    json!({"version":1,"contextPinSHA256":"b1ac1a26290cf210541a131fb4d7c1c7aa4f68e1faf5c3e79b590f45a271cc81","scope":scope,"primaryMessage":{"id":"message-one","senderId":"local-owner","text":text},"sources":[{"id":"message-one","kind":"original-user","version":1,"sha256":hash(text),"bytes":text.len(),"scope":scope,"accessRevision":0,"observedAt":null,"effectiveFrom":null,"effectiveTo":null,"entities":[],"span":{"offset":0,"end":text.len(),"sha256":hash(text),"text":text}}],"factCandidates":[]})
}
fn participant_manifest() -> Value {
    let mut m = manifest();
    m["version"] = json!(2);
    m["primaryMessage"]["senderId"] = json!("fixture-bob");
    m["requester"] = json!({"version":1,"actorId":"fixture-bob","spaceId":"local-space","authorityId":"fixture-office","deviceId":"device-bob","deviceRevision":1,"sessionId":"session-public-bob","sessionRevision":1,"issuerEpoch":12345,"expiresAtUnixMs":1800000000000u64});
    m
}
fn spec(m: Value) -> Value {
    json!({"contextProtocol":1,"contextManifestSHA256":manifest_digest(&m).unwrap_or_else(|_|"0".repeat(64)),"contextPinSHA256":m["contextPinSHA256"],"contextManifest":m})
}
fn binding(epoch: u64) -> Binding {
    let task = json!({"version":1,"mode":"fixed-active-task","id":"b".repeat(32),"root":"/task","activationEpoch":epoch,"activationId":if epoch==1 {"c".repeat(32)}else{"9".repeat(32)}});
    let execution = json!({"version":1,"taskId":task["id"],"root":"/task","activationEpoch":epoch,"activationId":task["activationId"],"authorityId":"d".repeat(64),"policyId":"e".repeat(64),"bashTarget":"host","capabilities":{"computer":"unavailable","allowedTools":acp::TOOLS}});
    Binding::admit(
        &json!({"sessionId":"a".repeat(32),"_meta":{"neko.task":task,"neko.execution":execution}}),
        "/task",
    )
    .unwrap()
}
fn receipt(b: &Binding, m: &Value) -> Value {
    json!({"version":1,"sessionId":b.session,"contextPinSHA256":m["contextPinSHA256"],"manifestSHA256":manifest_digest(m).unwrap(),"sourceIds":["f".repeat(64)],"task":b.task,"execution":b.execution})
}
fn admission(r: Value) -> Value {
    json!({"_meta":{"neko.contextAdmission":r}})
}

#[test]
fn genuine_go_unicode_and_js_ordinal_goldens() {
    assert_eq!(
        manifest_digest(&manifest()).unwrap(),
        "5df116dbd75e0f0b177be980ca21e9381222e8dc0fc9b1b0af011a5b26b34558"
    );
    let ordinal = json!({"\u{e000}":"BMP","😀":"astral","a":["<>&\u{2028}\u{2029}","\\u2028",9007199254740991u64],"z":null});
    assert_eq!(
        manifest_digest(&ordinal).unwrap(),
        "a215be35b5665a92c0742a8ff34ef9cb69dfee74dba76d1d59eaa83cc79259ad"
    );
}
#[test]
fn fresh_session_meta_and_bound_prompt_tool_echo() {
    let m = manifest();
    let launch = LaunchContext::parse(&spec(m.clone())).unwrap().unwrap();
    assert_eq!(
        launch.session_meta(),
        json!({"neko.contextProtocol":{"version":1},"neko.contextManifest":m})
    );
    let b = binding(1);
    let accepted = launch.admit(&admission(receipt(&b, &m)), &b).unwrap();
    assert_eq!(accepted.receipt(), &receipt(&b, &m));
    let meta = accepted.meta(&b).unwrap();
    assert_eq!(meta["neko.task"], b.task);
    assert_eq!(meta["neko.execution"], b.execution);
    assert_eq!(
        meta["neko.contextAdmission"]["sourceIds"],
        json!(["f".repeat(64)])
    );
    assert!(accepted.echo(&b, &json!({"_meta":meta})).is_ok());
    assert!(accepted.echo(&b, &json!({"_meta":b.meta()})).is_err());
    let mut forged = meta.clone();
    forged["neko.contextAdmission"]["unknown"] = json!(true);
    assert!(accepted.echo(&b, &json!({"_meta":forged})).is_err());
    let mut parallel = meta.clone();
    parallel["neko.contextManifest"] = m.clone();
    assert!(accepted.echo(&b, &json!({"_meta":parallel})).is_err());
    let mut foreign = b.clone();
    foreign.task["activationEpoch"] = json!(2);
    assert!(accepted.meta(&foreign).is_err());
}
#[test]
fn participant_attribution_preserves_workspace_owner_and_native_admission() {
    let m = participant_manifest();
    let l = LaunchContext::parse(&spec(m.clone())).unwrap().unwrap();
    assert_eq!(m["scope"]["ownerUserId"], "local-owner");
    assert_eq!(
        l.session_meta()["neko.contextManifest"]["primaryMessage"]["senderId"],
        "fixture-bob"
    );
    let old = binding(1);
    let accepted = l.admit(&admission(receipt(&old, &m)), &old).unwrap();
    assert!(accepted
        .echo(&old, &json!({"_meta":accepted.meta(&old).unwrap()}))
        .is_ok());
    let saved = receipt(&old, &m);
    let resumed = LaunchContext::parse(&json!({"contextProtocol":1,"contextExpected":saved,"resume":{"sessionId":old.session,"task":old.task,"execution":old.execution}})).unwrap().unwrap();
    let new = binding(2);
    assert!(resumed.admit(&admission(receipt(&new, &m)), &new).is_ok());
    let mut changed = m.clone();
    changed["requester"]["deviceRevision"] = json!(2);
    assert!(l.admit(&admission(receipt(&old, &changed)), &old).is_err());
}
#[test]
fn participant_manifest_rejects_spoofing_and_downgrade() {
    for mutation in [
        "missing", "v1-extra", "v1-bob", "v2-owner", "space", "actor", "unsafe", "zero", "extra",
        "token", "protocol",
    ] {
        let mut m = participant_manifest();
        match mutation {
            "missing" => {
                m.as_object_mut().unwrap().remove("requester");
            }
            "v1-extra" => m["version"] = json!(1),
            "v1-bob" => {
                m["version"] = json!(1);
                m.as_object_mut().unwrap().remove("requester");
            }
            "v2-owner" => m["primaryMessage"]["senderId"] = json!("local-owner"),
            "space" => m["requester"]["spaceId"] = json!("foreign-space"),
            "actor" => m["requester"]["actorId"] = json!("fixture-eve"),
            "unsafe" => m["requester"]["expiresAtUnixMs"] = json!(9007199254740992u64),
            "zero" => m["requester"]["issuerEpoch"] = json!(0),
            "extra" => m["requester"]["grant"] = json!(true),
            "token" => m["requester"]["sessionId"] = json!("bad/session"),
            _ => m["requester"]["version"] = json!(2),
        }
        assert!(LaunchContext::parse(&spec(m)).is_err(), "{mutation}");
    }
    assert!(LaunchContext::parse(&spec(manifest())).is_ok());
}
#[test]
fn legacy_absence_and_strict_mode_exclusivity() {
    assert!(LaunchContext::parse(&json!({"requestId":"legacy"}))
        .unwrap()
        .is_none());
    for (key, value) in [
        ("contextProtocol", json!(1)),
        ("contextManifest", Value::Null),
        ("contextManifestSHA256", json!("a".repeat(64))),
        ("contextPinSHA256", json!("a".repeat(64))),
        ("contextExpected", Value::Null),
    ] {
        let mut v = json!({});
        v[key] = value;
        assert!(LaunchContext::parse(&v).is_err(), "{key}");
    }
    let mut v = spec(manifest());
    v["contextProtocol"] = json!(2);
    assert!(LaunchContext::parse(&v).is_err());
    v = spec(manifest());
    v["contextExpected"] = json!({});
    assert!(LaunchContext::parse(&v).is_err());
    v = spec(manifest());
    v["resume"] = json!({});
    assert!(LaunchContext::parse(&v).is_err());
}
#[test]
fn fresh_receipt_hash_identity_and_opaque_source_negatives() {
    let m = manifest();
    let l = LaunchContext::parse(&spec(m.clone())).unwrap().unwrap();
    let b = binding(1);
    for key in [
        "version",
        "sessionId",
        "contextPinSHA256",
        "manifestSHA256",
        "sourceIds",
        "task",
        "execution",
        "extra",
    ] {
        let mut r = receipt(&b, &m);
        match key {
            "version" => r[key] = json!(2),
            "sessionId" => r[key] = json!("0".repeat(32)),
            "contextPinSHA256" | "manifestSHA256" => r[key] = json!("0".repeat(64)),
            "sourceIds" => r[key] = json!(["message-one"]),
            "task" => r[key]["activationEpoch"] = json!(2),
            "execution" => r[key]["authorityId"] = json!("0".repeat(64)),
            _ => r[key] = json!(true),
        }
        assert!(l.admit(&admission(r), &b).is_err(), "{key}");
    }
    for ids in [
        json!([]),
        json!(["f".repeat(64), "e".repeat(64)]),
        json!(["F".repeat(64)]),
    ] {
        let mut r = receipt(&b, &m);
        r["sourceIds"] = ids;
        assert!(l.admit(&admission(r), &b).is_err());
    }
}
#[test]
fn resume_retains_native_source_identity_but_advances_activation() {
    let m = manifest();
    let old = binding(1);
    let expected = receipt(&old, &m);
    let spec = json!({"contextProtocol":1,"contextExpected":expected,"resume":{"sessionId":old.session,"task":old.task,"execution":old.execution}});
    let l = LaunchContext::parse(&spec).unwrap().unwrap();
    assert_eq!(
        l.session_meta(),
        json!({"neko.contextProtocol":{"version":1},"neko.contextExpected":expected})
    );
    let new = binding(2);
    assert!(l.admit(&admission(receipt(&new, &m)), &new).is_ok());
    let mut r = receipt(&new, &m);
    r["sourceIds"] = json!(["1".repeat(64)]);
    assert!(l.admit(&admission(r), &new).is_err());
    let mut bad = spec.clone();
    bad["contextExpected"]["task"]["id"] = json!("1".repeat(32));
    assert!(LaunchContext::parse(&bad).is_err());
    bad = spec.clone();
    bad["contextExpected"]["extra"] = json!(true);
    assert!(LaunchContext::parse(&bad).is_err());
    bad = spec.clone();
    bad["contextManifest"] = m.clone();
    assert!(LaunchContext::parse(&bad).is_err());
    bad = spec.clone();
    bad["contextPinSHA256"] = m["contextPinSHA256"].clone();
    assert!(LaunchContext::parse(&bad).is_err());
}
#[test]
fn actual_manifest_schema_scope_spans_candidates_and_bounds() {
    for name in [
        "extra",
        "foreign-owner",
        "foreign-source",
        "original-id",
        "original-bytes",
        "span-hash",
        "split-span",
        "date",
        "interval",
        "entity",
        "candidate",
        "unsafe",
        "sources",
        "primary-bound",
    ] {
        let mut m = manifest();
        match name {
            "extra" => m["extra"] = json!(true),
            "foreign-owner" => m["primaryMessage"]["senderId"] = json!("other-owner"),
            "foreign-source" => m["sources"][0]["scope"]["channelId"] = json!("private-channel"),
            "original-id" => m["sources"][0]["id"] = json!("different-message"),
            "original-bytes" => m["sources"][0]["bytes"] = json!(100),
            "span-hash" => m["sources"][0]["span"]["sha256"] = json!("0".repeat(64)),
            "split-span" => {
                m["sources"][0]["span"] =
                    json!({"offset":12,"end":13,"sha256":hash("x"),"text":"x"});
            }
            "date" => m["sources"][0]["observedAt"] = json!("2026-02-30T00:00:00.000Z"),
            "interval" => {
                m["sources"][0]["effectiveFrom"] = json!("2026-10-11");
                m["sources"][0]["effectiveTo"] = json!("2026-10-10");
            }
            "entity" => m["sources"][0]["entities"] = json!([{"id":null,"label":"\u{feff}"}]),
            "candidate" => {
                m["factCandidates"] = json!([{"key":"candidate","sourceId":"message-one","entityId":null,"state":"confirmed","span":m["sources"][0]["span"]}])
            }
            "unsafe" => m["scope"]["audienceRevision"] = json!(9007199254740992u64),
            "sources" => m["sources"] = json!(vec![m["sources"][0].clone(); 17]),
            _ => m["primaryMessage"]["text"] = json!("x".repeat(32769)),
        }
        assert!(LaunchContext::parse(&spec(m)).is_err(), "{name}");
    }
    let mut v = spec(manifest());
    v["contextManifestSHA256"] = json!("0".repeat(64));
    assert!(LaunchContext::parse(&v).is_err());
    v = spec(manifest());
    v["contextPinSHA256"] = json!("0".repeat(64));
    assert!(LaunchContext::parse(&v).is_err());
}
#[test]
fn unchanged_go_export_fixtures() {
    let Ok(dir) = std::env::var("NEKO_CONTEXT_FIXTURES_DIR") else {
        eprintln!("External Go exports not supplied; portable goldens still run");
        return;
    };
    let mut count = 0;
    for entry in std::fs::read_dir(dir).unwrap() {
        let p = entry.unwrap().path();
        let name = p.file_name().unwrap().to_str().unwrap();
        if !name.ends_with(".json") {
            continue;
        }
        let m: Value = serde_json::from_slice(&std::fs::read(&p).unwrap()).unwrap();
        let expected = std::fs::read_to_string(p.with_extension("sha256")).unwrap();
        assert_eq!(manifest_digest(&m).unwrap(), expected, "{name}");
        if !name.ends_with(".value.json") {
            assert!(LaunchContext::parse(&spec(m)).unwrap().is_some(), "{name}");
        }
        count += 1;
    }
    assert_eq!(count, 11, "frozen Go fb export count changed");
}

#[test]
fn bounded_core_schema_vectors() {
    let mut cases: Vec<(&str, Value, bool)> = Vec::new();
    let mut m = manifest();
    m["sources"][0]["observedAt"] = json!("0000-02-29T23:59:59.999Z");
    m["sources"][0]["effectiveFrom"] = json!("2000-02-29");
    m["sources"][0]["effectiveTo"] = json!("2400-02-29");
    cases.push(("canonical leap dates", m, true));
    m = manifest();
    m["scope"]["audienceRevision"] = json!(1.0);
    m["sources"][0]["scope"]["audienceRevision"] = json!(1);
    m["sources"][0]["accessRevision"] = json!(-0.0);
    cases.push(("JS integer normalization", m, true));
    m = manifest();
    m["sources"][0]["entities"] = json!([{"id":null,"label":"\u{85}"}]);
    cases.push(("NEL is not JS trim whitespace", m, true));
    m = manifest();
    m["sources"] = json!([]);
    cases.push(("primary with no descriptive sources", m, true));
    m = manifest();
    let mut sources = vec![m["sources"][0].clone()];
    for n in 1..16 {
        let mut s = sources[0].clone();
        s["id"] = json!(format!("derived-{n}"));
        s["kind"] = json!("derived");
        sources.push(s);
    }
    m["sources"] = json!(sources);
    cases.push(("16 unique bounded sources", m, true));
    m = manifest();
    let candidate = json!({"key":"candidate","sourceId":"message-one","entityId":null,"state":"unresolved","span":m["sources"][0]["span"]});
    m["factCandidates"] = json!(vec![candidate.clone(); 64]);
    cases.push(("64 unresolved candidates", m.clone(), true));
    m["factCandidates"] = json!(vec![candidate; 65]);
    cases.push(("65 unresolved candidates", m, false));
    m = manifest();
    m["sources"][0]["entities"] = json!(vec![json!({"id":null,"label":"literal entity"}); 32]);
    cases.push(("32 entity descriptors", m.clone(), true));
    m["sources"][0]["entities"] = json!(vec![json!({"id":null,"label":"literal entity"}); 33]);
    cases.push(("33 entity descriptors", m, false));
    for (name, change) in [
        ("duplicate source ID", 0),
        ("unknown candidate entity", 1),
        ("candidate outside selected source span", 2),
        ("candidate different literal bytes", 3),
        ("fractional revision", 4),
        ("safe maximum revision", 5),
        ("non-leap Gregorian date", 6),
        ("noncanonical UTC timestamp", 7),
        ("missing explicit null", 8),
    ] {
        m = manifest();
        match change {
            0 => m["sources"] = json!([m["sources"][0].clone(), m["sources"][0].clone()]),
            1 => {
                m["factCandidates"] = json!([{"key":"candidate","sourceId":"message-one","entityId":"unknown-person","state":"unresolved","span":m["sources"][0]["span"]}])
            }
            2 | 3 => {
                let offset = if change == 2 { 100 } else { 0 };
                m["factCandidates"] = json!([{"key":"candidate","sourceId":"message-one","entityId":null,"state":"unresolved","span":{"offset":offset,"end":offset+1,"sha256":hash("x"),"text":"x"}}]);
            }
            4 => m["sources"][0]["version"] = json!(1.5),
            5 => {
                m["scope"]["audienceRevision"] = json!(9007199254740991u64);
                m["sources"][0]["scope"]["audienceRevision"] = json!(9007199254740991u64);
            }
            6 => m["sources"][0]["effectiveFrom"] = json!("2100-02-29"),
            7 => m["sources"][0]["observedAt"] = json!("2026-10-11T00:00:00Z"),
            _ => {
                m["scope"].as_object_mut().unwrap().remove("projectId");
            }
        }
        cases.push((name, m, change == 5));
    }
    for size in [8192, 8193] {
        m = manifest();
        let text = "x".repeat(size);
        m["primaryMessage"]["text"] = json!(text);
        m["sources"][0]["bytes"] = json!(size);
        m["sources"][0]["sha256"] = json!(hash(&text));
        m["sources"][0]["span"] = json!({"offset":0,"end":size,"text":text,"sha256":hash(&text)});
        cases.push((
            if size == 8192 {
                "8192 UTF8 span"
            } else {
                "8193 UTF8 span"
            },
            m,
            size == 8192,
        ));
    }
    m = manifest();
    let mut sources = Vec::new();
    let text = "x".repeat(8192);
    for n in 0..6 {
        let mut s = m["sources"][0].clone();
        s["id"] = json!(format!("derived-{n}"));
        s["kind"] = json!("derived");
        s["bytes"] = json!(8192);
        s["sha256"] = json!(hash(&text));
        s["span"] = json!({"offset":0,"end":8192,"sha256":hash(&text),"text":text});
        sources.push(s);
    }
    m["sources"] = json!(sources);
    cases.push(("48KiB aggregate metadata bound", m, false));
    let mut report = Vec::new();
    for (name, m, valid) in cases {
        let actual = LaunchContext::parse(&spec(m.clone())).is_ok();
        assert_eq!(actual, valid, "{name}");
        report.push(json!({"name":name,"manifest":m,"valid":valid,"rustAccepted":actual,"sha256":manifest_digest(&m).ok()}));
    }
    if let Ok(path) = std::env::var("NEKO_CONTEXT_CASES_PATH") {
        std::fs::write(path, serde_json::to_vec_pretty(&report).unwrap()).unwrap();
    }
}
