use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{
    collections::{HashMap, HashSet},
    sync::Mutex,
    time::Duration,
};
use tauri_plugin_http::reqwest::{self, header, redirect::Policy};

const MAX_PAGE: usize = 1_048_576;
const MAX_HISTORY_PAGE: usize = 900_000;
const HISTORY_CAPABILITY: &str = "local_adapter_history_repair_v1";
const MAX_BODY: usize = 262_144;
const MAX_SAFE: u64 = 9_007_199_254_740_991;
type Result<T> = std::result::Result<T, String>;
fn error(code: &str) -> String {
    format!("neko_bot_local:{code}")
}
fn id(x: &str) -> bool {
    !x.is_empty()
        && x.len() <= 160
        && x.bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"._:-".contains(&b))
}
fn rev(x: u64) -> bool {
    x > 0 && x <= MAX_SAFE
}
fn route_id(x: &str) -> bool {
    id(x) && !matches!(x, "." | "..")
}
fn exact(x: &Value, required: &[&str], optional: &[&str]) -> Result<()> {
    let m = x.as_object().ok_or_else(|| error("invalid_object"))?;
    if required.iter().any(|k| !m.contains_key(*k))
        || m.keys()
            .any(|k| !required.contains(&k.as_str()) && !optional.contains(&k.as_str()))
    {
        return Err(error("unsupported_fields"));
    }
    Ok(())
}
fn string_id(x: &Value) -> bool {
    x.as_str().is_some_and(id)
}
fn nonce(x: &Value) -> bool {
    x.as_str()
        .is_some_and(|s| id(s) && (8..=128).contains(&s.len()))
}
fn history_token(s: &str) -> bool {
    s.len() == 215
        && URL_SAFE_NO_PAD.decode(s).is_ok_and(|bytes| {
            bytes.len() == 161 && bytes[0] == 1 && URL_SAFE_NO_PAD.encode(&bytes) == s
        })
}

#[derive(Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CatalogBot {
    pub id: String,
    pub name: String,
}
#[derive(Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CatalogChannel {
    pub id: String,
    pub name: String,
    pub revision: u64,
    pub bot_ids: Vec<String>,
}
#[derive(Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Catalog {
    pub version: u8,
    pub owner_user_id: String,
    pub space_id: String,
    pub channels: Vec<CatalogChannel>,
    pub bots: Vec<CatalogBot>,
    #[serde(default)]
    pub capabilities: Vec<String>,
}
impl Catalog {
    fn validate(&self) -> Result<()> {
        let ids: HashSet<_> = self.bots.iter().map(|b| b.id.as_str()).collect();
        let channels: HashSet<_> = self.channels.iter().map(|c| c.id.as_str()).collect();
        if self.version != 1
            || !id(&self.owner_user_id)
            || !id(&self.space_id)
            || self.channels.len() > 256
            || self.bots.len() > 256
            || !valid_capabilities(&self.capabilities)
            || ids.len() != self.bots.len()
            || channels.len() != self.channels.len()
            || self
                .bots
                .iter()
                .any(|b| !id(&b.id) || b.name.is_empty() || b.name.len() > 512)
            || self.channels.iter().any(|c| {
                !id(&c.id)
                    || !rev(c.revision)
                    || c.name.is_empty()
                    || c.name.len() > 512
                    || c.bot_ids.is_empty()
                    || c.bot_ids.len() > 256
                    || c.bot_ids.iter().any(|b| !ids.contains(b.as_str()))
                    || c.bot_ids.iter().collect::<HashSet<_>>().len() != c.bot_ids.len()
            })
        {
            return Err(error("invalid_catalog"));
        }
        Ok(())
    }
}
#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Source {
    pub source_app: &'static str,
    pub source_session_id: String,
    pub source_thread_id: String,
    pub channel_id: String,
    pub expected_channel_revision: u64,
    pub owner_user_id: String,
    pub space_id: String,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct LinkInput {
    pub channel_id: String,
    pub expected_channel_revision: u64,
    pub source_session_id: String,
    pub source_thread_id: String,
    pub wiii_project_id: String,
}
#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Link {
    pub lease_id: String,
    pub source: Source,
    pub project_id: String,
    pub channel_name: String,
    pub bots: Vec<CatalogBot>,
    pub capabilities: Vec<String>,
}
#[derive(Clone)]
struct Lease {
    link: Link,
    adapter_ids: HashSet<String>,
}
#[derive(Clone)]
struct Connection {
    client: reqwest::Client,
    port: u16,
    cookie: String,
    catalog: Catalog,
}
#[derive(Default)]
struct State {
    generation: u64,
    connecting: bool,
    connection: Option<Connection>,
    leases: HashMap<String, Lease>,
}
#[derive(Default)]
pub struct LocalBotBridge {
    state: Mutex<State>,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LocalStatus {
    pub connected: bool,
    pub connecting: bool,
    pub catalog: Option<Catalog>,
}
#[derive(Serialize)]
pub struct Response {
    pub status: u16,
    pub body: String,
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Request {
    pub lease_id: String,
    pub method: String,
    pub path: String,
    pub body: Option<Value>,
}

pub fn trusted_surface(label: &str, url: &reqwest::Url, allow_dev: bool) -> bool {
    if label != "main" || !url.username().is_empty() || url.password().is_some() {
        return false;
    }
    match (url.scheme(), url.host_str(), url.port()) {
        ("tauri", Some("localhost"), None) | ("http", Some("tauri.localhost"), None) => true,
        ("http", Some("localhost"), Some(1420)) => allow_dev,
        _ => false,
    }
}

impl LocalBotBridge {
    pub fn status(&self) -> Result<LocalStatus> {
        let s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        Ok(LocalStatus {
            connected: s.connection.is_some(),
            connecting: s.connecting,
            catalog: s.connection.as_ref().map(|c| c.catalog.clone()),
        })
    }
    pub fn disconnect(&self) -> Result<()> {
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        s.generation = s
            .generation
            .checked_add(1)
            .ok_or_else(|| error("generation_exhausted"))?;
        s.connecting = false;
        s.connection = None;
        s.leases.clear();
        Ok(())
    }
    pub fn begin_connect(&self, port: u16) -> Result<u64> {
        if port == 0 {
            return Err(error("invalid_port"));
        }
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if s.connecting {
            return Err(error("connect_pending"));
        }
        s.generation = s
            .generation
            .checked_add(1)
            .ok_or_else(|| error("generation_exhausted"))?;
        s.connecting = true;
        s.connection = None;
        s.leases.clear();
        Ok(s.generation)
    }
    pub fn abort_connect(&self, generation: u64) -> Result<()> {
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if s.generation != generation {
            return Err(error("connection_changed"));
        }
        s.generation = s
            .generation
            .checked_add(1)
            .ok_or_else(|| error("generation_exhausted"))?;
        s.connecting = false;
        s.connection = None;
        s.leases.clear();
        Ok(())
    }
    fn current(&self, generation: u64) -> Result<()> {
        if self
            .state
            .lock()
            .map_err(|_| error("state_unavailable"))?
            .generation
            != generation
        {
            return Err(error("connection_changed"));
        }
        Ok(())
    }
    pub async fn finish_connect(
        &self,
        generation: u64,
        port: u16,
        owner_confirmed: bool,
    ) -> Result<LocalStatus> {
        self.current(generation)?;
        if !owner_confirmed {
            self.abort_connect(generation)?;
            return Err(error("owner_declined"));
        }
        let outcome = self.bootstrap(generation, port).await;
        if outcome.is_err() {
            let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
            if s.generation == generation {
                s.connecting = false;
                s.connection = None;
                s.leases.clear();
            }
        }
        outcome
    }
    async fn bootstrap(&self, generation: u64, port: u16) -> Result<LocalStatus> {
        self.current(generation)?;
        let client = reqwest::Client::builder()
            .no_proxy()
            .redirect(Policy::none())
            .connect_timeout(Duration::from_secs(3))
            .timeout(Duration::from_secs(5))
            .build()
            .map_err(|_| error("transport_unavailable"))?;
        let response = client
            .get(format!("http://127.0.0.1:{port}/"))
            .send()
            .await
            .map_err(|_| error("connect_failed"))?;
        self.current(generation)?;
        if response.status().as_u16() != 200 {
            return Err(error("bootstrap_rejected"));
        }
        let cookies: Vec<_> = response
            .headers()
            .get_all(header::SET_COOKIE)
            .iter()
            .filter_map(|h| h.to_str().ok())
            .filter(|s| s.starts_with("neko_local="))
            .collect();
        if cookies.len() != 1 {
            return Err(error("bootstrap_cookie_invalid"));
        }
        let parts: Vec<_> = cookies[0].split(';').map(str::trim).collect();
        let value = parts[0]
            .strip_prefix("neko_local=")
            .ok_or_else(|| error("bootstrap_cookie_invalid"))?;
        if value.len() != 64
            || !value.bytes().all(|b| b.is_ascii_hexdigit())
            || !parts.iter().any(|s| s.eq_ignore_ascii_case("HttpOnly"))
            || !parts
                .iter()
                .any(|s| s.eq_ignore_ascii_case("SameSite=Strict"))
            || !parts.contains(&"Path=/")
            || parts
                .iter()
                .any(|s| s.to_ascii_lowercase().starts_with("domain="))
        {
            return Err(error("bootstrap_cookie_invalid"));
        }
        let cookie = format!("neko_local={value}");
        drop(response);
        self.current(generation)?;
        let response = client
            .get(format!("http://127.0.0.1:{port}/v1/adapters/catalog"))
            .header(header::COOKIE, &cookie)
            .send()
            .await
            .map_err(|_| error("catalog_unavailable"))?;
        if response.status().as_u16() != 200 {
            return Err(error("catalog_rejected"));
        }
        let bytes = read_json(response).await?;
        let mut catalog: Catalog =
            serde_json::from_slice(&bytes).map_err(|_| error("invalid_catalog"))?;
        catalog.validate()?;
        let capabilities = client
            .get(format!("http://127.0.0.1:{port}/v1/adapters/capabilities"))
            .header(header::COOKIE, &cookie)
            .send()
            .await
            .map_err(|_| error("capabilities_unavailable"))?;
        self.current(generation)?;
        if capabilities.status().as_u16() == 200 {
            catalog.capabilities = check_capabilities(&read_json(capabilities).await?)?;
        } else if capabilities.status().as_u16() != 404 {
            return Err(error("capabilities_rejected"));
        } else {
            catalog.capabilities.clear();
        }
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if s.generation != generation || !s.connecting {
            return Err(error("connection_changed"));
        }
        s.connection = Some(Connection {
            client,
            port,
            cookie,
            catalog: catalog.clone(),
        });
        s.connecting = false;
        Ok(LocalStatus {
            connected: true,
            connecting: false,
            catalog: Some(catalog),
        })
    }
    pub fn link(&self, input: LinkInput) -> Result<Link> {
        if !id(&input.channel_id)
            || !id(&input.source_session_id)
            || !id(&input.source_thread_id)
            || !id(&input.wiii_project_id)
            || !rev(input.expected_channel_revision)
        {
            return Err(error("invalid_binding"));
        }
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if s.leases.len() >= 32 {
            return Err(error("link_limit"));
        }
        let c = s.connection.as_ref().ok_or_else(|| error("disconnected"))?;
        let channel = c
            .catalog
            .channels
            .iter()
            .find(|c| c.id == input.channel_id && c.revision == input.expected_channel_revision)
            .ok_or_else(|| error("catalog_binding_changed"))?;
        if let Some(existing) = s.leases.values().find(|l| {
            l.link.project_id == input.wiii_project_id
                && l.link.source.source_session_id == input.source_session_id
                && l.link.source.source_thread_id == input.source_thread_id
                && l.link.source.channel_id == input.channel_id
                && l.link.source.expected_channel_revision == input.expected_channel_revision
        }) {
            return Ok(existing.link.clone());
        }
        let link = Link {
            lease_id: uuid::Uuid::new_v4().to_string(),
            project_id: input.wiii_project_id,
            channel_name: channel.name.clone(),
            bots: c
                .catalog
                .bots
                .iter()
                .filter(|b| channel.bot_ids.contains(&b.id))
                .cloned()
                .collect(),
            capabilities: c.catalog.capabilities.clone(),
            source: Source {
                source_app: "wiii",
                source_session_id: input.source_session_id,
                source_thread_id: input.source_thread_id,
                channel_id: channel.id.clone(),
                expected_channel_revision: channel.revision,
                owner_user_id: c.catalog.owner_user_id.clone(),
                space_id: c.catalog.space_id.clone(),
            },
        };
        s.leases.insert(
            link.lease_id.clone(),
            Lease {
                link: link.clone(),
                adapter_ids: HashSet::new(),
            },
        );
        Ok(link)
    }
    pub async fn request(&self, input: Request) -> Result<Response> {
        let (generation, connection, lease) = {
            let s = self.state.lock().map_err(|_| error("state_unavailable"))?;
            (
                s.generation,
                s.connection.clone().ok_or_else(|| error("disconnected"))?,
                s.leases
                    .get(&input.lease_id)
                    .cloned()
                    .ok_or_else(|| error("unknown_link"))?,
            )
        };
        let route = Route::parse(&input, &lease.link)?;
        // A hydrated adapter ID is provenance until its actual server reply proves the complete binding.
        if let Route::Message(ref adapter) = route {
            if !lease.adapter_ids.contains(adapter) {
                let supports_history = lease
                    .link
                    .capabilities
                    .iter()
                    .any(|c| c == HISTORY_CAPABILITY);
                let verify_path = if supports_history {
                    format!("/v1/adapters/sessions/{adapter}/history")
                } else {
                    format!("/v1/adapters/sessions/{adapter}/replies?after=0")
                };
                let verify = self.http(&connection, "GET", &verify_path, None).await?;
                self.current(generation)?;
                if verify.0 != 200 {
                    return Ok(if supports_history {
                        rejected_history(verify.0, &verify.1)
                    } else {
                        rejected(verify.0)
                    });
                }
                if supports_history {
                    check_history_page(&verify.1, &lease.link.source, adapter)?;
                } else {
                    check_page(&verify.1, &lease.link.source, adapter, 0)?;
                }
            }
        }
        self.current(generation)?;
        let (status, bytes) = self
            .http(&connection, &input.method, &input.path, input.body.as_ref())
            .await?;
        self.current(generation)?;
        if !(200..300).contains(&status) {
            if status == 401 {
                self.abort_connect(generation)?;
            }
            return Ok(if matches!(route, Route::History(_)) {
                rejected_history(status, &bytes)
            } else {
                rejected(status)
            });
        }
        let value: Value = serde_json::from_slice(&bytes).map_err(|_| error("invalid_response"))?;
        let (output, adapter) = match route {
            Route::Capabilities => {
                check_capabilities(&bytes)?;
                (value, None)
            }
            Route::Register => {
                exact(&value, &["session", "duplicate"], &[])?;
                if !value["duplicate"].is_boolean() {
                    return Err(error("invalid_receipt"));
                }
                let adapter = value["session"]["id"]
                    .as_str()
                    .filter(|s| route_id(s))
                    .ok_or_else(|| error("invalid_receipt"))?;
                check_session(&value["session"], &lease.link.source, adapter)?;
                (value.clone(), Some(adapter.to_owned()))
            }
            Route::Replies(adapter, after) => {
                check_page(&bytes, &lease.link.source, &adapter, after)?;
                (value, Some(adapter))
            }
            Route::History(adapter) => {
                check_history_page(&bytes, &lease.link.source, &adapter)?;
                (value, Some(adapter))
            }
            Route::Message(adapter) => (
                project_receipt(
                    &value,
                    input.body.as_ref().ok_or_else(|| error("invalid_body"))?,
                    &lease.link.source,
                    &adapter,
                )?,
                Some(adapter),
            ),
        };
        let mut s = self.state.lock().map_err(|_| error("state_unavailable"))?;
        if s.generation != generation {
            return Err(error("connection_changed"));
        }
        let current_lease = s
            .leases
            .get_mut(&input.lease_id)
            .ok_or_else(|| error("unknown_link"))?;
        if let Some(adapter) = adapter {
            current_lease.adapter_ids.insert(adapter);
        }
        Ok(Response {
            status,
            body: serde_json::to_string(&output).map_err(|_| error("invalid_response"))?,
        })
    }
    async fn http(
        &self,
        c: &Connection,
        method: &str,
        path: &str,
        body: Option<&Value>,
    ) -> Result<(u16, Vec<u8>)> {
        let url = format!("http://127.0.0.1:{}{path}", c.port);
        let parsed = reqwest::Url::parse(&url).map_err(|_| error("invalid_route"))?;
        let (expected_path, expected_query) = path
            .split_once('?')
            .map_or((path, None), |(p, q)| (p, Some(q)));
        if parsed.path() != expected_path
            || parsed.query() != expected_query
            || parsed.fragment().is_some()
        {
            return Err(error("invalid_route"));
        }
        let builder = if method == "POST" {
            c.client
                .post(parsed)
                .header(header::CONTENT_TYPE, "application/json")
                .body(
                    serde_json::to_vec(body.ok_or_else(|| error("invalid_body"))?)
                        .map_err(|_| error("invalid_body"))?,
                )
        } else {
            c.client.get(parsed)
        };
        let response = builder
            .header(header::COOKIE, &c.cookie)
            .send()
            .await
            .map_err(|_| error("transport_failed"))?;
        let status = response.status().as_u16();
        if !(200..300).contains(&status) {
            let bytes = if path.contains("/history") {
                read_json(response).await.unwrap_or_default()
            } else {
                Vec::new()
            };
            return Ok((status, bytes));
        }
        Ok((status, read_json(response).await?))
    }
}
fn rejected(status: u16) -> Response {
    Response {
        status,
        body: "{\"error\":\"local_request_rejected\"}".to_owned(),
    }
}
fn rejected_history(status: u16, bytes: &[u8]) -> Response {
    let value: Value = serde_json::from_slice(bytes).unwrap_or(Value::Null);
    let allowed = matches!(
        (status, value["error"].as_str()),
        (400, Some("adapter_history_cursor_invalid"))
            | (403, Some("adapter_history_binding_mismatch"))
            | (
                409,
                Some("adapter_history_scope_changed" | "adapter_history_authority_changed")
            )
            | (410, Some("adapter_history_snapshot_expired"))
            | (413, Some("adapter_history_budget_exceeded"))
            | (404, Some("not_found"))
            | (503, Some("store_unavailable"))
    );
    if allowed && exact(&value, &["error"], &[]).is_ok() {
        Response {
            status,
            body: value.to_string(),
        }
    } else {
        rejected(status)
    }
}
fn valid_capabilities(values: &[String]) -> bool {
    values.len() <= 64
        && values.iter().collect::<HashSet<_>>().len() == values.len()
        && values.iter().all(|s| {
            (2..=81).contains(&s.len())
                && s.as_bytes()[0].is_ascii_lowercase()
                && s.bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'_')
        })
}
fn check_capabilities(bytes: &[u8]) -> Result<Vec<String>> {
    let value: Value = serde_json::from_slice(bytes).map_err(|_| error("invalid_capabilities"))?;
    exact(&value, &["version", "capabilities"], &[])?;
    let capabilities = value["capabilities"]
        .as_array()
        .ok_or_else(|| error("invalid_capabilities"))?
        .iter()
        .map(|v| {
            v.as_str()
                .map(str::to_owned)
                .ok_or_else(|| error("invalid_capabilities"))
        })
        .collect::<Result<Vec<_>>>()?;
    if value["version"] != 1 || !valid_capabilities(&capabilities) {
        return Err(error("invalid_capabilities"));
    }
    Ok(capabilities)
}
async fn read_json(mut response: reqwest::Response) -> Result<Vec<u8>> {
    if response
        .headers()
        .get(header::CONTENT_TYPE)
        .and_then(|h| h.to_str().ok())
        .is_none_or(|s| s.split(';').next() != Some("application/json"))
    {
        return Err(error("invalid_content_type"));
    }
    if response
        .content_length()
        .is_some_and(|n| n > MAX_PAGE as u64)
    {
        return Err(error("response_oversized"));
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response
        .chunk()
        .await
        .map_err(|_| error("response_failed"))?
    {
        if bytes.len() + chunk.len() > MAX_PAGE {
            return Err(error("response_oversized"));
        }
        bytes.extend_from_slice(&chunk);
    }
    Ok(bytes)
}
enum Route {
    Capabilities,
    Register,
    Message(String),
    Replies(String, u64),
    History(String),
}
impl Route {
    fn parse(input: &Request, link: &Link) -> Result<Self> {
        if input.path.len() > 512
            || input.path.contains('%')
            || input
                .body
                .as_ref()
                .is_some_and(|v| serde_json::to_vec(v).map_or(true, |x| x.len() > MAX_BODY))
        {
            return Err(error("invalid_route"));
        }
        if input.method == "POST" && input.path == "/v1/adapters/sessions" {
            let b = input.body.as_ref().ok_or_else(|| error("invalid_body"))?;
            exact(
                b,
                &[
                    "sourceApp",
                    "sourceSessionId",
                    "sourceThreadId",
                    "channelId",
                    "expectedChannelRevision",
                    "clientNonce",
                ],
                &[],
            )?;
            let s = &link.source;
            if b["sourceApp"] != "wiii"
                || b["sourceSessionId"] != s.source_session_id
                || b["sourceThreadId"] != s.source_thread_id
                || b["channelId"] != s.channel_id
                || b["expectedChannelRevision"] != s.expected_channel_revision
                || !nonce(&b["clientNonce"])
            {
                return Err(error("binding_mismatch"));
            }
            return Ok(Self::Register);
        }
        if input.method == "GET"
            && input.body.is_none()
            && input.path == "/v1/adapters/capabilities"
        {
            return Ok(Self::Capabilities);
        }
        let tail = input
            .path
            .strip_prefix("/v1/adapters/sessions/")
            .ok_or_else(|| error("invalid_route"))?;
        let (adapter, action) = tail.split_once('/').ok_or_else(|| error("invalid_route"))?;
        if !route_id(adapter) {
            return Err(error("invalid_route"));
        }
        if input.method == "GET" && input.body.is_none() {
            if action == "history"
                || action
                    .strip_prefix("history?cursor=")
                    .is_some_and(history_token)
            {
                if !link.capabilities.iter().any(|c| c == HISTORY_CAPABILITY) {
                    return Err(error("history_repair_unsupported"));
                }
                return Ok(Self::History(adapter.to_owned()));
            }
            let after = action
                .strip_prefix("replies?after=")
                .filter(|s| !s.is_empty() && s.len() <= 16 && s.bytes().all(|b| b.is_ascii_digit()))
                .ok_or_else(|| error("invalid_route"))?
                .parse::<u64>()
                .map_err(|_| error("invalid_cursor"))?;
            if after > MAX_SAFE {
                return Err(error("invalid_cursor"));
            }
            return Ok(Self::Replies(adapter.to_owned(), after));
        }
        if input.method != "POST" || action != "messages" {
            return Err(error("invalid_route"));
        }
        let b = input.body.as_ref().ok_or_else(|| error("invalid_body"))?;
        exact(
            b,
            &["sourceMessageId", "botId", "text", "lane", "clientNonce"],
            &[
                "accountId",
                "accountRevision",
                "projectId",
                "projectRevision",
                "fileIds",
            ],
        )?;
        if !string_id(&b["sourceMessageId"])
            || !nonce(&b["clientNonce"])
            || !link.bots.iter().any(|bot| b["botId"] == bot.id)
            || !b["text"]
                .as_str()
                .is_some_and(|s| !s.trim().is_empty() && s.len() <= 32768)
            || !matches!(b["lane"].as_str(), Some("foreground" | "background"))
        {
            return Err(error("invalid_message"));
        }
        for (key, revision) in [
            ("accountId", "accountRevision"),
            ("projectId", "projectRevision"),
        ] {
            if b.get(key).is_some() != b.get(revision).is_some()
                || b.get(key).is_some()
                    && (!string_id(&b[key]) || !b[revision].as_u64().is_some_and(rev))
            {
                return Err(error("invalid_resource_pin"));
            }
        }
        if let Some(files) = b.get("fileIds") {
            let a = files.as_array().ok_or_else(|| error("invalid_files"))?;
            if a.len() > 32
                || a.iter().any(|x| !string_id(x))
                || a.iter()
                    .filter_map(Value::as_str)
                    .collect::<HashSet<_>>()
                    .len()
                    != a.len()
            {
                return Err(error("invalid_files"));
            }
        }
        Ok(Self::Message(adapter.to_owned()))
    }
}
fn check_binding(v: &Value, source: &Source, adapter: &str, page: bool) -> Result<()> {
    let key = if page { "adapterSessionId" } else { "id" };
    if v[key] != adapter
        || v["sourceApp"] != "wiii"
        || v["sourceSessionId"] != source.source_session_id
        || v["sourceThreadId"] != source.source_thread_id
        || v["channelId"] != source.channel_id
        || v["channelRevision"] != source.expected_channel_revision
        || v["ownerUserId"] != source.owner_user_id
        || v["spaceId"] != source.space_id
        || v["returnRoute"] != format!("/v1/adapters/sessions/{adapter}/replies")
    {
        return Err(error("response_binding_mismatch"));
    }
    Ok(())
}
fn check_session(v: &Value, source: &Source, adapter: &str) -> Result<()> {
    exact(
        v,
        &[
            "id",
            "version",
            "sourceApp",
            "sourceSessionId",
            "sourceThreadId",
            "channelId",
            "channelRevision",
            "ownerUserId",
            "spaceId",
            "returnRoute",
        ],
        &[],
    )?;
    if v["version"] != 1 {
        return Err(error("invalid_version"));
    }
    check_binding(v, source, adapter, false)
}
fn check_page(bytes: &[u8], source: &Source, adapter: &str, after: u64) -> Result<()> {
    let v: Value = serde_json::from_slice(bytes).map_err(|_| error("invalid_response"))?;
    exact(
        &v,
        &[
            "adapterSessionId",
            "sourceApp",
            "sourceSessionId",
            "sourceThreadId",
            "channelId",
            "channelRevision",
            "ownerUserId",
            "spaceId",
            "returnRoute",
            "cursor",
            "replies",
        ],
        &[],
    )?;
    check_binding(&v, source, adapter, true)?;
    let cursor = v["cursor"]
        .as_u64()
        .filter(|n| *n >= after && *n <= MAX_SAFE)
        .ok_or_else(|| error("invalid_cursor"))?;
    let replies = v["replies"]
        .as_array()
        .filter(|a| a.len() <= 512)
        .ok_or_else(|| error("invalid_replies"))?;
    for r in replies {
        exact(
            r,
            &[
                "replyId",
                "requestId",
                "sourceMessageId",
                "botId",
                "role",
                "text",
                "status",
                "sequence",
            ],
            &["artifacts"],
        )?;
        if !string_id(&r["requestId"])
            || !string_id(&r["sourceMessageId"])
            || !string_id(&r["botId"])
            || r["role"] != "assistant"
            || r["replyId"] != format!("reply-{}", r["requestId"].as_str().unwrap_or(""))
            || !r["text"].is_string()
            || !r["status"].is_string()
            || !r["sequence"]
                .as_u64()
                .is_some_and(|n| n > after && n <= cursor)
        {
            return Err(error("invalid_reply"));
        }
        if let Some(a) = r.get("artifacts") {
            for item in a
                .as_array()
                .filter(|a| a.len() <= 128)
                .ok_or_else(|| error("invalid_artifacts"))?
            {
                exact(item, &["id", "filename", "sha256", "bytes"], &[])?;
                if !string_id(&item["id"])
                    || !item["filename"].is_string()
                    || !item["sha256"].is_string()
                    || !item["bytes"].as_u64().is_some_and(|n| n <= MAX_SAFE)
                {
                    return Err(error("invalid_artifact"));
                }
            }
        }
    }
    Ok(())
}
fn check_history_page(bytes: &[u8], source: &Source, adapter: &str) -> Result<()> {
    if bytes.len() > MAX_HISTORY_PAGE {
        return Err(error("response_oversized"));
    }
    let v: Value = serde_json::from_slice(bytes).map_err(|_| error("invalid_response"))?;
    exact(
        &v,
        &[
            "version",
            "adapterSessionId",
            "sourceApp",
            "sourceSessionId",
            "sourceThreadId",
            "channelId",
            "channelRevision",
            "ownerUserId",
            "spaceId",
            "returnRoute",
            "snapshotId",
            "snapshotCursor",
            "authorityDigest",
            "historyDigest",
            "offset",
            "totalRows",
            "totalBytes",
            "nextCursor",
            "confirmationCursor",
            "complete",
            "rows",
        ],
        &[],
    )?;
    check_binding(&v, source, adapter, true)?;
    let hash = |x: &Value| {
        x.as_str().is_some_and(|s| {
            s.len() == 64
                && s.bytes()
                    .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
        })
    };
    let token = |x: &Value| x.as_str().is_some_and(history_token);
    let cursor = v["snapshotCursor"]
        .as_u64()
        .filter(|n| *n <= MAX_SAFE)
        .ok_or_else(|| error("invalid_cursor"))?;
    let offset = v["offset"]
        .as_u64()
        .ok_or_else(|| error("invalid_cursor"))?;
    let total = v["totalRows"]
        .as_u64()
        .filter(|n| *n <= 8192)
        .ok_or_else(|| error("invalid_history_page"))?;
    let rows = v["rows"]
        .as_array()
        .filter(|r| r.len() <= 128)
        .ok_or_else(|| error("invalid_history_page"))?;
    let end = offset
        .checked_add(rows.len() as u64)
        .filter(|n| *n <= total)
        .ok_or_else(|| error("invalid_history_page"))?;
    let complete = end == total;
    if v["version"] != 1
        || !hash(&v["authorityDigest"])
        || !hash(&v["historyDigest"])
        || !v["snapshotId"]
            .as_str()
            .is_some_and(|s| s.strip_prefix("h1_").is_some_and(|h| hash(&json!(h))))
        || !v["totalBytes"].as_u64().is_some_and(|n| n <= 67_108_864)
        || offset > total
        || v["complete"] != complete
        || !complete && rows.is_empty()
        || if complete {
            !v["nextCursor"].is_null() || !token(&v["confirmationCursor"])
        } else {
            !token(&v["nextCursor"]) || !v["confirmationCursor"].is_null()
        }
    {
        return Err(error("invalid_history_page"));
    }
    let mut messages = HashSet::new();
    let mut requests = HashSet::new();
    for r in rows {
        exact(
            r,
            &[
                "replyId",
                "requestId",
                "sourceMessageId",
                "botId",
                "role",
                "text",
                "status",
                "sequence",
                "artifacts",
                "availability",
            ],
            &[],
        )?;
        let unavailable = r["availability"] == "source_unavailable";
        let artifacts = r["artifacts"]
            .as_array()
            .filter(|a| a.len() <= 8)
            .ok_or_else(|| error("invalid_artifacts"))?;
        if !string_id(&r["requestId"])
            || !string_id(&r["sourceMessageId"])
            || !string_id(&r["botId"])
            || r["role"] != "assistant"
            || r["replyId"] != format!("reply-{}", r["requestId"].as_str().unwrap_or(""))
            || !r["text"]
                .as_str()
                .is_some_and(|s| s.len() <= 65536 && (!unavailable || s.is_empty()))
            || !r["status"].as_str().is_some_and(|s| {
                !s.is_empty()
                    && s.len() <= 64
                    && s.bytes().all(|b| b.is_ascii_lowercase() || b == b'_')
            })
            || !r["sequence"].as_u64().is_some_and(|n| n <= cursor)
            || !matches!(
                r["availability"].as_str(),
                Some("available" | "source_unavailable")
            )
            || unavailable && !artifacts.is_empty()
            || !messages.insert(r["sourceMessageId"].as_str().unwrap_or(""))
            || !requests.insert(r["requestId"].as_str().unwrap_or(""))
        {
            return Err(error("invalid_reply"));
        }
        for a in artifacts {
            exact(a, &["id", "filename", "sha256", "bytes"], &[])?;
            if !string_id(&a["id"])
                || !hash(&a["sha256"])
                || !a["bytes"].as_u64().is_some_and(|n| n <= MAX_SAFE)
                || !a["filename"].as_str().is_some_and(|s| {
                    !s.trim().is_empty()
                        && s.len() <= 80
                        && !s
                            .chars()
                            .any(|c| c <= '\u{1f}' || c == '\u{7f}' || c == '/' || c == '\\')
                })
            {
                return Err(error("invalid_artifact"));
            }
        }
    }
    Ok(())
}
fn project_receipt(v: &Value, body: &Value, source: &Source, adapter: &str) -> Result<Value> {
    exact(v, &["work", "duplicate"], &[])?;
    if !v["duplicate"].is_boolean() {
        return Err(error("invalid_receipt"));
    }
    let w = &v["work"];
    let p = &w["adapterPin"];
    let publication = &w["publication"];
    exact(
        p,
        &[
            "adapterSessionId",
            "sourceApp",
            "sourceSessionId",
            "sourceThreadId",
            "sourceMessageId",
            "clientNonce",
            "channelRevision",
            "returnRoute",
        ],
        &[],
    )?;
    if !string_id(&w["requestId"])
        || !string_id(&w["clientNonce"])
        || w["ownerUserId"] != source.owner_user_id
        || w["spaceId"] != source.space_id
        || w["conversationId"] != source.channel_id
        || w["text"] != body["text"]
        || w["lane"] != body["lane"]
        || p["adapterSessionId"] != adapter
        || p["sourceApp"] != "wiii"
        || p["sourceSessionId"] != source.source_session_id
        || p["sourceThreadId"] != source.source_thread_id
        || p["sourceMessageId"] != body["sourceMessageId"]
        || p["clientNonce"] != body["clientNonce"]
        || p["channelRevision"] != source.expected_channel_revision
        || p["returnRoute"] != format!("/v1/adapters/sessions/{adapter}/replies")
        || publication["sourceChannelId"] != source.channel_id
        || publication["replyChannelId"] != source.channel_id
        || publication["audienceRevision"] != source.expected_channel_revision
    {
        return Err(error("receipt_binding_mismatch"));
    }
    // Work may carry private model context. Only the bridge's acknowledgement fields cross into the WebView.
    let mut projected = serde_json::Map::new();
    for key in [
        "requestId",
        "clientNonce",
        "ownerUserId",
        "spaceId",
        "conversationId",
        "text",
        "lane",
        "adapterPin",
    ] {
        projected.insert(key.to_owned(), w[key].clone());
    }
    projected.insert("publication".to_owned(), json!({"sourceChannelId": source.channel_id, "replyChannelId": source.channel_id, "audienceRevision": source.expected_channel_revision}));
    Ok(json!({"work": projected, "duplicate": v["duplicate"]}))
}

#[cfg(test)]
mod tests;
