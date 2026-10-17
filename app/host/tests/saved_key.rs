#![cfg(windows)]
use neko_host::saved_key;
pub use neko_host::{acp, canonical, digest, vault};
use serde_json::{json, Value};
use std::{
    fs,
    io::Write,
    path::PathBuf,
    process::{Command, Stdio},
    sync::atomic::{AtomicU64, Ordering},
    time::{SystemTime, UNIX_EPOCH},
};
static NEXT: AtomicU64 = AtomicU64::new(0);
fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_millis() as u64
}
fn fixture() -> (PathBuf, Value) {
    let root = std::env::temp_dir().join(format!(
        "saved-key-rust-{}-{}",
        std::process::id(),
        NEXT.fetch_add(1, Ordering::SeqCst)
    ));
    fs::create_dir_all(root.join("connections")).unwrap();
    fs::create_dir_all(root.join("task")).unwrap();
    fs::create_dir_all(root.join("private")).unwrap();
    let account = "account-".to_owned() + &"a".repeat(32);
    let reference = "secret-".to_owned() + &"c".repeat(32);
    let endpoint = "https://example.invalid/v1";
    let input = json!({"version":1,"purpose":"saved_account_task_v1","binding":{"fixtureOnly":false,"accountId":account,"accountRevision":1,"credentialGeneration":1,"provider":"openai-compatible","endpoint":endpoint,"model":"fixture-model","requestId":"fixture-request","attemptId":"fixture-attempt","epoch":1,"leaseFence":1,"contextSHA256":"b".repeat(64),"consentId":"fixture-consent","consentRevision":1,"expiresAtUnixMS":now()+30000,"limits":{"maxInputBytes":16000,"maxOutputTokens":256,"maxRequests":2,"maxSteps":2,"maxSeconds":30}},"secretRef":reference,"connectionsRoot":root.join("connections"),"capsulePath":root.join("private/saved-account-capsule.json"),"stateHome":root.join("task"),"revisionKey":"6".repeat(64)});
    fs::write(root.join("connections/connections.json"),serde_json::to_vec(&json!({"version":1,"revision":1,"accounts":[{"id":account,"name":"Fixture saved key","provider":"openai-compatible","endpoint":endpoint,"model":"fixture-model","revision":1,"credentialGeneration":1,"hasSecret":true,"status":"stored_unverified","secretRef":reference,"nonce":"fixture-account-save","fingerprint":"d".repeat(64)}],"plugins":[]})).unwrap()).unwrap();
    let mut secret = b"fixture-saved-account-private-material".to_vec();
    vault::seal(
        &root.join("connections"),
        &format!("local-owner:local-space:{account}"),
        &reference,
        &mut secret,
    )
    .unwrap();
    assert!(secret.iter().all(|b| *b == 0));
    (root, input)
}
fn response(frame: &[u8]) -> Value {
    let v: Value = serde_json::from_slice(frame).unwrap();
    assert_eq!(v["secret"], "fixture-saved-account-private-material");
    json!({"version":1,"capsuleId":"sa1_".to_owned()+&"e".repeat(32),"bindingSHA256":digest(&serde_json::to_vec(&v["binding"]).unwrap()).unwrap(),"expiresAtUnixMS":v["binding"]["expiresAtUnixMS"],"fixtureOnly":v["binding"]["fixtureOnly"]})
}
#[test]
fn private_dpapi_material_only_reaches_bound_broker_callback() {
    let (root, input) = fixture();
    let mut calls = 0;
    let r = saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |frame| {
            calls += 1;
            Ok(serde_json::to_vec(&response(frame)).unwrap())
        },
    )
    .unwrap();
    assert_eq!(calls, 1);
    let public = serde_json::to_string(&r).unwrap();
    assert!(!public.contains("fixture-saved-account-private-material"));
    assert!(!public.contains("secretRef"));
    assert!(!public.contains("revisionKey"));
    let denied = saved_key::prepare(&serde_json::to_vec(&input).unwrap(), now(), None, |_| {
        panic!("missing authority reached broker")
    });
    assert!(denied.is_err());
    assert!(root.exists());
}
#[test]
fn rotation_wrong_reference_and_unbound_response_refuse() {
    let (root, mut input) = fixture();
    input["binding"]["credentialGeneration"] = json!(2);
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |_| panic!("old key reached broker")
    )
    .is_err());
    input["binding"]["credentialGeneration"] = json!(1);
    input["secretRef"] = json!("secret-".to_owned() + &"f".repeat(32));
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |_| panic!("wrong reference reached broker")
    )
    .is_err());
    input["secretRef"] = json!("secret-".to_owned() + &"c".repeat(32));
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |frame| {
            let mut r = response(frame);
            r["bindingSHA256"] = json!("0".repeat(64));
            Ok(serde_json::to_vec(&r).unwrap())
        }
    )
    .is_err());
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |frame| {
            let r = response(frame);
            let path = root.join("connections/connections.json");
            let mut current: Value = serde_json::from_slice(&fs::read(&path).unwrap()).unwrap();
            current["accounts"][0]["credentialGeneration"] = json!(2);
            fs::write(path, serde_json::to_vec(&current).unwrap()).unwrap();
            Ok(serde_json::to_vec(&r).unwrap())
        }
    )
    .is_err());
}
#[test]
fn closed_inputs_expiry_and_fixture_relabelling_refuse() {
    let (_, mut input) = fixture();
    input["extra"] = json!(true);
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |_| panic!("extra field reached broker")
    )
    .is_err());
    input.as_object_mut().unwrap().remove("extra");
    input["binding"]["expiresAtUnixMS"] = json!(now() - 1);
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |_| panic!("expired consent reached broker")
    )
    .is_err());
    input["binding"]["expiresAtUnixMS"] = json!(now() + 30000);
    input["binding"]["fixtureOnly"] = json!(true);
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |_| panic!("relabelled real origin reached broker")
    )
    .is_err());
}
#[test]
fn real_private_stdin_to_core_creates_encrypted_capsule() {
    let (root, input) = fixture();
    let core = PathBuf::from(
        std::env::var("FIXTURE_SAVED_CORE_MODULE").expect("explicit authored Core module required"),
    );
    let bun =
        PathBuf::from(std::env::var("FIXTURE_BUN_EXE").expect("explicit fixture Bun required"));
    let script = root.join("broker-fixture.ts");
    fs::write(&script,format!("import {{prepareSavedAccountCapsule}} from {}; const bytes=await Bun.stdin.bytes(); const input=JSON.parse(new TextDecoder().decode(bytes)); bytes.fill(0);const r=prepareSavedAccountCapsule(input,b=>b.accountId===input.binding.accountId&&b.fixtureOnly===false);console.log(JSON.stringify(r));",serde_json::to_string(&core).unwrap())).unwrap();
    let r = saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |frame| {
            let mut cmd = Command::new(&bun);
            cmd.arg(&script)
                .stdin(Stdio::piped())
                .stdout(Stdio::piped())
                .stderr(Stdio::null())
                .env_clear();
            for key in ["SYSTEMROOT", "WINDIR", "TEMP", "TMP"] {
                if let Ok(v) = std::env::var(key) {
                    cmd.env(key, v);
                }
            }
            let mut child = cmd.spawn().map_err(|_| "fixture_core_spawn")?;
            child
                .stdin
                .take()
                .ok_or("fixture_core_pipe")?
                .write_all(frame)
                .map_err(|_| "fixture_core_pipe")?;
            let output = child.wait_with_output().map_err(|_| "fixture_core_wait")?;
            if !output.status.success() || output.stdout.len() > 1024 {
                return Err("fixture_core_result");
            }
            Ok(output.stdout)
        },
    )
    .unwrap();
    assert!(r.capsule_id.starts_with("sa1_"));
    let ciphertext = fs::read(root.join("private/saved-account-capsule.json")).unwrap();
    assert!(!ciphertext
        .windows(b"fixture-saved-account-private-material".len())
        .any(|w| w == b"fixture-saved-account-private-material"));
    assert!(root
        .join("private/saved-account-capsule.json.current")
        .exists());
}

#[test]
fn broker_completion_after_original_deadline_is_not_prepared() {
    let (_, mut input) = fixture();
    input["binding"]["expiresAtUnixMS"] = json!(now() + 200);
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        |frame| {
            let r = response(frame);
            std::thread::sleep(std::time::Duration::from_millis(250));
            Ok(serde_json::to_vec(&r).unwrap())
        }
    )
    .is_err());
}

#[test]
fn terminal_authority_check_cannot_extend_original_deadline() {
    let (_, mut input) = fixture();
    input["binding"]["expiresAtUnixMS"] = json!(now() + 200);
    let checks = AtomicU64::new(0);
    let authorize = |_: &saved_key::Binding| {
        let n = checks.fetch_add(1, Ordering::SeqCst);
        if n == 2 {
            std::thread::sleep(std::time::Duration::from_millis(250));
        }
        true
    };
    assert!(saved_key::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&authorize),
        |frame| Ok(serde_json::to_vec(&response(frame)).unwrap())
    )
    .is_err());
    assert_eq!(checks.load(Ordering::SeqCst), 3);
}

#[test]
fn private_native_metadata_reaches_prepare_and_provider_secret_stays_out_of_acp() {
    use neko_host::{
        executable_guard::ExecutableGuard,
        executor,
        saved_account_bootstrap::{self, ProtectedExecutables, WitnessMetadata},
    };
    let (root, mut input) = fixture();
    let task = root.join("execution");
    let home = task.join(".core-home");
    fs::create_dir_all(&home).unwrap();
    input["stateHome"] = json!(home);
    let executable = std::env::current_exe().unwrap();
    let image = ExecutableGuard::open(&executable).unwrap();
    let descriptor = executor::descriptor(image.sha256().to_owned(), &root).unwrap();
    drop(image);
    let images = ProtectedExecutables::open(&executable, &descriptor).unwrap();
    let witness = WitnessMetadata {
        path: root.join("private/witness.json"),
        service_generation: format!("executor-boot-{}", "e".repeat(32)),
        authority_sha256: "f".repeat(64),
    };
    let prepared = saved_account_bootstrap::prepare(
        &serde_json::to_vec(&input).unwrap(),
        now(),
        Some(&|_| true),
        &witness,
        &task,
        images,
        |protected, frame| {
            assert_eq!(
                protected.core_path(),
                fs::canonicalize(&executable).unwrap().as_path()
            );
            let value: Value = serde_json::from_slice(frame).unwrap();
            assert_eq!(value["witnessPath"], json!(witness.path));
            assert_eq!(value["serviceGeneration"], witness.service_generation);
            assert_eq!(value["authoritySHA256"], witness.authority_sha256);
            assert_eq!(
                value["nativeWitness"]["executableSHA256"],
                descriptor.host_sha256
            );
            Ok(serde_json::to_vec(&response(frame)).unwrap())
        },
    )
    .unwrap();
    let private_text = std::str::from_utf8(prepared.frame()).unwrap();
    assert!(!private_text.contains("fixture-saved-account-private-material"));
    assert!(!private_text.contains("secretRef"));
    assert!(!private_text.contains("connectionsRoot"));
    assert!(private_text.ends_with('\n'));
    drop(prepared);
    fs::remove_dir_all(root).unwrap();
}
