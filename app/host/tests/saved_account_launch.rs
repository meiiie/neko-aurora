use neko_host::{
    executor::{self, Descriptor},
    saved_account_launch::AdmittedLaunch,
};
use serde_json::{json, Value};
use std::{
    fs,
    path::PathBuf,
    sync::atomic::{AtomicU64, Ordering},
};
static NEXT: AtomicU64 = AtomicU64::new(0);
const GOLDEN: &[u8] = include_bytes!("fixtures/saved-account-launch-v1.json");
struct Fixture {
    install: PathBuf,
    envelope: Value,
    actual: Descriptor,
    now: u64,
}
impl Fixture {
    fn new() -> Self {
        // Authored by the Go runtime's actual ledger/selection/builder fixture.
        // Only filesystem locations and the corresponding install-root pin are
        // rebased. Complete manifest, binding and lease authority hashes survive.
        let mut v: Value = serde_json::from_slice(GOLDEN).unwrap();
        let install = std::env::temp_dir().join(format!(
            "saved-launch-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::SeqCst)
        ));
        fs::create_dir_all(&install).unwrap();
        let install = fs::canonicalize(install).unwrap();
        let request = v["launch"]["requestId"].as_str().unwrap();
        let attempt = v["launch"]["attemptLease"]["attemptId"].as_str().unwrap();
        let root = install.join("tasks").join(request);
        let home = root.join(".core-home");
        let private = install.join("saved-accounts").join(request).join(attempt);
        let connections = install.join("connections");
        for p in [&home, &private, &connections] {
            fs::create_dir_all(p).unwrap();
        }
        v["root"] = json!(root);
        v["request"]["stateHome"] = json!(home);
        v["request"]["connectionsRoot"] = json!(connections);
        v["request"]["capsulePath"] = json!(private.join("capsule.json"));
        v["witness"]["path"] = json!(private.join("witness.json"));
        v["launch"]["executorPin"]["rootSha256"] = json!(executor::root_hash(&install).unwrap());
        let pin = &v["launch"]["executorPin"];
        let actual = Descriptor {
            host_sha256: pin["hostSha256"].as_str().unwrap().into(),
            core_sha256: pin["coreSha256"].as_str().unwrap().into(),
            root_sha256: pin["rootSha256"].as_str().unwrap().into(),
            capabilities_sha256: pin["capabilitiesSha256"].as_str().unwrap().into(),
        };
        let now = v["launch"]["attemptLease"]["issuedAtMs"].as_u64().unwrap() + 1;
        Self {
            install,
            envelope: v,
            actual,
            now,
        }
    }
    fn parse(&self, v: &Value) -> Result<AdmittedLaunch, &'static str> {
        AdmittedLaunch::parse(
            &serde_json::to_vec(v).unwrap(),
            &self.install,
            &self.actual,
            self.now,
        )
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        fs::remove_dir_all(&self.install).unwrap();
    }
}
#[test]
fn go_authored_envelope_binds_full_manifest_and_distinct_fences() {
    let f = Fixture::new();
    assert_ne!(
        f.envelope["request"]["binding"]["leaseFence"],
        f.envelope["launch"]["attemptLease"]["fence"]
    );
    let admitted = f.parse(&f.envelope).unwrap();
    assert_eq!(
        admitted.context().session_meta()["neko.contextManifest"],
        f.envelope["launch"]["contextManifest"]
    );
    assert_eq!(
        admitted.root(),
        PathBuf::from(f.envelope["root"].as_str().unwrap())
    );
}
#[test]
fn request_cannot_change_validated_companion_context_or_prompt() {
    let f = Fixture::new();
    for (pointer, value) in [
        (
            "/request/binding/requestId",
            json!(format!("req-{}", "0".repeat(32))),
        ),
        (
            "/request/binding/attemptId",
            json!(format!("attempt-{}", "0".repeat(32))),
        ),
        ("/request/binding/epoch", json!(99)),
        ("/request/binding/leaseFence", json!(0)),
        (
            "/request/binding/contextSHA256",
            f.envelope["launch"]["contextPinSHA256"].clone(),
        ),
        (
            "/witness/serviceGeneration",
            json!(format!("executor-boot-{}", "0".repeat(32))),
        ),
        ("/witness/authoritySHA256", json!("0".repeat(64))),
        ("/launch/text", json!("synthetic different prompt")),
        ("/launch/botId", json!("bot-other")),
        ("/launch/contextProtocol", json!(0)),
        ("/launch/attemptLease/fence", json!(0)),
        ("/launch/executorPin/hostSha256", json!("0".repeat(64))),
        ("/request/binding/fixtureOnly", json!(true)),
    ] {
        let mut v = f.envelope.clone();
        *v.pointer_mut(pointer).unwrap() = value;
        assert!(f.parse(&v).is_err(), "mutation admitted: {pointer}");
    }
}
#[test]
fn closed_schema_rejects_ambient_profiles_native_descriptors_and_duplicates() {
    let f = Fixture::new();
    for pointer in ["", "/request", "/witness", "/launch"] {
        let mut v = f.envelope.clone();
        v.pointer_mut(pointer)
            .unwrap()
            .as_object_mut()
            .unwrap()
            .insert("unexpected".into(), json!(1));
        assert!(f.parse(&v).is_err());
    }
    for key in ["resume", "coreProfilePin", "nativeWitness"] {
        let mut v = f.envelope.clone();
        v["launch"][key] = json!({});
        assert!(f.parse(&v).is_err());
    }
    let mut raw = serde_json::to_vec(&f.envelope).unwrap();
    raw.pop();
    raw.extend_from_slice(b",\"version\":1}");
    assert!(AdmittedLaunch::parse(&raw, &f.install, &f.actual, f.now).is_err());
    assert!(AdmittedLaunch::parse(&vec![b' '; 64001], &f.install, &f.actual, f.now).is_err());
}
#[test]
fn private_paths_and_executor_liveness_cannot_be_substituted() {
    let f = Fixture::new();
    for pointer in [
        "/root",
        "/request/stateHome",
        "/request/connectionsRoot",
        "/request/capsulePath",
        "/witness/path",
    ] {
        let mut v = f.envelope.clone();
        *v.pointer_mut(pointer).unwrap() = json!(f.install);
        assert!(f.parse(&v).is_err(), "path mutation admitted: {pointer}");
    }
    let raw = serde_json::to_vec(&f.envelope).unwrap();
    let expiry = f.envelope["launch"]["attemptLease"]["expiresAtMs"]
        .as_u64()
        .unwrap();
    assert!(AdmittedLaunch::parse(&raw, &f.install, &f.actual, expiry).is_err());
}
#[cfg(not(windows))]
#[test]
fn admitted_envelope_has_no_portable_execution_bypass_and_revokes_failure() {
    let f = Fixture::new();
    let admitted = f.parse(&f.envelope).unwrap();
    let revoked = std::cell::Cell::new(0);
    assert!(admitted
        .prepare(&std::env::current_exe().unwrap(), &|_| true, &|| revoked
            .set(revoked.get() + 1))
        .is_err());
    assert_eq!(revoked.get(), 1);
}
