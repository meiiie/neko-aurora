use base64::{engine::general_purpose::STANDARD, Engine};
use serde::{Deserialize, Serialize};
use std::{
    collections::{HashMap, HashSet},
    sync::Mutex,
    time::Duration,
};
use tauri_plugin_http::reqwest::{self, redirect::Policy};

const MAX_BYTES: usize = 64 * 1024 * 1024;
type Result<T> = std::result::Result<T, String>;
fn error(code: &str) -> String {
    format!("wiii_service_http:{code}")
}
fn request_id(x: &str) -> bool {
    !x.is_empty()
        && x.len() <= 160
        && x.bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"._:-".contains(&b))
}
fn safe_segment(segment: &str) -> bool {
    let mut decoded = Vec::with_capacity(segment.len());
    let bytes = segment.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%' {
            let Some(pair) = bytes.get(i + 1..i + 3) else {
                return false;
            };
            let digit = |b: u8| (b as char).to_digit(16);
            let (Some(a), Some(b)) = (digit(pair[0]), digit(pair[1])) else {
                return false;
            };
            decoded.push((a * 16 + b) as u8);
            i += 3;
        } else {
            decoded.push(bytes[i]);
            i += 1;
        }
    }
    let Ok(value) = std::str::from_utf8(&decoded) else {
        return false;
    };
    !value.is_empty()
        && !matches!(value, "." | "..")
        && !value.chars().any(|c| {
            c.is_control() || matches!(c, '/' | '\\' | '%' | '?' | '#' | '\u{2028}' | '\u{2029}')
        })
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ServiceRequest {
    pub request_id: String,
    pub port: Option<u16>,
    pub remote: bool,
    pub path: String,
    pub method: String,
    pub headers: Vec<(String, String)>,
    pub body_base64: Option<String>,
    pub timeout_ms: u64,
}
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum ServiceEvent {
    Headers {
        status: u16,
        headers: Vec<(String, String)>,
    },
    Chunk {
        bytes: Vec<u8>,
    },
    Done,
}
fn target(request: &ServiceRequest) -> Result<String> {
    if !request_id(&request.request_id)
        || request.path.len() > 8192
        || request.path.contains(['#', '\\', '\r', '\n'])
        || !(1..=900_000).contains(&request.timeout_ms)
        || !matches!(
            request.method.as_str(),
            "GET" | "POST" | "PUT" | "PATCH" | "DELETE" | "HEAD" | "OPTIONS"
        )
    {
        return Err(error("invalid_request"));
    }
    let path = request.path.split('?').next().unwrap_or("");
    if path != "/health" && !path.starts_with("/api/v1/")
        || path.split('/').skip(1).any(|p| !safe_segment(p))
    {
        return Err(error("route_denied"));
    }
    if request.headers.len() > 32
        || request
            .headers
            .iter()
            .map(|(k, v)| k.len() + v.len())
            .sum::<usize>()
            > 32768
    {
        return Err(error("invalid_headers"));
    }
    let mut seen = HashSet::new();
    for (name, value) in &request.headers {
        let name = name.to_ascii_lowercase();
        if !matches!(
            name.as_str(),
            "authorization"
                | "x-api-key"
                | "x-user-id"
                | "x-organization-id"
                | "x-session-id"
                | "x-request-id"
                | "content-type"
                | "accept"
                | "accept-language"
        ) || !seen.insert(name)
            || value.contains(['\r', '\n', '\0'])
        {
            return Err(error("header_denied"));
        }
    }
    let origin = if request.remote {
        if request.port.is_some() {
            return Err(error("invalid_origin"));
        }
        "https://wiii.holilihu.online".to_owned()
    } else {
        format!(
            "http://127.0.0.1:{}",
            request
                .port
                .filter(|p| *p > 0)
                .ok_or_else(|| error("invalid_port"))?
        )
    };
    let target = format!("{origin}{}", request.path);
    let parsed = reqwest::Url::parse(&target).map_err(|_| error("invalid_path"))?;
    if parsed.path() != path || parsed.fragment().is_some() {
        return Err(error("route_denied"));
    }
    Ok(target)
}
pub async fn run_service(
    request: ServiceRequest,
    sink: impl Fn(ServiceEvent) -> Result<()>,
) -> Result<()> {
    let url = target(&request)?;
    let body = match request.body_base64 {
        Some(body) => {
            if body.len() > (MAX_BYTES + 2) / 3 * 4 {
                return Err(error("body_oversized"));
            }
            let decoded = STANDARD.decode(body).map_err(|_| error("invalid_body"))?;
            if decoded.len() > MAX_BYTES {
                return Err(error("body_oversized"));
            }
            Some(decoded)
        }
        None => None,
    };
    if body.is_some() && matches!(request.method.as_str(), "GET" | "HEAD") {
        return Err(error("body_denied"));
    }
    let client = reqwest::Client::builder()
        .no_proxy()
        .redirect(Policy::none())
        .connect_timeout(Duration::from_secs(3))
        .build()
        .map_err(|_| error("transport_unavailable"))?;
    let method = reqwest::Method::from_bytes(request.method.as_bytes())
        .map_err(|_| error("invalid_method"))?;
    let mut builder = client.request(method, url);
    for (key, value) in request.headers {
        builder = builder.header(key, value);
    }
    if let Some(body) = body {
        builder = builder.body(body);
    }
    let mut response =
        tokio::time::timeout(Duration::from_millis(request.timeout_ms), builder.send())
            .await
            .map_err(|_| error("header_timeout"))?
            .map_err(|_| error("transport_failed"))?;
    if response.status().is_redirection() {
        return Err(error("redirect_denied"));
    }
    if response
        .content_length()
        .is_some_and(|n| n > MAX_BYTES as u64)
    {
        return Err(error("response_oversized"));
    }
    let headers = response
        .headers()
        .iter()
        .filter_map(|(key, value)| {
            let name = key.as_str();
            if matches!(
                name,
                "content-type"
                    | "content-length"
                    | "content-disposition"
                    | "cache-control"
                    | "etag"
                    | "x-request-id"
                    | "x-trace-id"
            ) {
                value
                    .to_str()
                    .ok()
                    .map(|value| (name.to_owned(), value.to_owned()))
            } else {
                None
            }
        })
        .collect();
    sink(ServiceEvent::Headers {
        status: response.status().as_u16(),
        headers,
    })?;
    let mut total = 0;
    while let Some(bytes) = response.chunk().await.map_err(|_| error("stream_failed"))? {
        total += bytes.len();
        if total > MAX_BYTES {
            return Err(error("response_oversized"));
        }
        for bytes in bytes.chunks(16384) {
            sink(ServiceEvent::Chunk {
                bytes: bytes.to_vec(),
            })?;
        }
    }
    sink(ServiceEvent::Done)
}
#[derive(Default)]
pub struct ServiceHttp {
    state: Mutex<ServiceState>,
}
#[derive(Default)]
struct ServiceState {
    active: HashMap<String, tokio::task::AbortHandle>,
    cancelled: HashSet<String>,
    cancel_saturated: bool,
}
impl ServiceHttp {
    pub fn cancel(&self, id: &str) -> Result<()> {
        if !request_id(id) {
            return Err(error("invalid_request_id"));
        }
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if let Some(task) = s.active.get(id) {
            task.abort();
        } else if !s.cancelled.contains(id) {
            if s.cancelled.len() >= 256 {
                s.cancel_saturated = true;
                return Err(error("cancel_capacity"));
            }
            s.cancelled.insert(id.to_owned());
        }
        Ok(())
    }
    pub async fn fetch(
        &self,
        request: ServiceRequest,
        sink: impl Fn(ServiceEvent) -> Result<()> + Send + 'static,
    ) -> Result<()> {
        target(&request)?;
        let id = request.request_id.clone();
        let task = {
            let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
            if s.cancel_saturated {
                return Err(error("cancel_capacity"));
            }
            if s.cancelled.remove(&id) {
                return Err(error("cancelled"));
            }
            if s.active.len() >= 32 || s.active.contains_key(&id) {
                return Err(error("request_pending"));
            }
            let task = tokio::spawn(async move { run_service(request, sink).await });
            s.active.insert(id.clone(), task.abort_handle());
            task
        };
        let result = task.await.map_err(|_| error("cancelled"));
        self.state
            .lock()
            .map_err(|_| error("state_unavailable"))?
            .active
            .remove(&id);
        result?
    }
}

#[cfg(test)]
mod tests;
