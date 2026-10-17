use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{
    fs,
    io::Read,
    path::{Path, PathBuf},
};
type Result<T> = std::result::Result<T, &'static str>;
const SAFE: u64 = 9_007_199_254_740_991;
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Limits {
    pub max_input_bytes: u64,
    pub max_output_tokens: u64,
    pub max_requests: u64,
    pub max_steps: u64,
    pub max_seconds: u64,
}
#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Binding {
    pub fixture_only: bool,
    pub account_id: String,
    pub account_revision: u64,
    pub credential_generation: u64,
    pub provider: String,
    pub endpoint: String,
    pub model: String,
    pub request_id: String,
    pub attempt_id: String,
    pub epoch: u64,
    pub lease_fence: u64,
    #[serde(rename = "contextSHA256")]
    pub context_sha256: String,
    pub consent_id: String,
    pub consent_revision: u64,
    #[serde(rename = "expiresAtUnixMS")]
    pub expires_at_unix_ms: u64,
    pub limits: Limits,
}
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct Request {
    pub(crate) version: u32,
    pub(crate) purpose: String,
    pub(crate) binding: Binding,
    pub(crate) secret_ref: String,
    pub(crate) connections_root: String,
    pub(crate) capsule_path: String,
    pub(crate) state_home: String,
    pub(crate) revision_key: String,
}
impl Drop for Request {
    fn drop(&mut self) {
        let mut key = std::mem::take(&mut self.revision_key).into_bytes();
        key.fill(0);
    }
}
#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct BrokerReceipt {
    pub version: u32,
    pub capsule_id: String,
    #[serde(rename = "bindingSHA256")]
    pub binding_sha256: String,
    #[serde(rename = "expiresAtUnixMS")]
    pub expires_at_unix_ms: u64,
    pub fixture_only: bool,
}
fn hex(s: &str, n: usize) -> bool {
    s.len() == n
        && s.bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn id(s: &str) -> bool {
    (3..=128).contains(&s.len())
        && s.bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'_' | b'-'))
}
fn text(s: &str, bound: usize) -> bool {
    !s.is_empty() && s.len() <= bound && !s.chars().any(|c| c.is_control() || c == '\u{fffd}')
}
fn endpoint(s: &str, fixture: bool) -> bool {
    if !text(s, 512) || s.bytes().any(|b| matches!(b, b'@' | b'?' | b'#' | b'\\')) {
        return false;
    }
    if fixture {
        return s
            .strip_prefix("http://127.0.0.1:")
            .and_then(|tail| tail.split('/').next())
            .and_then(|p| p.parse::<u16>().ok())
            .is_some_and(|p| p > 0);
    }
    s.strip_prefix("https://").is_some_and(|tail| {
        let host = tail.split('/').next().unwrap_or("");
        !host.is_empty()
            && host
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'-' | b':' | b'[' | b']'))
    })
}
fn valid(b: &Binding, now: u64) -> bool {
    b.account_id
        .strip_prefix("account-")
        .is_some_and(|x| hex(x, 32))
        && [
            b.account_revision,
            b.credential_generation,
            b.epoch,
            b.lease_fence,
            b.consent_revision,
        ]
        .iter()
        .all(|n| *n > 0 && *n <= SAFE)
        && matches!(b.provider.as_str(), "openai-compatible" | "anthropic")
        && endpoint(&b.endpoint, b.fixture_only)
        && text(&b.model, 120)
        && [&b.request_id, &b.attempt_id, &b.consent_id]
            .iter()
            .all(|s| id(s))
        && hex(&b.context_sha256, 64)
        && b.expires_at_unix_ms > now
        && b.expires_at_unix_ms <= SAFE
        && b.expires_at_unix_ms - now <= b.limits.max_seconds.saturating_mul(1000)
        && (1..=64000).contains(&b.limits.max_input_bytes)
        && (1..=4096).contains(&b.limits.max_output_tokens)
        && (1..=8).contains(&b.limits.max_requests)
        && (1..=6).contains(&b.limits.max_steps)
        && (1..=600).contains(&b.limits.max_seconds)
}
pub(crate) fn validate_request(request: &Request, now: u64) -> Result<()> {
    let b = &request.binding;
    if request.version != 1
        || request.purpose != "saved_account_task_v1"
        || !valid(b, now)
        || !hex(&request.revision_key, 64)
        || !request
            .secret_ref
            .strip_prefix("secret-")
            .is_some_and(|s| hex(s, 32))
    {
        return Err("saved_key_input_invalid");
    }
    Ok(())
}
fn no_alias(path: &Path) -> Result<()> {
    for component in path.ancestors() {
        if let Ok(m) = fs::symlink_metadata(component) {
            if m.file_type().is_symlink() {
                return Err("saved_key_path_invalid");
            }
            #[cfg(windows)]
            {
                use std::os::windows::fs::MetadataExt;
                if m.file_attributes() & 0x400 != 0 {
                    return Err("saved_key_path_invalid");
                }
            }
        }
    }
    Ok(())
}
fn directory(s: &str) -> Result<PathBuf> {
    let p = Path::new(s);
    if !p.is_absolute() || !text(s, 4096) {
        return Err("saved_key_path_invalid");
    }
    no_alias(p)?;
    let real = fs::canonicalize(p).map_err(|_| "saved_key_path_invalid")?;
    if !real.is_dir() {
        return Err("saved_key_path_invalid");
    }
    Ok(real)
}
fn current(root: &Path, b: &Binding, reference: &str) -> Result<()> {
    let p = root.join("connections.json");
    no_alias(&p)?;
    let before = fs::metadata(&p).map_err(|_| "saved_key_catalog_unavailable")?;
    if !before.is_file() || before.len() > 65536 {
        return Err("saved_key_catalog_unavailable");
    }
    let mut raw = Vec::new();
    fs::File::open(&p)
        .map_err(|_| "saved_key_catalog_unavailable")?
        .take(65537)
        .read_to_end(&mut raw)
        .map_err(|_| "saved_key_catalog_unavailable")?;
    let after = fs::metadata(&p).map_err(|_| "saved_key_catalog_unavailable")?;
    if before.len() != after.len() || before.modified().ok() != after.modified().ok() {
        return Err("saved_key_catalog_changed");
    }
    let value = crate::acp::frame(&raw).map_err(|_| "saved_key_catalog_unavailable")?;
    let accounts = value["accounts"]
        .as_array()
        .ok_or("saved_key_catalog_unavailable")?;
    if value["version"] != 1 || accounts.len() > 32 {
        return Err("saved_key_catalog_unavailable");
    }
    let selected: Vec<_> = accounts
        .iter()
        .filter(|a| a["id"] == b.account_id)
        .collect();
    if selected.len() != 1 {
        return Err("saved_key_authority_changed");
    }
    let a = selected[0];
    if a["hasSecret"] != true
        || a["secretRef"] != reference
        || a["revision"] != b.account_revision
        || a["credentialGeneration"] != b.credential_generation
        || a["provider"] != b.provider
        || a["endpoint"] != b.endpoint
        || a["model"] != b.model
    {
        return Err("saved_key_authority_changed");
    }
    Ok(())
}
struct PrivateFrame(Vec<u8>);
impl Drop for PrivateFrame {
    fn drop(&mut self) {
        self.0.fill(0)
    }
}
fn deadline_current(binding: &Binding, initial: u64, started: &std::time::Instant) -> Result<()> {
    let wall = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|_| "saved_key_expired")?
        .as_millis();
    if wall < u128::from(initial)
        || wall >= u128::from(binding.expires_at_unix_ms)
        || started.elapsed().as_millis()
            >= u128::from(binding.expires_at_unix_ms.saturating_sub(initial))
    {
        return Err("saved_key_expired");
    }
    Ok(())
}
pub fn prepare<F>(
    private_request: &[u8],
    now: u64,
    authorize: Option<&dyn Fn(&Binding) -> bool>,
    broker: F,
) -> Result<BrokerReceipt>
where
    F: FnMut(&mut [u8]) -> Result<Vec<u8>>,
{
    prepare_internal(private_request, now, authorize, None, broker)
}
pub(crate) fn prepare_internal<F>(
    private_request: &[u8],
    now: u64,
    authorize: Option<&dyn Fn(&Binding) -> bool>,
    metadata: Option<&Value>,
    mut broker: F,
) -> Result<BrokerReceipt>
where
    F: FnMut(&mut [u8]) -> Result<Vec<u8>>,
{
    let started = std::time::Instant::now();
    let raw =
        PrivateFrame(crate::canonical(private_request).map_err(|_| "saved_key_input_invalid")?);
    let request: Request = serde_json::from_slice(&raw.0).map_err(|_| "saved_key_input_invalid")?;
    let b = &request.binding;
    validate_request(&request, now)?;
    deadline_current(b, now, &started)?;
    let authorize = authorize.ok_or("saved_key_authority_denied")?;
    deadline_current(b, now, &started)?;
    if !authorize(b) {
        return Err("saved_key_authority_denied");
    }
    let root = directory(&request.connections_root)?;
    let state = directory(&request.state_home)?;
    let capsule = Path::new(&request.capsule_path);
    if !capsule.is_absolute() || !text(&request.capsule_path, 4096) {
        return Err("saved_key_path_invalid");
    }
    no_alias(capsule)?;
    let parent = directory(
        capsule
            .parent()
            .and_then(|p| p.to_str())
            .ok_or("saved_key_path_invalid")?,
    )?;
    let canonical_capsule = parent.join(capsule.file_name().ok_or("saved_key_path_invalid")?);
    if root.starts_with(&state)
        || state.starts_with(&root)
        || canonical_capsule.starts_with(&root)
        || canonical_capsule.starts_with(&state)
    {
        return Err("saved_key_path_invalid");
    }
    current(&root, b, &request.secret_ref)?;
    let binding_sha256 =
        crate::digest(&serde_json::to_vec(b).map_err(|_| "saved_key_input_invalid")?)
            .map_err(|_| "saved_key_input_invalid")?;
    let returned = crate::vault::with_secret(
        &root,
        &format!("local-owner:local-space:{}", b.account_id),
        &request.secret_ref,
        |secret| {
            deadline_current(b, now, &started)?;
            if !authorize(b) {
                return Err("saved_key_authority_denied");
            }
            current(&root, b, &request.secret_ref)?;
            let material = std::str::from_utf8(secret).map_err(|_| "saved_key_material_invalid")?;
            if !(8..=4096).contains(&material.len())
                || material.bytes().any(|c| !(b'!'..=b'~').contains(&c))
            {
                return Err("saved_key_material_invalid");
            }
            let mut payload = json!({"version":1,"purpose":"saved_account_task_v1","binding":b,"secret":material,"capsulePath":request.capsule_path,"stateHome":request.state_home,"revisionKey":request.revision_key});
            if let Some(metadata) = metadata {
                for key in [
                    "witnessPath",
                    "serviceGeneration",
                    "authoritySHA256",
                    "nativeWitness",
                ] {
                    payload[key] = metadata[key].clone();
                }
            }
            let frame = serde_json::to_vec(&payload).map_err(|_| "saved_key_input_invalid");
            for key in ["secret", "revisionKey"] {
                if let Some(Value::String(material)) =
                    payload.as_object_mut().and_then(|o| o.remove(key))
                {
                    let mut bytes = material.into_bytes();
                    bytes.fill(0);
                }
            }
            let mut frame = PrivateFrame(frame?);
            if frame.0.len() > 16384 {
                return Err("saved_key_input_invalid");
            }
            deadline_current(b, now, &started)?;
            broker(&mut frame.0)
        },
    )?;
    deadline_current(b, now, &started)?;
    if !authorize(b) {
        return Err("saved_key_authority_denied");
    }
    current(&root, b, &request.secret_ref)?;
    let reply = crate::canonical(&returned).map_err(|_| "saved_key_broker_invalid")?;
    if reply.len() > 1024 {
        return Err("saved_key_broker_invalid");
    }
    let receipt: BrokerReceipt =
        serde_json::from_slice(&reply).map_err(|_| "saved_key_broker_invalid")?;
    if receipt.version != 1
        || !receipt
            .capsule_id
            .strip_prefix("sa1_")
            .is_some_and(|s| hex(s, 32))
        || receipt.binding_sha256 != binding_sha256
        || receipt.expires_at_unix_ms != b.expires_at_unix_ms
        || receipt.fixture_only != b.fixture_only
    {
        return Err("saved_key_broker_invalid");
    }
    deadline_current(b, now, &started)?;
    Ok(receipt)
}
