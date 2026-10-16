use neko_host::core_profile::{CoreProfileLaunch, PrivateProfileSource};
use serde_json::{json, Value};

fn request() -> (Value, PrivateProfileSource) {
    let limits = json!({"maxInputBytes":64000,"maxOutputTokens":1024,"maxRequests":2,"maxSteps":2,"maxSeconds":60});
    let spec = json!({"text":"fixture request","contextProtocol":1,"contextPinSHA256":"c".repeat(64),"coreProfilePin":{
        "version":1,"fixtureOnly":true,"runtimeSha256":"a".repeat(64),"sourceId":"fixture-source","sourceRevision":1,"profileId":"fixture","profileRevision":format!("pr1_{}","b".repeat(64)),"provider":"openai_compat","model":"fixture-model","providerOrigin":"http://127.0.0.1:9123","consentId":"consent-fixture","consentRevision":1,
        "actorScope":{"version":1,"mode":"local_owner","actorId":"local-owner","spaceId":"local-space","authorityId":"nekobot-local","deviceId":"local-executor","deviceRevision":1},"channelId":"shared-fixture","audienceRevision":1,"botId":"meiiie","clientNonce":"fixture-nonce","contextSha256":"c".repeat(64),"resourcesSha256":"d".repeat(64),"limits":limits,"expiresAtUnixMs":10000
    }});
    let source = PrivateProfileSource {
        home: std::env::temp_dir().join("neko-owned-profile-fixture"),
        key: "e".repeat(64),
        source_id: "fixture-source".into(),
        source_revision: 1,
        profile_id: "fixture".into(),
        profile_revision: format!("pr1_{}", "b".repeat(64)),
        limits,
        fixture_only: true,
        capsule_path: None,
        snapshot_id: None,
    };
    (spec, source)
}

#[test]
fn profile_launch_requires_private_source_and_exact_current_pin() {
    let (spec, source) = request();
    assert!(
        CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 1000)
            .unwrap()
            .is_some()
    );
    assert!(CoreProfileLaunch::parse(&spec, &"a".repeat(64), None, 1000).is_err());
    assert!(CoreProfileLaunch::parse(&spec, &"f".repeat(64), Some(&source), 1000).is_err());
    assert!(CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 10000).is_err());
    for (field, value) in [
        ("sourceId", json!("other")),
        ("profileRevision", json!(format!("pr1_{}", "f".repeat(64)))),
        ("contextSha256", json!("f".repeat(64))),
        ("sourceRevision", json!(2)),
        ("fixtureOnly", json!(false)),
        ("credentialHome", json!("untrusted")),
    ] {
        let mut changed = spec.clone();
        changed["coreProfilePin"][field] = value;
        assert!(
            CoreProfileLaunch::parse(&changed, &"a".repeat(64), Some(&source), 1000).is_err(),
            "accepted changed {field}"
        );
    }
}

#[test]
fn profile_launch_rejects_unsupported_transport_and_scope_or_budget_expansion() {
    let (spec, source) = request();
    for (field, value) in [
        ("provider", json!("gemini_cli")),
        (
            "providerOrigin",
            json!("https://fixture-user@fixture.example"),
        ),
        (
            "actorScope",
            json!({"version":1,"mode":"local_owner","actorId":"foreign","spaceId":"local-space","authorityId":"nekobot-local","deviceId":"local-executor","deviceRevision":1}),
        ),
        (
            "limits",
            json!({"maxInputBytes":64000,"maxOutputTokens":1024,"maxRequests":9,"maxSteps":2,"maxSeconds":60}),
        ),
    ] {
        let mut changed = spec.clone();
        changed["coreProfilePin"][field] = value;
        assert!(
            CoreProfileLaunch::parse(&changed, &"a".repeat(64), Some(&source), 1000).is_err(),
            "accepted changed {field}"
        );
    }
    assert!(
        CoreProfileLaunch::parse(&json!({}), &"a".repeat(64), None, 1000)
            .unwrap()
            .is_none()
    );
    assert!(CoreProfileLaunch::parse(&json!({}), &"a".repeat(64), Some(&source), 1000).is_err());
}

#[test]
fn profile_v2_cached_kimi_requires_bounded_no_refresh_policy() {
    let (mut spec, source) = request();
    let pin = &mut spec["coreProfilePin"];
    pin["version"] = json!(2);
    pin["profileRevision"] = json!(format!("pr2_{}", "b".repeat(64)));
    pin["provider"] = json!("kimi");
    pin["authKind"] = json!("kimi_oauth");
    pin["credentialSource"] = json!("oauth_cache");
    pin["authPolicy"] = json!("cached_token_no_refresh");
    pin["credentialExpiresAtUnixMs"] = json!(12000);
    let mut source = source;
    source.profile_revision = format!("pr2_{}", "b".repeat(64));
    assert!(
        CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 1000)
            .expect("bounded cached OAuth fixture must launch")
            .is_some()
    );
    for (field, value) in [
        ("authPolicy", json!("refresh_automatically")),
        ("credentialExpiresAtUnixMs", json!(999)),
        ("provider", json!("chatgpt")),
    ] {
        let mut changed = spec.clone();
        changed["coreProfilePin"][field] = value;
        assert!(
            CoreProfileLaunch::parse(&changed, &"a".repeat(64), Some(&source), 1000).is_err(),
            "expanded {field}"
        );
    }
}

#[test]
fn profile_v2_environment_requires_exact_private_snapshot_and_deadline() {
    let (mut spec, mut source) = request();
    let snapshot = format!("cs1_{}", "f".repeat(32));
    source.profile_revision = format!("pr2_{}", "b".repeat(64));
    source.capsule_path = Some(std::env::temp_dir().join("private-broker/capsule.bin"));
    source.snapshot_id = Some(snapshot.clone());
    let pin = &mut spec["coreProfilePin"];
    pin["version"] = json!(2);
    pin["profileRevision"] = json!(source.profile_revision);
    pin["authKind"] = json!("api_key");
    pin["credentialSource"] = json!("process_env");
    pin["authPolicy"] = json!("static_key");
    pin["credentialSnapshotId"] = json!(snapshot);
    pin["credentialExpiresAtUnixMs"] = json!(12000);
    let launch = CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 1000)
        .unwrap()
        .unwrap();
    let mut command = std::process::Command::new("fixture-unused-core");
    launch.apply(&mut command).unwrap();
    let environment = command.get_envs().collect::<Vec<_>>();
    assert!(environment
        .iter()
        .any(|(k, _)| *k == "NEKO_PROFILE_CAPSULE_PATH"));
    assert!(environment
        .iter()
        .any(|(k, v)| *k == "NEKO_PROFILE_DEADLINE_UNIX_MS"
            && *v == Some(std::ffi::OsStr::new("10000"))));
    assert!(!environment
        .iter()
        .any(|(k, _)| *k == "FIXTURE_PROVIDER_KEY"));
    for (field, value) in [
        (
            "credentialSnapshotId",
            json!(format!("cs1_{}", "e".repeat(32))),
        ),
        ("credentialExpiresAtUnixMs", json!(9999)),
        ("authKind", json!("chatgpt_oauth")),
    ] {
        let mut changed = spec.clone();
        changed["coreProfilePin"][field] = value;
        assert!(CoreProfileLaunch::parse(&changed, &"a".repeat(64), Some(&source), 1000).is_err());
    }
    source.capsule_path = Some(source.home.join("unsafe-capsule.bin"));
    assert!(CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 1000).is_err());
}

#[test]
fn profile_v2_file_and_local_none_omit_absent_credential_expiry() {
    for (kind, credential_source) in [("api_key", "file_api_key"), ("none", "local_none")] {
        let (mut spec, mut source) = request();
        source.profile_revision = format!("pr2_{}", "b".repeat(64));
        let pin = &mut spec["coreProfilePin"];
        pin["version"] = json!(2);
        pin["profileRevision"] = json!(source.profile_revision);
        pin["authKind"] = json!(kind);
        pin["credentialSource"] = json!(credential_source);
        pin["authPolicy"] = json!("static_key");
        let launch = CoreProfileLaunch::parse(&spec, &"a".repeat(64), Some(&source), 1000)
            .unwrap()
            .unwrap();
        let mut command = std::process::Command::new("fixture-unused-core");
        launch.apply(&mut command).unwrap();
        assert!(
            !command.get_envs().any(|(key, value)| key
                == "NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS"
                && value.is_some()),
            "absent optional expiry must not become zero for {credential_source}"
        );
    }
}
