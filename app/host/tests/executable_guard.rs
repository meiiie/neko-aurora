use neko_host::executable_guard::ExecutableGuard;
use sha2::{Digest, Sha256};
use std::{fs, path::PathBuf};
fn fixture() -> PathBuf {
    let path = std::env::temp_dir().join(format!(
        "neko-executable-guard-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    fs::create_dir(&path).unwrap();
    path
}
#[test]
fn hashes_exact_opened_bytes_and_detects_native_header() {
    let root = fixture();
    let path = root.join("fixture.exe");
    let bytes = b"MZsynthetic-executable-fixture";
    fs::write(&path, bytes).unwrap();
    let guard = ExecutableGuard::open(&path).unwrap();
    assert_eq!(guard.sha256(), format!("{:x}", Sha256::digest(bytes)));
    assert!(guard.native());
    drop(guard);
    fs::remove_dir_all(root).unwrap();
}
#[cfg(windows)]
#[test]
fn held_executable_and_ancestors_deny_write_delete_and_rename() {
    let root = fixture();
    let dir = root.join("install");
    fs::create_dir(&dir).unwrap();
    let path = dir.join("fixture.exe");
    fs::write(&path, b"MZsynthetic-fixture").unwrap();
    let guard = ExecutableGuard::open(&path).unwrap();
    assert!(fs::read(&path).is_ok());
    assert!(fs::write(&path, b"replacement").is_err());
    assert!(fs::remove_file(&path).is_err());
    assert!(fs::rename(&path, dir.join("moved.exe")).is_err());
    assert!(fs::rename(&dir, root.join("moved")).is_err());
    drop(guard);
    fs::write(&path, b"replacement").unwrap();
    fs::rename(&dir, root.join("moved")).unwrap();
    fs::remove_dir_all(root).unwrap();
}
#[cfg(windows)]
#[test]
fn prior_write_handle_hardlinks_and_reparse_paths_fail_closed() {
    use std::os::windows::fs::OpenOptionsExt;
    let root = fixture();
    let path = root.join("fixture.exe");
    fs::write(&path, b"MZsynthetic-fixture").unwrap();
    let writer = fs::OpenOptions::new()
        .write(true)
        .share_mode(7)
        .open(&path)
        .unwrap();
    assert!(ExecutableGuard::open(&path).is_err());
    drop(writer);
    let alias = root.join("alias.exe");
    fs::hard_link(&path, &alias).unwrap();
    assert!(ExecutableGuard::open(&path).is_err());
    fs::remove_file(alias).unwrap();
    let real = root.join("real");
    fs::create_dir(&real).unwrap();
    fs::write(real.join("fixture.exe"), b"MZsynthetic-fixture").unwrap();
    let junction = root.join("junction");
    assert!(std::process::Command::new("cmd.exe")
        .args(["/d", "/c", "mklink", "/J"])
        .arg(&junction)
        .arg(&real)
        .status()
        .unwrap()
        .success());
    assert!(ExecutableGuard::open(&junction.join("fixture.exe")).is_err());
    fs::remove_dir(junction).unwrap();
    fs::remove_dir_all(root).unwrap();
}
#[cfg(windows)]
#[test]
fn guarded_native_process_can_launch_and_guard_remains_until_exit() {
    let root = fixture();
    let path = root.join("host.exe");
    fs::copy(env!("CARGO_BIN_EXE_neko-host"), &path).unwrap();
    let guard = ExecutableGuard::open(&path).unwrap();
    let result = std::process::Command::new(&path)
        .arg("--invalid-fixture-argument")
        .output()
        .unwrap();
    assert!(!result.status.success());
    assert!(fs::remove_file(&path).is_err());
    drop(guard);
    fs::remove_dir_all(root).unwrap();
}

#[test]
fn retained_host_digest_preserves_descriptor_and_rejects_malformed_input() {
    let root = fixture();
    let host = ExecutableGuard::open(&std::env::current_exe().unwrap()).unwrap();
    let core = "a".repeat(64);
    let expected = neko_host::executor::descriptor(core.clone(), &root).unwrap();
    let actual = neko_host::executor::descriptor_with_host_digest(
        core.clone(),
        host.sha256().to_owned(),
        &root,
    )
    .unwrap();
    assert_eq!(
        serde_json::to_value(expected).unwrap(),
        serde_json::to_value(actual).unwrap()
    );
    for invalid in ["", "ABCDEF", &"g".repeat(64), &"A".repeat(64)] {
        assert!(neko_host::executor::descriptor_with_host_digest(
            core.clone(),
            invalid.to_owned(),
            &root
        )
        .is_err());
        assert!(neko_host::executor::descriptor_with_host_digest(
            invalid.to_owned(),
            host.sha256().to_owned(),
            &root
        )
        .is_err());
    }
    drop(host);
    fs::remove_dir_all(root).unwrap();
}
