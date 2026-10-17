use std::process::Command;
#[cfg(not(windows))]
use std::{io::Write, process::Stdio};

#[test]
fn saved_task_witness_command_rejects_extra_arguments_without_output() {
    let result = Command::new(env!("CARGO_BIN_EXE_neko-host"))
        .args(["--saved-witness-reader", "--path", "ignored"])
        .output()
        .unwrap();
    assert!(!result.status.success());
    assert!(result.stdout.is_empty());
    assert_eq!(
        String::from_utf8(result.stderr).unwrap().trim(),
        "witness_reader_arguments"
    );
}
#[cfg(not(windows))]
#[test]
fn saved_task_witness_command_is_not_a_portable_bypass() {
    let mut child = Command::new(env!("CARGO_BIN_EXE_neko-host"))
        .arg("--saved-witness-reader")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    child
        .stdin
        .take()
        .unwrap()
        .write_all(b"{\"version\":1,\"path\":\"/tmp/fixture.json\"}\n")
        .unwrap();
    let result = child.wait_with_output().unwrap();
    assert!(!result.status.success());
    assert!(result.stdout.is_empty());
    assert_eq!(
        String::from_utf8(result.stderr).unwrap().trim(),
        "witness_reader_windows_required"
    );
}
