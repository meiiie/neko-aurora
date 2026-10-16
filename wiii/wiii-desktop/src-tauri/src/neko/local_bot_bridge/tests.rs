use super::*;
use std::{
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    sync::{
        atomic::{AtomicBool, AtomicUsize, Ordering},
        Arc,
    },
    thread,
};

struct Fixture {
    port: u16,
    hits: Arc<AtomicUsize>,
    stop: Arc<AtomicBool>,
    worker: Option<thread::JoinHandle<()>>,
}
impl Fixture {
    fn new(reply: impl Fn(&str, &Value) -> (u16, Value) + Send + 'static) -> Self {
        Self::with_capabilities(None, reply)
    }
    fn with_capabilities(
        capabilities: Option<Value>,
        reply: impl Fn(&str, &Value) -> (u16, Value) + Send + 'static,
    ) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        listener.set_nonblocking(true).unwrap();
        let port = listener.local_addr().unwrap().port();
        let hits = Arc::new(AtomicUsize::new(0));
        let stop = Arc::new(AtomicBool::new(false));
        let counted = hits.clone();
        let stopping = stop.clone();
        let worker = thread::spawn(move || {
            while !stopping.load(Ordering::SeqCst) {
                let Ok((mut stream, _)) = listener.accept() else {
                    thread::sleep(Duration::from_millis(2));
                    continue;
                };
                stream.set_nonblocking(false).unwrap();
                stream
                    .set_read_timeout(Some(Duration::from_secs(2)))
                    .unwrap();
                let mut bytes = Vec::new();
                let mut buf = [0; 4096];
                let header_end = loop {
                    let n = stream.read(&mut buf).unwrap();
                    if n == 0 {
                        break None;
                    }
                    bytes.extend_from_slice(&buf[..n]);
                    if let Some(i) = bytes.windows(4).position(|w| w == b"\r\n\r\n") {
                        break Some(i + 4);
                    }
                };
                let Some(end) = header_end else { continue };
                let headers = String::from_utf8(bytes[..end].to_vec()).unwrap();
                let length = headers
                    .lines()
                    .find_map(|s| {
                        s.to_ascii_lowercase()
                            .strip_prefix("content-length:")
                            .map(str::trim)
                            .and_then(|s| s.parse::<usize>().ok())
                    })
                    .unwrap_or(0);
                while bytes.len() < end + length {
                    let n = stream.read(&mut buf).unwrap();
                    if n == 0 {
                        break;
                    }
                    bytes.extend_from_slice(&buf[..n]);
                }
                let path = headers.lines().next().unwrap().split(' ').nth(1).unwrap();
                let body = serde_json::from_slice(&bytes[end..]).unwrap_or(Value::Null);
                counted.fetch_add(1, Ordering::SeqCst);
                let (status, value, cookie) = if path == "/" {
                    (200, json!({}), "Set-Cookie: neko_local=0000000000000000000000000000000000000000000000000000000000000000; Path=/; HttpOnly; SameSite=Strict\r\n")
                } else if path == "/v1/adapters/capabilities" {
                    assert!(headers.to_ascii_lowercase().contains("cookie: neko_local="));
                    capabilities
                        .as_ref()
                        .map_or((404, json!({"error":"not_found"}), ""), |value| {
                            (200, value.clone(), "")
                        })
                } else {
                    assert!(headers.to_ascii_lowercase().contains("cookie: neko_local="));
                    let (s, v) = reply(path, &body);
                    (s, v, "")
                };
                let body = serde_json::to_vec(&value).unwrap();
                let response = format!("HTTP/1.1 {status} OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n{cookie}\r\n", body.len());
                let _ = stream.write_all(response.as_bytes());
                let _ = stream.write_all(&body);
            }
        });
        Self {
            port,
            hits,
            stop,
            worker: Some(worker),
        }
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::SeqCst);
        let _ = TcpStream::connect(("127.0.0.1", self.port));
        if let Some(w) = self.worker.take() {
            let joined = w.join();
            if !thread::panicking() {
                assert!(joined.is_ok(), "owned HTTP fixture worker failed");
            }
        }
    }
}
fn runtime() -> tokio::runtime::Runtime {
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap()
}
fn catalog() -> Value {
    json!({"version":1,"ownerUserId":"local-owner","spaceId":"local-space","channels":[{"id":"channel-a","name":"Shared","revision":2,"botIds":["meiiie"]}],"bots":[{"id":"meiiie","name":"Meiiie"}]})
}
fn input() -> LinkInput {
    LinkInput {
        channel_id: "channel-a".into(),
        expected_channel_revision: 2,
        source_session_id: "wiii-source-1".into(),
        source_thread_id: "wiii-thread-1".into(),
        wiii_project_id: "project-local".into(),
    }
}
fn session() -> Value {
    json!({"id":"adapter-a","version":1,"ownerUserId":"local-owner","spaceId":"local-space","sourceApp":"wiii","sourceSessionId":"wiii-source-1","sourceThreadId":"wiii-thread-1","channelId":"channel-a","channelRevision":2,"returnRoute":"/v1/adapters/sessions/adapter-a/replies"})
}
fn page() -> Value {
    let mut p = session();
    let m = p.as_object_mut().unwrap();
    m.remove("version");
    m.remove("id");
    m.insert("adapterSessionId".into(), json!("adapter-a"));
    m.insert("cursor".into(), json!(1));
    m.insert("replies".into(), json!([]));
    p
}
fn history_page() -> Value {
    let mut p = page();
    let m = p.as_object_mut().unwrap();
    m.remove("cursor");
    m.remove("replies");
    let mut raw = vec![0; 161];
    raw[0] = 1;
    for (key, value) in [
        ("version", json!(1)),
        ("snapshotId", json!(format!("h1_{}", "a".repeat(64)))),
        ("snapshotCursor", json!(1)),
        ("authorityDigest", json!("a".repeat(64))),
        ("historyDigest", json!("b".repeat(64))),
        ("offset", json!(0)),
        ("totalRows", json!(0)),
        ("totalBytes", json!(0)),
        ("nextCursor", Value::Null),
        ("confirmationCursor", json!(URL_SAFE_NO_PAD.encode(raw))),
        ("complete", json!(true)),
        ("rows", json!([])),
    ] {
        m.insert(key.into(), value);
    }
    p
}
fn registration(link: &Link) -> Request {
    Request {
        lease_id: link.lease_id.clone(),
        method: "POST".into(),
        path: "/v1/adapters/sessions".into(),
        body: Some(
            json!({"sourceApp":"wiii","sourceSessionId":"wiii-source-1","sourceThreadId":"wiii-thread-1","channelId":"channel-a","expectedChannelRevision":2,"clientNonce":"nonce-0001"}),
        ),
    }
}
fn connect(bridge: &LocalBotBridge, fixture: &Fixture) -> Link {
    runtime()
        .block_on(bridge.finish_connect(
            bridge.begin_connect(fixture.port).unwrap(),
            fixture.port,
            true,
        ))
        .unwrap();
    bridge.link(input()).unwrap()
}

#[test]
fn default_disconnected_and_owner_denial_make_zero_http_requests() {
    let f = Fixture::new(|_, _| panic!("HTTP must not happen"));
    let b = LocalBotBridge::default();
    assert!(!b.status().unwrap().connected);
    assert!(b.link(input()).is_err());
    assert!(b.begin_connect(0).is_err());
    let t = b.begin_connect(f.port).unwrap();
    assert!(b.status().unwrap().connecting);
    assert_eq!(
        runtime()
            .block_on(b.finish_connect(t, f.port, false))
            .err()
            .unwrap(),
        error("owner_declined")
    );
    assert_eq!(f.hits.load(Ordering::SeqCst), 0);
    assert!(!b.status().unwrap().connected);
}
#[test]
fn only_main_local_webview_can_invoke_and_dev_origin_is_debug_only() {
    for url in ["tauri://localhost/", "http://tauri.localhost/index.html"] {
        assert!(trusted_surface(
            "main",
            &reqwest::Url::parse(url).unwrap(),
            false
        ));
        assert!(!trusted_surface(
            "splashscreen",
            &reqwest::Url::parse(url).unwrap(),
            true
        ));
    }
    for url in [
        "https://example.com/",
        "http://127.0.0.1:8765/",
        "http://tauri.localhost:1234/",
        "http://user:pass@tauri.localhost/",
        "https://tauri.localhost/",
        "tauri://other/",
    ] {
        assert!(!trusted_surface(
            "main",
            &reqwest::Url::parse(url).unwrap(),
            true
        ));
    }
    let dev = reqwest::Url::parse("http://localhost:1420/").unwrap();
    assert!(trusted_surface("main", &dev, true));
    assert!(!trusted_surface("main", &dev, false));
    let capability: Value = serde_json::from_str(include_str!(
        "../../../capabilities/workbench-nekobot-local.json"
    ))
    .unwrap();
    assert_eq!(capability["windows"], json!(["main"]));
    assert!(capability.get("remote").is_none());
}
#[test]
fn approved_bootstrap_is_native_only_metadata_and_link_does_not_register() {
    let f = Fixture::new(|path, _| {
        assert_eq!(path, "/v1/adapters/catalog");
        (200, catalog())
    });
    let b = LocalBotBridge::default();
    let link = connect(&b, &f);
    assert_eq!(f.hits.load(Ordering::SeqCst), 3);
    let public = serde_json::to_string(&b.status().unwrap()).unwrap();
    assert!(!public.contains("cookie") && !public.contains("000000000000"));
    assert_eq!(link.source.owner_user_id, "local-owner");
    assert_eq!(link.source.space_id, "local-space");
    assert_eq!(link.bots.len(), 1);
    let mut stale = input();
    stale.expected_channel_revision = 3;
    assert!(b.link(stale).is_err());
    assert_eq!(f.hits.load(Ordering::SeqCst), 3);
    b.disconnect().unwrap();
    assert!(!b.status().unwrap().connected);
    assert!(b.link(input()).is_err());
}
#[test]
fn unsupported_authority_fields_and_arbitrary_routes_are_rejected_before_http() {
    let f = Fixture::new(|_, _| (200, catalog()));
    let b = LocalBotBridge::default();
    let link = connect(&b, &f);
    for path in [
        "https://example.com/",
        "/v1/connections",
        "/v1/adapters/sessions/adapter-a/replies?after=0&cookie=x",
        "/v1/adapters/sessions/%61/replies?after=0",
        "/v1/adapters/sessions/adapter-a/replies?after=-1",
        "/v1/adapters/sessions/./replies?after=0",
        "/v1/adapters/sessions/../replies?after=0",
    ] {
        let r = Request {
            lease_id: link.lease_id.clone(),
            method: "GET".into(),
            path: path.into(),
            body: None,
        };
        assert!(runtime().block_on(b.request(r)).is_err(), "{path}");
    }
    let mut r = registration(&link);
    r.body.as_mut().unwrap()["actorId"] = json!("owner");
    assert!(runtime().block_on(b.request(r)).is_err());
    let mut r = registration(&link);
    r.body.as_mut().unwrap()["sourceThreadId"] = json!("other-thread");
    assert!(runtime().block_on(b.request(r)).is_err());
    assert_eq!(f.hits.load(Ordering::SeqCst), 3);
}
#[test]
fn registration_receipt_must_match_full_server_binding() {
    for field in [
        "ownerUserId",
        "spaceId",
        "sourceSessionId",
        "sourceThreadId",
        "channelId",
        "returnRoute",
    ] {
        let field = field.to_owned();
        let f = Fixture::new(move |path, _| {
            if path == "/v1/adapters/catalog" {
                (200, catalog())
            } else {
                let mut s = session();
                s[&field] = json!("wrong");
                (200, json!({"session":s,"duplicate":false}))
            }
        });
        let b = LocalBotBridge::default();
        let l = connect(&b, &f);
        assert!(runtime().block_on(b.request(registration(&l))).is_err());
    }
}
#[test]
fn registration_and_saved_adapter_replay_are_scoped_to_selected_link() {
    let f = Fixture::new(|path, _| {
        if path == "/v1/adapters/catalog" {
            (200, catalog())
        } else if path.ends_with("?after=0") {
            (200, page())
        } else {
            (200, json!({"session":session(),"duplicate":true}))
        }
    });
    let b = LocalBotBridge::default();
    let l = connect(&b, &f);
    let ack = runtime().block_on(b.request(registration(&l))).unwrap();
    assert_eq!(ack.status, 200);
    assert!(ack.body.contains("adapter-a"));
    let r = Request {
        lease_id: l.lease_id,
        method: "GET".into(),
        path: "/v1/adapters/sessions/adapter-a/replies?after=0".into(),
        body: None,
    };
    assert_eq!(runtime().block_on(b.request(r)).unwrap().status, 200);
}
#[test]
fn registration_receipt_cannot_introduce_a_normalized_adapter_route() {
    for adapter in [".", ".."] {
        let f = Fixture::new(move |path, _| {
            if path == "/v1/adapters/catalog" {
                (200, catalog())
            } else {
                let mut s = session();
                s["id"] = json!(adapter);
                s["returnRoute"] = json!(format!("/v1/adapters/sessions/{adapter}/replies"));
                (200, json!({"session":s,"duplicate":false}))
            }
        });
        let b = LocalBotBridge::default();
        let l = connect(&b, &f);
        assert!(runtime().block_on(b.request(registration(&l))).is_err());
        assert_eq!(f.hits.load(Ordering::SeqCst), 4);
        assert!(b.state.lock().unwrap().leases[&l.lease_id]
            .adapter_ids
            .is_empty());
    }
}
#[test]
fn reply_with_other_owner_and_hostile_oversized_page_never_reaches_webview() {
    for oversize in [false, true] {
        let f = Fixture::new(move |path, _| {
            if path == "/v1/adapters/catalog" {
                (200, catalog())
            } else {
                let mut p = page();
                if oversize {
                    p["padding"] = json!("x".repeat(MAX_PAGE));
                } else {
                    p["ownerUserId"] = json!("other-owner");
                }
                (200, p)
            }
        });
        let b = LocalBotBridge::default();
        let l = connect(&b, &f);
        let r = Request {
            lease_id: l.lease_id,
            method: "GET".into(),
            path: "/v1/adapters/sessions/adapter-a/replies?after=0".into(),
            body: None,
        };
        assert!(runtime().block_on(b.request(r)).is_err());
    }
}
#[test]
fn server_errors_are_sanitized_and_unauthorized_disconnects() {
    let f = Fixture::new(|path, _| {
        if path == "/v1/adapters/catalog" {
            (200, catalog())
        } else {
            (401, json!({"private":"not-public"}))
        }
    });
    let b = LocalBotBridge::default();
    let l = connect(&b, &f);
    let r = runtime().block_on(b.request(registration(&l))).unwrap();
    assert_eq!(r.status, 401);
    assert!(!r.body.contains("not-public"));
    assert!(!b.status().unwrap().connected);
}
#[test]
fn disconnect_fences_late_connect_approval_before_any_http() {
    let f = Fixture::new(|_, _| panic!("no HTTP"));
    let b = LocalBotBridge::default();
    let t = b.begin_connect(f.port).unwrap();
    b.disconnect().unwrap();
    assert!(runtime()
        .block_on(b.finish_connect(t, f.port, true))
        .is_err());
    assert_eq!(f.hits.load(Ordering::SeqCst), 0);
}
#[test]
fn disconnect_fences_inflight_ack_and_does_not_cancel_server_task() {
    let entered = Arc::new(AtomicBool::new(false));
    let e = entered.clone();
    let f = Fixture::new(move |path, _| {
        if path == "/v1/adapters/catalog" {
            (200, catalog())
        } else {
            e.store(true, Ordering::SeqCst);
            thread::sleep(Duration::from_millis(100));
            (200, json!({"session":session(),"duplicate":false}))
        }
    });
    let b = Arc::new(LocalBotBridge::default());
    let l = connect(&b, &f);
    let owned = b.clone();
    let worker = thread::spawn(move || runtime().block_on(owned.request(registration(&l))));
    for _ in 0..200 {
        if entered.load(Ordering::SeqCst) {
            break;
        }
        thread::sleep(Duration::from_millis(2));
    }
    assert!(entered.load(Ordering::SeqCst));
    b.disconnect().unwrap();
    assert!(worker.join().unwrap().is_err());
    assert_eq!(f.hits.load(Ordering::SeqCst), 4);
}
#[test]
fn message_receipt_projection_does_not_export_model_context() {
    let source = Source {
        source_app: "wiii",
        source_session_id: "wiii-source-1".into(),
        source_thread_id: "wiii-thread-1".into(),
        channel_id: "channel-a".into(),
        expected_channel_revision: 2,
        owner_user_id: "local-owner".into(),
        space_id: "local-space".into(),
    };
    let body = json!({"sourceMessageId":"message-a","text":"hello","lane":"foreground","clientNonce":"nonce-0001"});
    let pin = json!({"adapterSessionId":"adapter-a","sourceApp":"wiii","sourceSessionId":"wiii-source-1","sourceThreadId":"wiii-thread-1","sourceMessageId":"message-a","clientNonce":"nonce-0001","channelRevision":2,"returnRoute":"/v1/adapters/sessions/adapter-a/replies"});
    let ack = json!({"duplicate":false,"work":{"requestId":"request-a","clientNonce":"namespaced-nonce","ownerUserId":"local-owner","spaceId":"local-space","conversationId":"channel-a","text":"hello","lane":"foreground","adapterPin":pin,"publication":{"sourceChannelId":"channel-a","replyChannelId":"channel-a","audienceRevision":2},"contextPin":{"private":"model-only-history"}}});
    let output = project_receipt(&ack, &body, &source, "adapter-a").unwrap();
    assert!(output["work"].get("contextPin").is_none());
    assert!(!output.to_string().contains("model-only-history"));
}

#[test]
fn exact_capability_history_get_routes_are_read_only_and_lease_bound() {
    let f = Fixture::with_capabilities(
        Some(json!({"version":1,"capabilities":[HISTORY_CAPABILITY]})),
        |path, _| {
            if path == "/v1/adapters/catalog" {
                (200, catalog())
            } else {
                assert!(path.starts_with("/v1/adapters/sessions/adapter-a/history"));
                (200, history_page())
            }
        },
    );
    let b = LocalBotBridge::default();
    let l = connect(&b, &f);
    assert_eq!(l.capabilities, vec![HISTORY_CAPABILITY]);
    let make = |path: String| Request {
        lease_id: l.lease_id.clone(),
        method: "GET".into(),
        path,
        body: None,
    };
    let path = "/v1/adapters/sessions/adapter-a/history";
    assert_eq!(
        runtime()
            .block_on(b.request(make(path.into())))
            .unwrap()
            .status,
        200
    );
    let token = history_page()["confirmationCursor"]
        .as_str()
        .unwrap()
        .to_owned();
    assert_eq!(
        runtime()
            .block_on(b.request(make(format!("{path}?cursor={token}"))))
            .unwrap()
            .status,
        200
    );
    let before = f.hits.load(Ordering::SeqCst);
    for path in [
        format!("{path}?cursor="),
        format!("{path}?cursor=short"),
        format!("{path}?cursor={token}&cursor={token}"),
        format!("{path}?cursor={token}&other=1"),
        format!("{path}?after=0"),
        path.replace("adapter-a", "%61dapter-a"),
    ] {
        assert!(runtime().block_on(b.request(make(path))).is_err());
    }
    let mut r = make(path.into());
    r.body = Some(json!({}));
    assert!(runtime().block_on(b.request(r)).is_err());
    let mut r = make(path.into());
    r.method = "POST".into();
    assert!(runtime().block_on(b.request(r)).is_err());
    assert_eq!(f.hits.load(Ordering::SeqCst), before);
    b.disconnect().unwrap();
    assert!(runtime().block_on(b.request(make(path.into()))).is_err());
}
#[test]
fn hydrated_message_verifies_history_binding_instead_of_unbounded_delta() {
    let f = Fixture::with_capabilities(
        Some(json!({"version":1,"capabilities":[HISTORY_CAPABILITY]})),
        |path, body| {
            if path == "/v1/adapters/catalog" {
                return (200, catalog());
            }
            if path.ends_with("/history") {
                return (200, history_page());
            }
            assert!(
                path.ends_with("/messages"),
                "No delta after=0 request: {path}"
            );
            (
                200,
                json!({"duplicate":true,"work":{"requestId":"request-a","clientNonce":"namespaced-nonce","ownerUserId":"local-owner","spaceId":"local-space","conversationId":"channel-a","text":body["text"],"lane":body["lane"],
            "adapterPin":{"adapterSessionId":"adapter-a","sourceApp":"wiii","sourceSessionId":"wiii-source-1","sourceThreadId":"wiii-thread-1","sourceMessageId":body["sourceMessageId"],"clientNonce":body["clientNonce"],"channelRevision":2,"returnRoute":"/v1/adapters/sessions/adapter-a/replies"},
            "publication":{"sourceChannelId":"channel-a","replyChannelId":"channel-a","audienceRevision":2},"contextPin":{"private":"never-export"}}}),
            )
        },
    );
    let b = LocalBotBridge::default();
    let l = connect(&b, &f);
    let response = runtime().block_on(b.request(Request { lease_id: l.lease_id, method: "POST".into(), path: "/v1/adapters/sessions/adapter-a/messages".into(),
        body: Some(json!({"sourceMessageId":"message-a","botId":"meiiie","text":"Synthetic original intent","lane":"foreground","clientNonce":"nonce-0001"})) })).unwrap();
    assert_eq!(response.status, 200);
    assert!(!response.body.contains("never-export"));
    assert_eq!(f.hits.load(Ordering::SeqCst), 5);
}
#[test]
fn history_error_forwarding_is_closed_and_status_bound() {
    let safe = rejected_history(409, br#"{"error":"adapter_history_authority_changed"}"#);
    assert!(safe.body.contains("adapter_history_authority_changed"));
    for value in [
        json!({"error":"adapter_history_authority_changed","private":"not-public"}),
        json!({"error":"private-not-allowed"}),
    ] {
        assert_eq!(
            rejected_history(409, &serde_json::to_vec(&value).unwrap()).body,
            rejected(409).body
        );
    }
    assert_eq!(
        rejected_history(500, br#"{"error":"adapter_history_authority_changed"}"#).body,
        rejected(500).body
    );
}
#[test]
fn history_proxy_rejects_unavailable_content_scope_drift_and_out_of_range_offset() {
    let source = Source {
        source_app: "wiii",
        source_session_id: "wiii-source-1".into(),
        source_thread_id: "wiii-thread-1".into(),
        channel_id: "channel-a".into(),
        expected_channel_revision: 2,
        owner_user_id: "local-owner".into(),
        space_id: "local-space".into(),
    };
    assert!(check_history_page(
        &serde_json::to_vec(&history_page()).unwrap(),
        &source,
        "adapter-a"
    )
    .is_ok());
    let mut p = history_page();
    p["ownerUserId"] = json!("other-owner");
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_err());
    let mut p = history_page();
    p["totalRows"] = json!(1);
    p["rows"] = json!([{"replyId":"reply-r","requestId":"r","sourceMessageId":"m","botId":"meiiie","role":"assistant","status":"completed","sequence":1,"text":"must-hide","artifacts":[],"availability":"source_unavailable"}]);
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_err());
    p["rows"][0]["text"] = json!("");
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_ok());
    p["offset"] = json!(1);
    p["complete"] = json!(false);
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_err());
}

#[test]
fn new_history_artifacts_and_utf8_filename_have_frozen_bounds() {
    let source = Source {
        source_app: "wiii",
        source_session_id: "wiii-source-1".into(),
        source_thread_id: "wiii-thread-1".into(),
        channel_id: "channel-a".into(),
        expected_channel_revision: 2,
        owner_user_id: "local-owner".into(),
        space_id: "local-space".into(),
    };
    let files: Vec<_> = (0..8).map(|i| json!({"id":format!("file-{i}"),"filename":format!("{}aa", "猫".repeat(26)),"sha256":"a".repeat(64),"bytes":1})).collect();
    let mut p = history_page();
    p["totalRows"] = json!(1);
    p["rows"] = json!([{"replyId":"reply-r","requestId":"r","sourceMessageId":"m","botId":"meiiie","role":"assistant","status":"completed","sequence":1,"text":"x".repeat(65536),"artifacts":files,"availability":"available"}]);
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_ok());
    p["rows"][0]["artifacts"]
        .as_array_mut()
        .unwrap()
        .push(json!({"id":"ninth","filename":"x","sha256":"a".repeat(64),"bytes":1}));
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_err());
    p["rows"][0]["artifacts"].as_array_mut().unwrap().pop();
    p["rows"][0]["artifacts"][0]["filename"] = json!("猫".repeat(27));
    assert!(check_history_page(&serde_json::to_vec(&p).unwrap(), &source, "adapter-a").is_err());
}
#[test]
#[ignore = "Requires an explicitly owned Go28 loopback fixture; never connects to a user service"]
fn actual_owned_go_catalog_registration_intake_replay_and_disconnect() {
    let port = std::env::var("WIII_NATIVE_OWNED_GO_PORT")
        .expect("owned fixture required")
        .parse::<u16>()
        .unwrap();
    let b = LocalBotBridge::default();
    let r = runtime();
    let status = r
        .block_on(b.finish_connect(b.begin_connect(port).unwrap(), port, true))
        .unwrap();
    let catalog = status.catalog.unwrap();
    let channel = catalog
        .channels
        .iter()
        .find(|c| c.name == "Wiii native fixture")
        .expect("owned channel");
    let link = b
        .link(LinkInput {
            channel_id: channel.id.clone(),
            expected_channel_revision: channel.revision,
            source_session_id: "wiii-native-live".into(),
            source_thread_id: "thread-native-live".into(),
            wiii_project_id: "project-native-local".into(),
        })
        .unwrap();
    let body = json!({"sourceApp":"wiii","sourceSessionId":link.source.source_session_id,"sourceThreadId":link.source.source_thread_id,"channelId":link.source.channel_id,"expectedChannelRevision":link.source.expected_channel_revision,"clientNonce":"native-registration-0001"});
    let registration = r
        .block_on(b.request(Request {
            lease_id: link.lease_id.clone(),
            method: "POST".into(),
            path: "/v1/adapters/sessions".into(),
            body: Some(body),
        }))
        .unwrap();
    let v: Value = serde_json::from_str(&registration.body).unwrap();
    let adapter = v["session"]["id"].as_str().unwrap();
    let message = json!({"sourceMessageId":"native-message-0001","botId":link.bots[0].id,"text":"Owned native Wiii message. No provider or host.","lane":"foreground","clientNonce":"native-message-nonce-0001"});
    let make = || Request {
        lease_id: link.lease_id.clone(),
        method: "POST".into(),
        path: format!("/v1/adapters/sessions/{adapter}/messages"),
        body: Some(message.clone()),
    };
    let first = r.block_on(b.request(make())).unwrap();
    let duplicate = r.block_on(b.request(make())).unwrap();
    let a: Value = serde_json::from_str(&first.body).unwrap();
    let d: Value = serde_json::from_str(&duplicate.body).unwrap();
    assert_eq!(a["work"]["requestId"], d["work"]["requestId"]);
    assert_eq!(d["duplicate"], true);
    assert!(a["work"].get("contextPin").is_none());
    assert!(!first.body.contains("cookie"));
    let replay = || Request {
        lease_id: link.lease_id.clone(),
        method: "GET".into(),
        path: format!("/v1/adapters/sessions/{adapter}/replies?after=0"),
        body: None,
    };
    let page = r.block_on(b.request(replay())).unwrap();
    let p: Value = serde_json::from_str(&page.body).unwrap();
    assert_eq!(p["adapterSessionId"], adapter);
    assert!(p["cursor"].as_u64().unwrap() > 0);
    assert!(!page.body.contains("PRIVATE-UNRELATED-NATIVE-FIXTURE"));
    b.disconnect().unwrap();
    assert!(r.block_on(b.request(replay())).is_err());
    assert!(!b.status().unwrap().connected);
}
