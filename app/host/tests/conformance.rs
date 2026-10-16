use neko_host::{canonical, digest, Admission, CommandGate, Policy, Rejection, MAX_FRAME};
use serde::Deserialize;
use serde_json::{json, Value};

fn policy() -> Policy {
    Policy {
        request_id: "req-one".into(),
        attempt_id: "attempt-one".into(),
        owner_user_id: "local-owner".into(),
        space_id: "local-space".into(),
        bot_id: "bot-meiiie".into(),
        epoch: 2,
        lease_fence: 7,
        root_digest: "a".repeat(64),
    }
}
fn hello() -> Value {
    json!({"contractVersion":1,"acpVersion":1,"profile":"nekobot","exclusive":true,"rootDigest":"a".repeat(64),"capabilities":{"fixedTask":1,"executionBinding":1,"operationIdentity":1,"hostEffect":1}})
}
fn receipt() -> Value {
    json!({"requestId":"req-one","attemptId":"attempt-one","ownerUserId":"local-owner","spaceId":"local-space","botId":"bot-meiiie","coreSessionId":"core-session","coreTaskId":"core-task","epoch":2,"leaseFence":7,"rootDigest":"a".repeat(64)})
}
fn bytes(v: &Value) -> Vec<u8> {
    serde_json::to_vec(v).unwrap()
}
#[test]
fn shared_canonical_fixtures() {
    #[derive(Deserialize)]
    struct Case {
        name: String,
        input: String,
        canonical: Option<String>,
        digest: Option<String>,
        valid: bool,
    }
    let cases: Vec<Case> =
        serde_json::from_str(include_str!("../../contracts/canonical-v1.json")).unwrap();
    for c in cases {
        let result = canonical(c.input.as_bytes());
        if !c.valid {
            assert!(result.is_err(), "{}", c.name);
            continue;
        }
        assert_eq!(
            String::from_utf8(result.unwrap()).unwrap(),
            c.canonical.unwrap(),
            "{}",
            c.name
        );
        assert_eq!(
            digest(c.input.as_bytes()).unwrap(),
            c.digest.unwrap(),
            "{}",
            c.name
        );
    }
}
#[test]
fn matching_capabilities_and_receipt_admitted() {
    let b = policy()
        .admit(&bytes(&hello()), &bytes(&receipt()))
        .unwrap();
    assert_eq!(b.core_task_id, "core-task");
    assert_ne!(b.request_id, b.core_task_id);
}
#[test]
fn changed_capabilities_and_broader_profile_rejected() {
    for key in [
        "fixedTask",
        "executionBinding",
        "operationIdentity",
        "hostEffect",
    ] {
        let mut h = hello();
        h["capabilities"][key] = json!(2);
        assert_eq!(
            policy().admit(&bytes(&h), &bytes(&receipt())),
            Err(Rejection::Capability)
        );
    }
    let mut h = hello();
    h["profile"] = json!("unrestricted");
    assert_eq!(
        policy().admit(&bytes(&h), &bytes(&receipt())),
        Err(Rejection::Capability)
    );
    let mut h = hello();
    h["exclusive"] = json!(false);
    assert_eq!(
        policy().admit(&bytes(&h), &bytes(&receipt())),
        Err(Rejection::Capability)
    );
}
#[test]
fn fake_owner_attempt_epoch_fence_and_root_rejected() {
    for (key, value) in [
        ("ownerUserId", json!("other-owner")),
        ("spaceId", json!("other-space")),
        ("botId", json!("bot-davin")),
        ("requestId", json!("req-other")),
        ("attemptId", json!("attempt-other")),
        ("epoch", json!(1)),
        ("leaseFence", json!(6)),
        ("rootDigest", json!("b".repeat(64))),
    ] {
        let mut r = receipt();
        r[key] = value;
        assert_eq!(
            policy().admit(&bytes(&hello()), &bytes(&r)),
            Err(Rejection::Binding),
            "{key}"
        );
    }
    let mut h = hello();
    h["rootDigest"] = json!("b".repeat(64));
    assert_eq!(
        policy().admit(&bytes(&h), &bytes(&receipt())),
        Err(Rejection::Binding)
    );
}
#[test]
fn unknown_privilege_field_and_oversized_frame_rejected() {
    let mut h = hello();
    h["executable"] = json!("arbitrary-shell");
    assert_eq!(
        policy().admit(&bytes(&h), &bytes(&receipt())),
        Err(Rejection::Frame)
    );
    assert_eq!(canonical(&vec![b' '; MAX_FRAME + 1]), Err(Rejection::Frame));
    assert_eq!(canonical(&[0xff]), Err(Rejection::Frame));
    let deep = format!("{}0{}", "[".repeat(34), "]".repeat(34));
    assert_eq!(canonical(deep.as_bytes()), Err(Rejection::Frame));
}
fn command() -> Value {
    json!({"contractVersion":1,"commandId":"command-one","kind":"Start","requestId":"req-one","attemptId":"attempt-one","epoch":2,"leaseFence":7,"expiresAtMillis":1000})
}
#[test]
fn duplicate_command_and_conflicting_identity_checked() {
    let mut gate = CommandGate::new(policy());
    let c = command();
    assert_eq!(gate.check(&bytes(&c), 100), Ok(Admission::New));
    assert_eq!(gate.check(&bytes(&c), 2000), Ok(Admission::Duplicate));
    let mut changed = c.clone();
    changed["kind"] = json!("Cancel");
    assert_eq!(gate.check(&bytes(&changed), 100), Err(Rejection::Conflict));
}
#[test]
fn expired_and_stale_command_rejected() {
    let mut gate = CommandGate::new(policy());
    let mut c = command();
    c["epoch"] = json!(1);
    assert_eq!(gate.check(&bytes(&c), 100), Err(Rejection::Binding));
    assert_eq!(
        gate.check(&bytes(&command()), 1000),
        Err(Rejection::Expired)
    );
}
