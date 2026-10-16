use serde::Deserialize;
use serde_json::Value;
use std::{path::PathBuf, process::Command};

type Result<T> = std::result::Result<T, &'static str>;
const SAFE: u64 = 9_007_199_254_740_991;

pub struct PrivateProfileSource {
    pub home: PathBuf,
    pub key: String,
    pub source_id: String,
    pub source_revision: u64,
    pub profile_id: String,
    pub profile_revision: String,
    pub limits: Value,
    pub fixture_only: bool,
    pub capsule_path: Option<PathBuf>,
    pub snapshot_id: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct ActorScope {
    version: u32,
    mode: String,
    actor_id: String,
    space_id: String,
    authority_id: String,
    device_id: String,
    device_revision: u64,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Limits {
    max_input_bytes: usize,
    max_output_tokens: usize,
    max_requests: usize,
    max_steps: usize,
    max_seconds: u64,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Pin {
    version: u32,
    fixture_only: bool,
    runtime_sha256: String,
    source_id: String,
    source_revision: u64,
    profile_id: String,
    profile_revision: String,
    provider: String,
    model: String,
    provider_origin: String,
    consent_id: String,
    consent_revision: u64,
    actor_scope: ActorScope,
    channel_id: String,
    audience_revision: u64,
    bot_id: String,
    project_id: Option<String>,
    project_revision: Option<u64>,
    client_nonce: String,
    context_sha256: String,
    resources_sha256: String,
    limits: Limits,
    expires_at_unix_ms: u64,
    auth_kind: Option<String>,
    credential_source: Option<String>,
    auth_policy: Option<String>,
    credential_snapshot_id: Option<String>,
    credential_expires_at_unix_ms: Option<u64>,
}

pub struct CoreProfileLaunch {
    home: PathBuf,
    key: String,
    id: String,
    revision: String,
    origin: String,
    limits: Value,
    max_steps: usize,
    max_seconds: u64,
    auth_kind: Option<String>,
    credential_source: Option<String>,
    auth_policy: Option<String>,
    credential_expiry: Option<u64>,
    capsule_path: Option<PathBuf>,
    snapshot_id: Option<String>,
    deadline: u64,
}

fn hex(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
}
fn text(value: &str, bound: usize) -> bool {
    !value.is_empty()
        && value.len() <= bound
        && !value.chars().any(|c| c.is_control() || c == '\u{fffd}')
}
fn profile_id(value: &str) -> bool {
    text(value, 64)
        && value
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || c == b'_' || c == b'-')
        && value.as_bytes()[0].is_ascii_alphanumeric()
}
fn origin(value: &str, fixture: bool) -> bool {
    if fixture {
        return value
            .strip_prefix("http://127.0.0.1:")
            .and_then(|s| s.parse::<u16>().ok())
            .is_some_and(|p| p > 0);
    }
    value.strip_prefix("https://").is_some_and(|s| {
        !s.is_empty()
            && s.bytes()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, b'-' | b'.' | b':'))
    })
}

fn auth_policy(pin: &Pin, source: &PrivateProfileSource, now: u64) -> bool {
    if pin.version == 1 {
        return pin.auth_kind.is_none()
            && pin.credential_source.is_none()
            && pin.auth_policy.is_none()
            && pin.credential_snapshot_id.is_none()
            && pin.credential_expires_at_unix_ms.is_none()
            && source.capsule_path.is_none()
            && source.snapshot_id.is_none();
    }
    if pin.version != 2 {
        return false;
    }
    let tuple = (
        pin.auth_kind.as_deref(),
        pin.credential_source.as_deref(),
        pin.auth_policy.as_deref(),
    );
    let supported = match tuple {
        (Some("api_key"), Some("file_api_key" | "process_env"), Some("static_key")) => true,
        (Some("none"), Some("local_none"), Some("static_key")) => true,
        (Some("kimi_oauth"), Some("oauth_cache"), Some("cached_token_no_refresh")) => {
            pin.provider == "kimi" && pin.credential_expires_at_unix_ms.is_some()
        }
        _ => false,
    };
    if !supported {
        return false;
    }
    if pin.credential_source.as_deref() == Some("process_env")
        && (pin.credential_snapshot_id.is_none() || pin.credential_expires_at_unix_ms.is_none())
    {
        return false;
    }
    if pin
        .credential_expires_at_unix_ms
        .is_some_and(|v| v <= now || v > SAFE || pin.expires_at_unix_ms > v)
    {
        return false;
    }
    match (
        &pin.credential_snapshot_id,
        &source.snapshot_id,
        &source.capsule_path,
    ) {
        (None, None, None) => true,
        (Some(id), Some(bound), Some(path)) => {
            id == bound
                && id.strip_prefix("cs1_").is_some_and(|s| {
                    s.len() == 32
                        && s.bytes()
                            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
                })
                && path.is_absolute()
                && !path.starts_with(&source.home)
                && pin.credential_expires_at_unix_ms.is_some()
        }
        _ => false,
    }
}

impl CoreProfileLaunch {
    pub fn parse(
        spec: &Value,
        digest: &str,
        source: Option<&PrivateProfileSource>,
        now: u64,
    ) -> Result<Option<Self>> {
        let Some(value) = spec.get("coreProfilePin") else {
            return if source.is_none() {
                Ok(None)
            } else {
                Err("core_profile_binding_invalid")
            };
        };
        let source = source.ok_or("core_profile_binding_missing")?;
        let pin: Pin =
            serde_json::from_value(value.clone()).map_err(|_| "core_profile_pin_invalid")?;
        let context = spec.get("contextPinSHA256").or_else(|| {
            spec.get("contextExpected")
                .and_then(|v| v.get("contextPinSHA256"))
        });
        if !auth_policy(&pin, source, now)
            || !hex(digest)
            || pin.runtime_sha256 != digest
            || !source.home.is_absolute()
            || !hex(&source.key)
            || pin.source_id != source.source_id
            || pin.source_revision != source.source_revision
            || pin.source_revision == 0
            || pin.source_revision > SAFE
            || pin.profile_id != source.profile_id
            || !profile_id(&pin.profile_id)
            || !profile_id(&pin.source_id)
            || pin.profile_revision != source.profile_revision
            || !pin
                .profile_revision
                .strip_prefix(if pin.version == 2 { "pr2_" } else { "pr1_" })
                .is_some_and(hex)
            || pin.fixture_only != source.fixture_only
            || !origin(&pin.provider_origin, pin.fixture_only)
            || !matches!(
                pin.provider.as_str(),
                "openai_compat" | "responses" | "anthropic" | "kimi"
            )
            || !text(&pin.model, 256)
            || !text(&pin.consent_id, 128)
            || pin.consent_revision == 0
            || pin.consent_revision > SAFE
            || !text(&pin.channel_id, 128)
            || pin.audience_revision == 0
            || pin.audience_revision > SAFE
            || !text(&pin.bot_id, 128)
            || !text(&pin.client_nonce, 128)
            || pin.client_nonce.len() < 8
            || !hex(&pin.resources_sha256)
            || context != Some(&Value::String(pin.context_sha256.clone()))
            || !hex(&pin.context_sha256)
            || spec["contextProtocol"] != 1
            || pin.expires_at_unix_ms <= now
            || pin.expires_at_unix_ms > SAFE
            || pin.expires_at_unix_ms
                > now.saturating_add(pin.limits.max_seconds.saturating_mul(1000))
            || pin.project_id.is_some() != pin.project_revision.is_some()
            || pin.project_id.as_ref().is_some_and(|v| !text(v, 128))
            || pin.project_revision.is_some_and(|v| v == 0 || v > SAFE)
            || spec.get("accountPin").is_some()
            || spec.get("accountLeasePin").is_some()
        {
            return Err("core_profile_binding_invalid");
        }
        let actor = &pin.actor_scope;
        if actor.version != 1
            || actor.mode != "local_owner"
            || actor.actor_id != "local-owner"
            || actor.space_id != "local-space"
            || actor.authority_id != "nekobot-local"
            || actor.device_id != "local-executor"
            || actor.device_revision != 1
        {
            return Err("core_profile_actor_invalid");
        }
        let limits = &pin.limits;
        if !(1..=64000).contains(&limits.max_input_bytes)
            || !(1..=4096).contains(&limits.max_output_tokens)
            || !(1..=8).contains(&limits.max_requests)
            || !(1..=6).contains(&limits.max_steps)
            || !(1..=600).contains(&limits.max_seconds)
            || value["limits"] != source.limits
            || spec["text"]
                .as_str()
                .is_none_or(|v| v.len() > limits.max_input_bytes)
        {
            return Err("core_profile_limits_invalid");
        }
        Ok(Some(Self {
            home: source.home.clone(),
            key: source.key.clone(),
            id: pin.profile_id,
            revision: pin.profile_revision,
            origin: pin.provider_origin,
            limits: source.limits.clone(),
            max_steps: limits.max_steps,
            max_seconds: limits.max_seconds,
            auth_kind: pin.auth_kind,
            credential_source: pin.credential_source,
            auth_policy: pin.auth_policy,
            credential_expiry: pin.credential_expires_at_unix_ms,
            capsule_path: source.capsule_path.clone(),
            snapshot_id: source.snapshot_id.clone(),
            deadline: pin.expires_at_unix_ms,
        }))
    }

    pub fn apply(&self, command: &mut Command) -> Result<()> {
        let limits =
            serde_json::to_string(&self.limits).map_err(|_| "core_profile_limits_invalid")?;
        command.args(["--profile", self.id.as_str()]);
        command
            .env("NEKO_CREDENTIAL_HOME", &self.home)
            .env("NEKO_PROFILE_REVISION_KEY", &self.key)
            .env("NEKO_PROFILE_REVISION", &self.revision)
            .env("NEKO_PROFILE_ORIGIN", &self.origin)
            .env("NEKO_PROFILE_LIMITS", limits)
            .env("NEKO_MAX_STEPS", self.max_steps.to_string())
            .env("NEKO_TIMEOUT_SECONDS", self.max_seconds.to_string());
        if let (Some(kind), Some(source), Some(policy)) =
            (&self.auth_kind, &self.credential_source, &self.auth_policy)
        {
            command
                .env("NEKO_PROFILE_AUTH_KIND", kind)
                .env("NEKO_PROFILE_CREDENTIAL_SOURCE", source)
                .env("NEKO_PROFILE_AUTH_POLICY", policy)
                .env_remove("NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS")
                .env("NEKO_PROFILE_DEADLINE_UNIX_MS", self.deadline.to_string());
            if let Some(expiry) = self.credential_expiry {
                command.env(
                    "NEKO_PROFILE_CREDENTIAL_EXPIRES_AT_UNIX_MS",
                    expiry.to_string(),
                );
            }
        }
        if let (Some(path), Some(id)) = (&self.capsule_path, &self.snapshot_id) {
            command
                .env("NEKO_PROFILE_CAPSULE_PATH", path)
                .env("NEKO_PROFILE_SNAPSHOT_ID", id);
        }
        Ok(())
    }
}
