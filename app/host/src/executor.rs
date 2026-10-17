use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{
    fs,
    path::Path,
    time::{SystemTime, UNIX_EPOCH},
};
const SAFE: u64 = 9_007_199_254_740_991;
pub const CAPABILITIES: &str = "[\"executor_lease_v1\",\"native_pid_session_stop_v1\"]";
type Result<T> = std::result::Result<T, &'static str>;
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Descriptor {
    pub host_sha256: String,
    pub core_sha256: String,
    pub root_sha256: String,
    pub capabilities_sha256: String,
}
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Pin {
    version: u64,
    executor_id: String,
    generation: u64,
    host_sha256: String,
    core_sha256: String,
    root_sha256: String,
    capabilities_sha256: String,
}
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Lease {
    version: u64,
    id: String,
    executor_id: String,
    generation: u64,
    boot_id: String,
    request_id: String,
    attempt_id: String,
    work_epoch: u64,
    authority_sha256: String,
    fence: u64,
    issued_at_ms: u64,
    expires_at_ms: u64,
}
fn positive(n: u64) -> bool {
    n > 0 && n <= SAFE
}
fn hex(s: &str, n: usize) -> bool {
    s.len() == n
        && s.bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn id(s: &str, prefix: &str) -> bool {
    s.strip_prefix(prefix).is_some_and(|s| hex(s, 32))
}
fn strict<T: DeserializeOwned + Serialize>(v: &Value) -> Result<T> {
    let raw = serde_json::to_vec(v).map_err(|_| "executor_companion_invalid")?;
    if raw.len() > 1024 {
        return Err("executor_companion_invalid");
    };
    let out: T = serde_json::from_slice(&raw).map_err(|_| "executor_companion_invalid")?;
    let rendered = serde_json::to_vec(&out).map_err(|_| "executor_companion_invalid")?;
    let actual = crate::canonical(&raw).map_err(|_| "executor_companion_invalid")?;
    let expected = crate::canonical(&rendered).map_err(|_| "executor_companion_invalid")?;
    if actual != expected {
        return Err("executor_companion_invalid");
    };
    Ok(out)
}
pub fn now_ms() -> Result<u64> {
    let n = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|_| "executor_clock_invalid")?
        .as_millis();
    u64::try_from(n)
        .ok()
        .filter(|n| positive(*n))
        .ok_or("executor_clock_invalid")
}
pub fn root_hash(path: &Path) -> Result<String> {
    let resolved = fs::canonicalize(path).map_err(|_| "executor_state_root_missing")?;
    let text = resolved.to_str().ok_or("executor_state_root_invalid")?;
    #[cfg(windows)]
    let text = text
        .trim_start_matches("\\\\?\\")
        .replace('\\', "/")
        .to_lowercase();
    #[cfg(not(windows))]
    let text = text.to_owned();
    Ok(format!(
        "{:x}",
        Sha256::digest(text.trim_end_matches('/').as_bytes())
    ))
}
pub fn descriptor(core_sha: String, state: &Path) -> Result<Descriptor> {
    let own = std::env::current_exe().map_err(|_| "executor_host_missing")?;
    let bytes = fs::read(own).map_err(|_| "executor_host_missing")?;
    descriptor_with_host_digest(core_sha, format!("{:x}", Sha256::digest(bytes)), state)
}
pub fn descriptor_with_host_digest(core_sha: String, host_sha: String, state: &Path) -> Result<Descriptor> {
    if ![&core_sha, &host_sha].iter().all(|s| {
        s.len() == 64 && s.bytes().all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
    }) {
        return Err("executor_descriptor_invalid");
    }
    Ok(Descriptor {
        host_sha256: host_sha,
        core_sha256: core_sha,
        root_sha256: root_hash(state)?,
        capabilities_sha256: format!("{:x}", Sha256::digest(CAPABILITIES.as_bytes())),
    })
}
#[derive(Debug, Clone)]
pub struct Companion {
    lease: Lease,
}
impl Companion {
    pub fn validate(
        pin: &Value,
        lease: &Value,
        actual: &Descriptor,
        request: &str,
        now: u64,
    ) -> Result<Self> {
        let p: Pin = strict(pin)?;
        let l: Lease = strict(lease)?;
        let d = Descriptor {
            host_sha256: p.host_sha256,
            core_sha256: p.core_sha256,
            root_sha256: p.root_sha256,
            capabilities_sha256: p.capabilities_sha256,
        };
        if p.version != 1
            || !id(&p.executor_id, "executor-")
            || !positive(p.generation)
            || d != *actual
            || ![
                &d.host_sha256,
                &d.core_sha256,
                &d.root_sha256,
                &d.capabilities_sha256,
            ]
            .iter()
            .all(|s| hex(s, 64))
            || l.version != 1
            || !id(&l.id, "executor-lease-")
            || l.executor_id != p.executor_id
            || l.generation != p.generation
            || !id(&l.boot_id, "executor-boot-")
            || !id(&l.request_id, "req-")
            || l.request_id != request
            || !id(&l.attempt_id, "attempt-")
            || !positive(l.work_epoch)
            || !hex(&l.authority_sha256, 64)
            || !positive(l.fence)
            || !positive(l.issued_at_ms)
            || !positive(l.expires_at_ms)
            || l.expires_at_ms <= l.issued_at_ms
            || l.expires_at_ms - l.issued_at_ms > 600000
        {
            return Err("executor_companion_invalid");
        }
        let c = Self { lease: l };
        c.live(now)?;
        Ok(c)
    }
    pub fn live(&self, now: u64) -> Result<()> {
        if !positive(now) || now < self.lease.issued_at_ms || now >= self.lease.expires_at_ms {
            Err("executor_lease_expired")
        } else {
            Ok(())
        }
    }
    pub fn proof(&self) -> Value {
        let l = &self.lease;
        json!({"version":1,"executorId":l.executor_id,"generation":l.generation,"bootId":l.boot_id,"leaseId":l.id,"requestId":l.request_id,"attemptId":l.attempt_id,"workEpoch":l.work_epoch,"authoritySha256":l.authority_sha256,"fence":l.fence})
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    fn fixture() -> (serde_json::Value, serde_json::Value, Descriptor) {
        let d = Descriptor {
            host_sha256: "1".repeat(64),
            core_sha256: "2".repeat(64),
            root_sha256: "3".repeat(64),
            capabilities_sha256: "4".repeat(64),
        };
        let p = json!({"version":1,"executorId":format!("executor-{}","a".repeat(32)),"generation":1,"hostSha256":d.host_sha256,"coreSha256":d.core_sha256,"rootSha256":d.root_sha256,"capabilitiesSha256":d.capabilities_sha256});
        let l = json!({"version":1,"id":format!("executor-lease-{}","b".repeat(32)),"executorId":p["executorId"],"generation":1,"bootId":format!("executor-boot-{}","c".repeat(32)),"requestId":format!("req-{}","d".repeat(32)),"attemptId":format!("attempt-{}","e".repeat(32)),"workEpoch":1,"authoritySha256":"f".repeat(64),"fence":1,"issuedAtMs":1000,"expiresAtMs":601000});
        (p, l, d)
    }
    #[test]
    fn strict_companion_and_proof() {
        let (p, l, d) = fixture();
        let c = Companion::validate(&p, &l, &d, l["requestId"].as_str().unwrap(), 1001).unwrap();
        assert_eq!(c.proof()["attemptId"], l["attemptId"]);
        assert_eq!(c.proof()["leaseId"], l["id"]);
    }
    #[test]
    fn rejects_expired_future_overlong_or_mismatch() {
        let (p, l, d) = fixture();
        for now in [999, 601000, 601001] {
            assert!(Companion::validate(&p, &l, &d, l["requestId"].as_str().unwrap(), now).is_err())
        }
        let mut bad = l.clone();
        bad["expiresAtMs"] = json!(601001);
        assert!(Companion::validate(&p, &bad, &d, l["requestId"].as_str().unwrap(), 1001).is_err());
        let mut bad = p.clone();
        bad["rootSha256"] = json!("0".repeat(64));
        assert!(Companion::validate(&bad, &l, &d, l["requestId"].as_str().unwrap(), 1001).is_err());
    }
    #[test]
    fn rejects_unknown_missing_cased_and_wrong_version() {
        let (p, l, d) = fixture();
        for key in ["unknown", "Generation"] {
            let mut bad = l.clone();
            bad[key] = json!(1);
            assert!(
                Companion::validate(&p, &bad, &d, l["requestId"].as_str().unwrap(), 1001).is_err()
            )
        }
        let mut bad = l.clone();
        bad.as_object_mut().unwrap().remove("fence");
        assert!(Companion::validate(&p, &bad, &d, l["requestId"].as_str().unwrap(), 1001).is_err());
        let mut bad = p.clone();
        bad["version"] = json!(2);
        assert!(Companion::validate(&bad, &l, &d, l["requestId"].as_str().unwrap(), 1001).is_err());
    }
    #[test]
    fn rejects_wrong_request_and_lease_install() {
        let (p, l, d) = fixture();
        assert!(Companion::validate(&p, &l, &d, "req-wrong", 1001).is_err());
        let mut bad = l.clone();
        bad["generation"] = json!(2);
        assert!(Companion::validate(&p, &bad, &d, l["requestId"].as_str().unwrap(), 1001).is_err());
    }
}
