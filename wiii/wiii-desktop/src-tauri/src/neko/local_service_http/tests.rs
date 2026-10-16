use super::*;
use std::{
    io::{Read, Write},
    net::TcpListener,
    sync::{
        atomic::{AtomicUsize, Ordering},
        Arc,
    },
    thread,
};
fn runtime() -> tokio::runtime::Runtime {
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .unwrap()
}
fn request(port: u16, path: &str) -> ServiceRequest {
    ServiceRequest {
        request_id: uuid::Uuid::new_v4().to_string(),
        port: Some(port),
        remote: false,
        path: path.into(),
        method: "GET".into(),
        headers: vec![],
        body_base64: None,
        timeout_ms: 60000,
    }
}
fn fixture(response: Vec<u8>) -> (u16, Arc<AtomicUsize>, thread::JoinHandle<Vec<u8>>) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let count = Arc::new(AtomicUsize::new(0));
    let c = count.clone();
    let worker = thread::spawn(move || {
        let (mut stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(Duration::from_secs(2)))
            .unwrap();
        let mut bytes = Vec::new();
        let mut part = [0; 4096];
        let end = loop {
            let n = stream.read(&mut part).unwrap();
            assert!(n > 0);
            bytes.extend_from_slice(&part[..n]);
            if let Some(i) = bytes.windows(4).position(|p| p == b"\r\n\r\n") {
                break i + 4;
            }
        };
        let headers = String::from_utf8(bytes[..end].to_vec()).unwrap();
        let length = headers
            .lines()
            .find_map(|l| {
                l.to_ascii_lowercase()
                    .strip_prefix("content-length:")
                    .and_then(|s| s.trim().parse::<usize>().ok())
            })
            .unwrap_or(0);
        while bytes.len() < end + length {
            let n = stream.read(&mut part).unwrap();
            assert!(n > 0);
            bytes.extend_from_slice(&part[..n]);
        }
        c.fetch_add(1, Ordering::SeqCst);
        let _ = stream.write_all(&response);
        bytes
    });
    (port, count, worker)
}
#[test]
fn denies_neko_routes_arbitrary_origin_and_encoded_path_before_http() {
    for path in [
        "/",
        "/v1/adapters/catalog",
        "/v1/messages",
        "/api/v1/../..",
        "/api/v1/%2e%2e/",
        "/api/v1/foo%2f..%2f..",
        "//api/v1/x",
        "/api/v1//x",
        "/health/",
        "/health#x",
        "/api/v1/x/%2E%2e/y",
        "/api/v1/x/%252e%252e/y",
        "/api/v1/x/a%2Fb",
        "/api/v1/x/a%5cb",
        "/api/v1/x/a%3Fb",
        "/api/v1/x/a%23b",
        "/api/v1/x/a%00b",
        "/api/v1/x/a%C2%80b",
        "/api/v1/x/%FF",
        "/api/v1/x/%C0%AF",
        "/api/v1/x/%",
        "/api/v1/x/%1",
    ] {
        assert!(target(&request(8765, path)).is_err(), "{path}");
    }
    let mut r = request(0, "/health");
    assert!(target(&r).is_err());
    r.port = Some(8765);
    r.remote = true;
    assert!(target(&r).is_err());
    r.port = None;
    assert_eq!(target(&r).unwrap(), "https://wiii.holilihu.online/health");
    let mut r = request(8765, "/api/v1/chat");
    r.headers
        .push(("Cookie".into(), "neko_local=fixture-only".into()));
    assert!(target(&r).is_err());
    r.headers = vec![("Host".into(), "example.com".into())];
    assert!(target(&r).is_err());
    r.headers = vec![("Origin".into(), "http://127.0.0.1:8765".into())];
    assert!(target(&r).is_err());
}
#[test]
fn binary_multipart_bytes_and_existing_headers_pass_without_cookie_storage() {
    let response=b"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 4\r\nSet-Cookie: neko_local=fixture-only\r\nConnection: close\r\n\r\n\x00\x01\xfe\xff".to_vec();
    let (port, count, worker) = fixture(response);
    let multipart=b"--fixture-boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.bin\"\r\n\r\n\x00\x01\xfe\xff\r\n--fixture-boundary--\r\n";
    let mut r = request(port, "/api/v1/document-context/parse");
    r.method = "POST".into();
    r.timeout_ms = 900000;
    r.body_base64 = Some(STANDARD.encode(multipart));
    r.headers = vec![
        (
            "Content-Type".into(),
            "multipart/form-data; boundary=fixture-boundary".into(),
        ),
        ("X-Session-ID".into(), "fixture-session".into()),
    ];
    let events = Mutex::new(Vec::new());
    runtime()
        .block_on(run_service(r, |e| {
            events.lock().unwrap().push(e);
            Ok(())
        }))
        .unwrap();
    let bytes = worker.join().unwrap();
    assert!(bytes.ends_with(multipart));
    assert_eq!(count.load(Ordering::SeqCst), 1);
    let headers =
        String::from_utf8_lossy(&bytes[..bytes.windows(4).position(|w| w == b"\r\n\r\n").unwrap()]);
    assert!(!headers.to_ascii_lowercase().contains("cookie:"));
    assert!(headers
        .to_ascii_lowercase()
        .contains("x-session-id: fixture-session"));
    let events = events.into_inner().unwrap();
    assert!(matches!(
        events.first(),
        Some(ServiceEvent::Headers { status: 200, .. })
    ));
    assert!(matches!(events.last(), Some(ServiceEvent::Done)));
    let output = serde_json::to_string(&events).unwrap();
    assert!(!output.to_ascii_lowercase().contains("set-cookie"));
    assert_eq!(
        events
            .iter()
            .filter_map(|e| if let ServiceEvent::Chunk { bytes } = e {
                Some(bytes.clone())
            } else {
                None
            })
            .flatten()
            .collect::<Vec<_>>(),
        vec![0, 1, 254, 255]
    );
}
#[test]
fn redirect_to_neko_bootstrap_is_denied_without_following() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let protected = listener.local_addr().unwrap().port();
    let response=format!("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:{protected}/\r\nContent-Length: 0\r\nConnection: close\r\n\r\n").into_bytes();
    let (port, _, worker) = fixture(response);
    let result = runtime().block_on(run_service(request(port, "/health"), |_| {
        panic!("redirect must not publish headers")
    }));
    assert_eq!(result.err().unwrap(), error("redirect_denied"));
    worker.join().unwrap();
    assert!(listener.accept().is_err());
}
#[test]
fn already_cancelled_request_never_opens_http() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let port = listener.local_addr().unwrap().port();
    let state = ServiceHttp::default();
    let r = request(port, "/health");
    state.cancel(&r.request_id).unwrap();
    assert_eq!(
        runtime()
            .block_on(state.fetch(r, |_| Ok(())))
            .err()
            .unwrap(),
        error("cancelled")
    );
    assert!(listener.accept().is_err());
}
#[test]
fn sse_headers_and_bytes_are_emitted_as_a_stream() {
    let bytes = b"event: delta\ndata: {\"text\":\"fixture\"}\n\n";
    let mut response=format!("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",bytes.len()).into_bytes();
    response.extend_from_slice(bytes);
    let (port, _, worker) = fixture(response);
    let events = Mutex::new(Vec::new());
    runtime()
        .block_on(run_service(
            request(port, "/api/v1/chat/stream/v3"),
            |event| {
                events.lock().unwrap().push(event);
                Ok(())
            },
        ))
        .unwrap();
    worker.join().unwrap();
    let events = events.into_inner().unwrap();
    assert!(matches!(
        events.first(),
        Some(ServiceEvent::Headers { status: 200, .. })
    ));
    assert_eq!(
        events
            .iter()
            .filter_map(|e| if let ServiceEvent::Chunk { bytes } = e {
                Some(bytes.clone())
            } else {
                None
            })
            .flatten()
            .collect::<Vec<_>>(),
        bytes
    );
}
#[test]
fn capability_removes_generic_loopback_and_plugin_registration() {
    let capability: serde_json::Value =
        serde_json::from_str(include_str!("../../../capabilities/workbench-network.json")).unwrap();
    let text = capability.to_string();
    assert!(!text.contains("http://localhost") && !text.contains("http://127.0.0.1"));
    assert!(text.contains("https://wiii.holilihu.online/**"));
    assert_eq!(capability["windows"], serde_json::json!(["main"]));
    let lib = include_str!("../../lib.rs");
    assert!(!lib.contains(".plugin(tauri_plugin_http::init())"));
    let conf: serde_json::Value =
        serde_json::from_str(include_str!("../../../tauri.conf.json")).unwrap();
    let connect = conf["app"]["security"]["csp"]["connect-src"]
        .as_str()
        .unwrap();
    assert!(!connect.contains("http://localhost:") && !connect.contains("http://127.0.0.1:"));
    assert!(connect.contains("ws://127.0.0.1:"));
}

#[test]
fn in_flight_sse_cancel_closes_only_the_requested_stream() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let worker = thread::spawn(move || {
        let (mut stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(Duration::from_secs(3)))
            .unwrap();
        let mut part = [0; 4096];
        let mut request = Vec::new();
        while !request.windows(4).any(|x| x == b"\r\n\r\n") {
            let n = stream.read(&mut part).unwrap();
            assert!(n > 0);
            request.extend_from_slice(&part[..n]);
        }
        stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\ndata: first\n\n").unwrap();
        matches!(stream.read(&mut part), Ok(0))
    });
    runtime().block_on(async {
        let state = Arc::new(ServiceHttp::default());
        let r = request(port, "/api/v1/chat/stream/v3");
        let id = r.request_id.clone();
        let ready = Arc::new(tokio::sync::Notify::new());
        let notify = ready.clone();
        let service = state.clone();
        let pending = tokio::spawn(async move {
            service
                .fetch(r, move |event| {
                    if matches!(event, ServiceEvent::Chunk { .. }) {
                        notify.notify_one();
                    }
                    Ok(())
                })
                .await
        });
        tokio::time::timeout(Duration::from_secs(2), ready.notified())
            .await
            .unwrap();
        let (other_port, _, other) = fixture(
            b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok".to_vec(),
        );
        state
            .fetch(request(other_port, "/health"), |_| Ok(()))
            .await
            .unwrap();
        other.join().unwrap();
        state.cancel(&id).unwrap();
        assert_eq!(pending.await.unwrap().err().unwrap(), error("cancelled"));
        assert!(state.state.lock().unwrap().active.is_empty());
    });
    assert!(
        worker.join().unwrap(),
        "request cancellation must close its pending HTTP stream"
    );
}

#[test]
fn oversized_response_headers_fail_before_channel_publication() {
    let response = format!(
        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        MAX_BYTES + 1
    )
    .into_bytes();
    let (port, _, worker) = fixture(response);
    assert_eq!(
        runtime()
            .block_on(run_service(request(port, "/health"), |_| {
                panic!("oversized response must not publish headers")
            }))
            .err()
            .unwrap(),
        error("response_oversized")
    );
    worker.join().unwrap();
}

#[test]
fn delayed_headers_obey_the_existing_bounded_timeout() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let worker = thread::spawn(move || {
        let (mut stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(Duration::from_secs(2)))
            .unwrap();
        let mut request = [0; 4096];
        assert!(stream.read(&mut request).unwrap() > 0);
        thread::sleep(Duration::from_millis(80));
        let _ = stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n");
    });
    let mut r = request(port, "/health");
    r.timeout_ms = 20;
    assert_eq!(
        runtime()
            .block_on(run_service(r, |_| {
                panic!("timed-out headers must not publish")
            }))
            .err()
            .unwrap(),
        error("header_timeout")
    );
    worker.join().unwrap();
}

#[test]
fn exhausted_cancel_race_capacity_fails_closed_before_http() {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    listener.set_nonblocking(true).unwrap();
    let port = listener.local_addr().unwrap().port();
    let state = ServiceHttp::default();
    for i in 0..256 {
        state.cancel(&format!("cancel-race-{i}")).unwrap();
    }
    assert_eq!(
        state.cancel("cancel-race-overflow").err().unwrap(),
        error("cancel_capacity")
    );
    assert_eq!(
        runtime()
            .block_on(state.fetch(request(port, "/health"), |_| Ok(())))
            .err()
            .unwrap(),
        error("cancel_capacity")
    );
    assert!(listener.accept().is_err());
    assert_eq!(state.state.lock().unwrap().cancelled.len(), 256);
}

#[test]
fn encoded_unicode_and_colon_ids_reach_the_exact_validated_api_path() {
    let path = "/api/v1/wiii-connect/providers/fixture/connections/link%3A%C4%91%E1%BB%99i";
    let (port, _, worker) =
        fixture(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok".to_vec());
    runtime()
        .block_on(run_service(request(port, path), |_| Ok(())))
        .unwrap();
    let observed = worker.join().unwrap();
    assert!(observed.starts_with(format!("GET {path} HTTP/1.1\r\n").as_bytes()));
}
