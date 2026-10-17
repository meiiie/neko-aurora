//! Private preparation/envelope seam, not a process launcher or execution grant.
//! The broker borrows retained executable guards within preparation. Native
//! transport acceptance is still required before run() may select it.
use crate::{
    executable_guard::ExecutableGuard,
    executor::Descriptor,
    saved_key::{self, Binding},
};
use serde_json::{json, Value};
use std::{
    fs,
    path::{Path, PathBuf},
};
type Result<T> = std::result::Result<T, &'static str>;
const ERROR: &str = "saved_account_bootstrap_unavailable";
const MAX_FRAME: usize = 16384;

/// Exact images remain held through preparation and the later ACP child teardown.
/// Callers must keep the returned bootstrap alive until their child has stopped.
pub struct ProtectedExecutables {
    core: PathBuf,
    host: PathBuf,
    _core_guard: ExecutableGuard,
    host_guard: ExecutableGuard,
}
impl ProtectedExecutables {
    pub fn open(core: &Path, expected: &Descriptor) -> Result<Self> {
        #[cfg(not(windows))]
        {
            let _ = (core, expected);
            Err(ERROR)
        }
        #[cfg(windows)]
        {
            let core = fs::canonicalize(core).map_err(|_| ERROR)?;
            let host =
                fs::canonicalize(std::env::current_exe().map_err(|_| ERROR)?).map_err(|_| ERROR)?;
            let core_guard = ExecutableGuard::open(&core).map_err(|_| ERROR)?;
            let host_guard = ExecutableGuard::open(&host).map_err(|_| ERROR)?;
            if !core_guard.native()
                || !host_guard.native()
                || core_guard.sha256() != expected.core_sha256
                || host_guard.sha256() != expected.host_sha256
            {
                return Err(ERROR);
            }
            Ok(Self {
                core,
                host,
                _core_guard: core_guard,
                host_guard,
            })
        }
    }
    pub fn core_path(&self) -> &Path {
        &self.core
    }
    fn native_descriptor(&self) -> Result<Value> {
        let path = self
            .host
            .to_str()
            .ok_or(ERROR)?
            .strip_prefix(r"\\?\")
            .unwrap_or(self.host.to_str().ok_or(ERROR)?);
        Ok(json!({"executable":path,"executableSHA256":self.host_guard.sha256()}))
    }
}

#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct WitnessMetadata {
    pub path: PathBuf,
    pub service_generation: String,
    #[serde(rename = "authoritySHA256")]
    pub authority_sha256: String,
}
fn hex(s: &str, n: usize) -> bool {
    s.len() == n
        && s.bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
}
fn safe_path(path: &Path) -> bool {
    path.is_absolute()
        && path
            .to_str()
            .is_some_and(|s| s.len() <= 4096 && !s.chars().any(char::is_control))
        && !path.components().any(|c| {
            matches!(
                c,
                std::path::Component::ParentDir | std::path::Component::CurDir
            )
        })
}
fn metadata(w: &WitnessMetadata, native: Value) -> Result<Value> {
    if !safe_path(&w.path)
        || !w
            .service_generation
            .strip_prefix("executor-boot-")
            .is_some_and(|s| hex(s, 32))
        || !hex(&w.authority_sha256, 64)
    {
        return Err(ERROR);
    }
    Ok(
        json!({"witnessPath":w.path,"serviceGeneration":w.service_generation,"authoritySHA256":w.authority_sha256,"nativeWitness":native}),
    )
}
fn acp_frame(
    request: &saved_key::Request,
    receipt: &saved_key::BrokerReceipt,
    root: &Path,
    meta: &Value,
) -> Result<Vec<u8>> {
    let mut body = json!({"version":1,"purpose":"saved_account_acp_v1","binding":request.binding,"capsuleId":receipt.capsule_id,"capsulePath":request.capsule_path,"stateHome":request.state_home,"revisionKey":request.revision_key,"witnessPath":meta["witnessPath"],"serviceGeneration":meta["serviceGeneration"],"authoritySHA256":meta["authoritySHA256"],"root":root,"nativeWitness":meta["nativeWitness"]});
    let encoded = serde_json::to_vec(&body).map_err(|_| ERROR);
    if let Some(Value::String(key)) = body.as_object_mut().and_then(|o| o.remove("revisionKey")) {
        let mut bytes = key.into_bytes();
        bytes.fill(0);
    }
    let mut encoded = encoded?;
    let canonical = crate::canonical(&encoded).map_err(|_| ERROR);
    encoded.fill(0);
    let mut frame = canonical?;
    if frame.len() + 1 > MAX_FRAME {
        frame.fill(0);
        return Err(ERROR);
    }
    frame.push(b'\n');
    Ok(frame)
}

/// Private bytes include a revision key; never serialize this object or log the
/// frame. Provider secret and opaque secretRef are excluded from ACP entirely.
pub struct PreparedBootstrap {
    frame: Vec<u8>,
    _images: ProtectedExecutables,
}
impl PreparedBootstrap {
    pub fn frame(&self) -> &[u8] {
        &self.frame
    }
}
impl Drop for PreparedBootstrap {
    fn drop(&mut self) {
        self.frame.fill(0);
    }
}

fn separated_layout(
    root: &Path,
    state: &Path,
    capsule_parent: &Path,
    witness_parent: &Path,
    connections: &Path,
) -> bool {
    state != root
        && state.starts_with(root)
        && !capsule_parent.starts_with(root)
        && !witness_parent.starts_with(root)
        && !connections.starts_with(root)
        && !root.starts_with(connections)
}

pub fn prepare<F>(
    private_request: &[u8],
    now: u64,
    authorize: Option<&dyn Fn(&Binding) -> bool>,
    witness: &WitnessMetadata,
    root: &Path,
    images: ProtectedExecutables,
    mut broker: F,
) -> Result<PreparedBootstrap>
where
    F: FnMut(&ProtectedExecutables, &mut [u8]) -> Result<Vec<u8>>,
{
    if private_request.len() > MAX_FRAME || !safe_path(root) {
        return Err(ERROR);
    }
    let mut canonical = crate::canonical(private_request).map_err(|_| ERROR)?;
    let request = serde_json::from_slice::<saved_key::Request>(&canonical).map_err(|_| ERROR);
    canonical.fill(0);
    let request = request?;
    // The admitted task cannot read its capsule or replace its witness through
    // ordinary root-scoped tools. No new home or budget is created here.
    let root = fs::canonicalize(root).map_err(|_| ERROR)?;
    let state = fs::canonicalize(&request.state_home).map_err(|_| ERROR)?;
    let connections = fs::canonicalize(&request.connections_root).map_err(|_| ERROR)?;
    let capsule_parent = fs::canonicalize(Path::new(&request.capsule_path).parent().ok_or(ERROR)?)
        .map_err(|_| ERROR)?;
    let witness_parent =
        fs::canonicalize(witness.path.parent().ok_or(ERROR)?).map_err(|_| ERROR)?;
    if !root.is_dir()
        || !separated_layout(
            &root,
            &state,
            &capsule_parent,
            &witness_parent,
            &connections,
        )
        || request.binding.fixture_only
    {
        return Err(ERROR);
    }
    let meta = metadata(witness, images.native_descriptor()?)?;
    // saved_key owns catalog/DPAPI/live-authority/receipt/deadline validation.
    let receipt =
        saved_key::prepare_internal(private_request, now, authorize, Some(&meta), |frame| {
            broker(&images, frame)
        })
        .map_err(|_| ERROR)?;
    // Match the existing native host's physical_root wire convention; Core
    // canonicalizes it again before admitting the single ACP session.
    #[cfg(windows)]
    let wire_root = PathBuf::from(
        root.to_str()
            .ok_or(ERROR)?
            .trim_start_matches(r"\\?\")
            .to_lowercase(),
    );
    #[cfg(not(windows))]
    let wire_root = root;
    let frame = acp_frame(&request, &receipt, &wire_root, &meta)?;
    Ok(PreparedBootstrap {
        frame,
        _images: images,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    fn request() -> saved_key::Request {
        serde_json::from_value(json!({"version":1,"purpose":"saved_account_task_v1","binding":{"fixtureOnly":false,"accountId":format!("account-{}","a".repeat(32)),"accountRevision":1,"credentialGeneration":1,"provider":"openai-compatible","endpoint":"https://example.invalid/v1","model":"synthetic","requestId":"req-synthetic","attemptId":"attempt-synthetic","epoch":1,"leaseFence":1,"contextSHA256":"b".repeat(64),"consentId":"req-synthetic","consentRevision":1,"expiresAtUnixMS":2000000000000u64,"limits":{"maxInputBytes":64000,"maxOutputTokens":4096,"maxRequests":8,"maxSteps":6,"maxSeconds":600}},"secretRef":format!("secret-{}","c".repeat(32)),"connectionsRoot":"/synthetic/connections","capsulePath":"/synthetic/private/capsule","stateHome":"/synthetic/task/.core-home","revisionKey":"6".repeat(64)})).unwrap()
    }
    #[test]
    fn layout_rejects_catalog_inside_task_even_when_separate_from_state_home() {
        let root = Path::new("/synthetic/task");
        let state = root.join(".core-home");
        let private = Path::new("/synthetic/private");
        let connections = Path::new("/synthetic/connections");
        assert!(separated_layout(
            root,
            &state,
            private,
            private,
            connections
        ));
        assert!(!separated_layout(
            root,
            &state,
            private,
            private,
            &root.join("connections")
        ));
        assert!(!separated_layout(root, &state, private, private, root));
        assert!(!separated_layout(
            root,
            &state,
            private,
            private,
            Path::new("/synthetic")
        ));
        assert!(!separated_layout(root, &state, root, private, connections));
        assert!(!separated_layout(root, &state, private, root, connections));
        assert!(!separated_layout(root, root, private, private, connections));
    }
    #[cfg(unix)]
    #[test]
    fn canonical_catalog_alias_cannot_hide_task_visible_location() {
        let base = std::env::temp_dir().join(format!(
            "neko-bootstrap-layout-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let root = base.join("task");
        let state = root.join(".core-home");
        let catalog = root.join("connections");
        let private = base.join("private");
        fs::create_dir_all(&state).unwrap();
        fs::create_dir(&catalog).unwrap();
        fs::create_dir(&private).unwrap();
        let alias = base.join("catalog-alias");
        std::os::unix::fs::symlink(&catalog, &alias).unwrap();
        assert!(!separated_layout(
            &fs::canonicalize(&root).unwrap(),
            &fs::canonicalize(&state).unwrap(),
            &fs::canonicalize(&private).unwrap(),
            &fs::canonicalize(&private).unwrap(),
            &fs::canonicalize(&alias).unwrap()
        ));
        fs::remove_dir_all(base).unwrap();
    }
    #[test]
    fn acp_frame_has_exact_keys_one_line_and_no_provider_material() {
        let request = request();
        let receipt = saved_key::BrokerReceipt {
            version: 1,
            capsule_id: format!("sa1_{}", "d".repeat(32)),
            binding_sha256: crate::digest(&serde_json::to_vec(&request.binding).unwrap()).unwrap(),
            expires_at_unix_ms: request.binding.expires_at_unix_ms,
            fixture_only: false,
        };
        let meta = json!({"witnessPath":"/synthetic/private/witness","serviceGeneration":format!("executor-boot-{}","e".repeat(32)),"authoritySHA256":"f".repeat(64),"nativeWitness":{"executable":"C:\\synthetic\\host.exe","executableSHA256":"a".repeat(64)}});
        let frame = acp_frame(&request, &receipt, Path::new("/synthetic/task"), &meta).unwrap();
        assert_eq!(frame.iter().filter(|b| **b == b'\n').count(), 1);
        assert_eq!(frame.last(), Some(&b'\n'));
        let body = &frame[..frame.len() - 1];
        assert_eq!(crate::canonical(body).unwrap(), body);
        let value: Value = serde_json::from_slice(body).unwrap();
        let keys: Vec<_> = value
            .as_object()
            .unwrap()
            .keys()
            .map(String::as_str)
            .collect();
        assert_eq!(
            keys,
            vec![
                "authoritySHA256",
                "binding",
                "capsuleId",
                "capsulePath",
                "nativeWitness",
                "purpose",
                "revisionKey",
                "root",
                "serviceGeneration",
                "stateHome",
                "version",
                "witnessPath"
            ]
        );
        assert_eq!(value["purpose"], "saved_account_acp_v1");
        assert_eq!(
            value["binding"]["contextSHA256"],
            request.binding.context_sha256
        );
        let text = std::str::from_utf8(body).unwrap();
        assert!(!text.contains("secretRef"));
        assert!(!text.contains("connectionsRoot"));
        assert!(value.get("secret").is_none());
    }
    #[test]
    fn acp_frame_enforces_total_line_limit() {
        let request = request();
        let receipt = saved_key::BrokerReceipt {
            version: 1,
            capsule_id: "sa1_synthetic".into(),
            binding_sha256: "a".repeat(64),
            expires_at_unix_ms: request.binding.expires_at_unix_ms,
            fixture_only: false,
        };
        assert!(acp_frame(
            &request,
            &receipt,
            Path::new("/synthetic"),
            &json!({"nativeWitness":{"executable":"x".repeat(MAX_FRAME)}})
        )
        .is_err());
    }
    #[test]
    fn witness_metadata_rejects_malformed_identity_and_relative_path() {
        let root = std::env::temp_dir();
        let mut witness = WitnessMetadata {
            path: root.join("synthetic-witness"),
            service_generation: format!("executor-boot-{}", "a".repeat(32)),
            authority_sha256: "b".repeat(64),
        };
        assert!(metadata(&witness, json!({})).is_ok());
        witness.authority_sha256 = "B".repeat(64);
        assert!(metadata(&witness, json!({})).is_err());
        witness.authority_sha256 = "b".repeat(64);
        witness.service_generation = "wrong-generation".into();
        assert!(metadata(&witness, json!({})).is_err());
        witness.service_generation = format!("executor-boot-{}", "a".repeat(32));
        witness.path = PathBuf::from("relative");
        assert!(metadata(&witness, json!({})).is_err());
    }
    #[cfg(not(windows))]
    #[test]
    fn production_images_have_no_portable_execution_bypass() {
        let expected = Descriptor {
            host_sha256: "a".repeat(64),
            core_sha256: "b".repeat(64),
            root_sha256: "c".repeat(64),
            capabilities_sha256: "d".repeat(64),
        };
        assert!(ProtectedExecutables::open(Path::new("/synthetic/core"), &expected).is_err());
    }
}
