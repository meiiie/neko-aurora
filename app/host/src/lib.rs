use serde::{
    de::{self, MapAccess, SeqAccess, Visitor},
    Deserialize, Deserializer,
};
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::{collections::HashMap, fmt};
pub mod acp;
pub mod context_admission;
pub mod core_profile;
pub mod vault;
pub mod saved_key;

pub const MAX_FRAME: usize = 65536;
const SAFE: i64 = 9_007_199_254_740_991;

#[derive(Debug, PartialEq, Eq)]
pub enum Rejection {
    Frame,
    Capability,
    Binding,
    Expired,
    Conflict,
    Sequence,
}
struct StrictValue(Value);
impl<'de> Deserialize<'de> for StrictValue {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        struct Strict;
        impl<'de> Visitor<'de> for Strict {
            type Value = StrictValue;
            fn expecting(&self, f: &mut fmt::Formatter) -> fmt::Result {
                f.write_str("canonical v1 data")
            }
            fn visit_bool<E: de::Error>(self, v: bool) -> Result<Self::Value, E> {
                Ok(StrictValue(v.into()))
            }
            fn visit_unit<E: de::Error>(self) -> Result<Self::Value, E> {
                Ok(StrictValue(Value::Null))
            }
            fn visit_i64<E: de::Error>(self, v: i64) -> Result<Self::Value, E> {
                if !(-SAFE..=SAFE).contains(&v) {
                    return Err(E::custom("integer bound"));
                }
                Ok(StrictValue(v.into()))
            }
            fn visit_u64<E: de::Error>(self, v: u64) -> Result<Self::Value, E> {
                if v > SAFE as u64 {
                    return Err(E::custom("integer bound"));
                }
                Ok(StrictValue(v.into()))
            }
            fn visit_str<E: de::Error>(self, v: &str) -> Result<Self::Value, E> {
                if v.contains('\u{fffd}') {
                    return Err(E::custom("invalid Unicode"));
                }
                Ok(StrictValue(v.into()))
            }
            fn visit_string<E: de::Error>(self, v: String) -> Result<Self::Value, E> {
                self.visit_str(&v)
            }
            fn visit_seq<A: SeqAccess<'de>>(self, mut seq: A) -> Result<Self::Value, A::Error> {
                let mut out = vec![];
                while let Some(v) = seq.next_element::<StrictValue>()? {
                    out.push(v.0);
                }
                Ok(StrictValue(Value::Array(out)))
            }
            fn visit_map<A: MapAccess<'de>>(self, mut map: A) -> Result<Self::Value, A::Error> {
                let mut out = serde_json::Map::new();
                while let Some(key) = map.next_key::<String>()? {
                    if out.contains_key(&key) || key.contains('\u{fffd}') {
                        return Err(de::Error::custom("duplicate or invalid key"));
                    }
                    out.insert(key, map.next_value::<StrictValue>()?.0);
                }
                Ok(StrictValue(Value::Object(out)))
            }
        }
        d.deserialize_any(Strict)
    }
}
fn encode(value: &Value, depth: usize, out: &mut String) -> Result<(), Rejection> {
    if depth > 32 {
        return Err(Rejection::Frame);
    }
    match value {
        Value::Object(map) => {
            let mut keys: Vec<_> = map.keys().collect();
            keys.sort();
            out.push('{');
            for (index, key) in keys.iter().enumerate() {
                if index > 0 {
                    out.push(',');
                }
                out.push_str(&serde_json::to_string(key).map_err(|_| Rejection::Frame)?);
                out.push(':');
                encode(&map[*key], depth + 1, out)?;
            }
            out.push('}');
        }
        Value::Array(values) => {
            out.push('[');
            for (index, v) in values.iter().enumerate() {
                if index > 0 {
                    out.push(',');
                }
                encode(v, depth + 1, out)?;
            }
            out.push(']');
        }
        other => out.push_str(&serde_json::to_string(other).map_err(|_| Rejection::Frame)?),
    }
    Ok(())
}
pub fn canonical(frame: &[u8]) -> Result<Vec<u8>, Rejection> {
    if frame.len() > MAX_FRAME {
        return Err(Rejection::Frame);
    }
    let value: StrictValue = serde_json::from_slice(frame).map_err(|_| Rejection::Frame)?;
    let mut out = String::new();
    encode(&value.0, 0, &mut out)?;
    Ok(out.into_bytes())
}
pub fn digest(frame: &[u8]) -> Result<String, Rejection> {
    Ok(format!("{:x}", Sha256::digest(canonical(frame)?)))
}
fn parse<T: for<'de> Deserialize<'de>>(frame: &[u8]) -> Result<T, Rejection> {
    let bytes = canonical(frame)?;
    serde_json::from_slice(&bytes).map_err(|_| Rejection::Frame)
}
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct CoreHello {
    pub contract_version: u32,
    pub acp_version: u32,
    pub profile: String,
    pub exclusive: bool,
    pub root_digest: String,
    pub capabilities: Capabilities,
}
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Capabilities {
    pub fixed_task: u32,
    pub execution_binding: u32,
    pub operation_identity: u32,
    pub host_effect: u32,
}
#[derive(Debug, Clone, PartialEq, Eq, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Binding {
    pub request_id: String,
    pub attempt_id: String,
    pub owner_user_id: String,
    pub space_id: String,
    pub bot_id: String,
    pub core_session_id: String,
    pub core_task_id: String,
    pub epoch: u64,
    pub lease_fence: u64,
    pub root_digest: String,
}
#[derive(Debug, Clone)]
pub struct Policy {
    pub request_id: String,
    pub attempt_id: String,
    pub owner_user_id: String,
    pub space_id: String,
    pub bot_id: String,
    pub epoch: u64,
    pub lease_fence: u64,
    pub root_digest: String,
}
fn id(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || c == b'-' || c == b'_')
}
impl Policy {
    pub fn admit(&self, hello: &[u8], receipt: &[u8]) -> Result<Binding, Rejection> {
        let h: CoreHello = parse(hello)?;
        let c = h.capabilities;
        if h.contract_version != 1
            || h.acp_version != 1
            || h.profile != "nekobot"
            || !h.exclusive
            || c.fixed_task != 1
            || c.execution_binding != 1
            || c.operation_identity != 1
            || c.host_effect != 1
        {
            return Err(Rejection::Capability);
        }
        if self.root_digest.len() != 64
            || !self
                .root_digest
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            || h.root_digest != self.root_digest
        {
            return Err(Rejection::Binding);
        }
        let b: Binding = parse(receipt)?;
        if b.request_id != self.request_id
            || b.attempt_id != self.attempt_id
            || b.owner_user_id != self.owner_user_id
            || b.space_id != self.space_id
            || b.bot_id != self.bot_id
            || b.epoch != self.epoch
            || b.lease_fence != self.lease_fence
            || b.root_digest != self.root_digest
            || !id(&b.core_session_id)
            || !id(&b.core_task_id)
        {
            return Err(Rejection::Binding);
        }
        Ok(b)
    }
}
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Command {
    contract_version: u32,
    command_id: String,
    kind: String,
    request_id: String,
    attempt_id: String,
    epoch: u64,
    lease_fence: u64,
    expires_at_millis: u64,
}
#[derive(Debug, PartialEq, Eq)]
pub enum Admission {
    New,
    Duplicate,
}
pub struct CommandGate {
    policy: Policy,
    commands: HashMap<String, String>,
}
impl CommandGate {
    pub fn new(policy: Policy) -> Self {
        Self {
            policy,
            commands: HashMap::new(),
        }
    }
    pub fn check(&mut self, frame: &[u8], now: u64) -> Result<Admission, Rejection> {
        let c: Command = parse(frame)?;
        if c.contract_version != 1
            || !id(&c.command_id)
            || !["Start", "Pause", "Resume", "Cancel", "GetStatus"].contains(&c.kind.as_str())
        {
            return Err(Rejection::Frame);
        }
        if c.request_id != self.policy.request_id
            || c.attempt_id != self.policy.attempt_id
            || c.epoch != self.policy.epoch
            || c.lease_fence != self.policy.lease_fence
        {
            return Err(Rejection::Binding);
        }
        let d = digest(frame)?;
        if let Some(previous) = self.commands.get(&c.command_id) {
            return if *previous == d {
                Ok(Admission::Duplicate)
            } else {
                Err(Rejection::Conflict)
            };
        }
        if c.expires_at_millis <= now {
            return Err(Rejection::Expired);
        }
        if self.commands.len() >= 1024 {
            return Err(Rejection::Frame);
        }
        self.commands.insert(c.command_id, d);
        Ok(Admission::New)
    }
}
pub mod executor;
pub mod runtime_stop;
pub mod saved_task_witness;
pub mod executable_guard;

pub mod saved_account_bootstrap;
