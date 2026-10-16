use crate::{canonical, Rejection};
use serde_json::{json, Value};

pub const TOOLS: [&str; 2] = ["mcp__nekobot__capabilities", "mcp__nekobot__execute_tool"];

fn hex(value: &Value, size: usize) -> bool {
    value.as_str().is_some_and(|s| {
        s.len() == size
            && s.bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    })
}
pub fn initialize(value: &Value) -> Result<(), Rejection> {
    let m = &value["_meta"];
    let p = &m["neko.hostProfile"];
    if value["protocolVersion"] != 1
        || m["neko.taskProtocol"] != json!({"version":1,"mode":"fixed-active-task"})
        || m["neko.executionProtocol"] != json!({"version":1})
        || m["neko.operationProtocol"] != json!({"version":1})
        || m["neko.hostEffectProtocol"] != json!({"version":1})
        || p["id"] != "nekobot"
        || p["version"] != 1
        || p["isolation"] != "exclusive"
        || p["transport"] != "acp"
        || p["operationProtocol"] != 1
    {
        return Err(Rejection::Capability);
    }
    let mut tools = p["tools"].as_array().ok_or(Rejection::Capability)?.clone();
    tools.sort_by_key(|v| v.as_str().unwrap_or("").to_owned());
    if tools != json!(TOOLS).as_array().unwrap().to_vec() {
        return Err(Rejection::Capability);
    }
    Ok(())
}

#[derive(Clone, Debug)]
pub struct Binding {
    pub session: String,
    pub task: Value,
    pub execution: Value,
}
impl Binding {
    pub fn admit(value: &Value, root: &str) -> Result<Self, Rejection> {
        let task = &value["_meta"]["neko.task"];
        let exec = &value["_meta"]["neko.execution"];
        if !hex(&value["sessionId"], 32)
            || !hex(&task["id"], 32)
            || !hex(&task["activationId"], 32)
            || task["version"] != 1
            || task["mode"] != "fixed-active-task"
            || task["root"] != root
            || task["activationEpoch"]
                .as_u64()
                .is_none_or(|n| n == 0 || n > 9_007_199_254_740_991)
            || exec["version"] != 1
            || exec["taskId"] != task["id"]
            || exec["root"] != task["root"]
            || exec["activationEpoch"] != task["activationEpoch"]
            || exec["activationId"] != task["activationId"]
            || exec["authorityId"].as_str().is_none_or(str::is_empty)
            || exec["policyId"].as_str().is_none_or(str::is_empty)
            || exec["bashTarget"] != "host"
            || exec["capabilities"]["computer"] != "unavailable"
        {
            return Err(Rejection::Binding);
        }
        let mut tools = exec["capabilities"]["allowedTools"]
            .as_array()
            .ok_or(Rejection::Binding)?
            .clone();
        tools.sort_by_key(|v| v.as_str().unwrap_or("").to_owned());
        if tools != json!(TOOLS).as_array().unwrap().to_vec() {
            return Err(Rejection::Binding);
        }
        Ok(Self {
            session: value["sessionId"].as_str().unwrap().into(),
            task: task.clone(),
            execution: exec.clone(),
        })
    }
    pub fn meta(&self) -> Value {
        json!({"neko.task":self.task,"neko.execution":self.execution})
    }
    pub fn echo(&self, params: &Value) -> Result<(), Rejection> {
        if params["_meta"]["neko.task"] != self.task
            || params["_meta"]["neko.execution"] != self.execution
        {
            return Err(Rejection::Binding);
        }
        Ok(())
    }
    pub fn operation(&self, op: &Value) -> Result<(), Rejection> {
        if op.as_object().is_none_or(|o| o.len() != 7)
            || op["version"] != 1
            || !hex(&op["operationId"], 32)
            || op["sessionId"] != self.session
            || op["taskId"] != self.task["id"]
            || op["root"] != self.task["root"]
            || op["activationEpoch"] != self.task["activationEpoch"]
            || op["activationId"] != self.task["activationId"]
        {
            return Err(Rejection::Binding);
        }
        Ok(())
    }
    pub fn effects_clear(&self, value: &Value) -> Result<(), Rejection> {
        let e = &value["_meta"]["neko.hostEffects"];
        if e["version"] != 1 || e["pending"] != json!([]) || e["unknown"] != json!([]) {
            return Err(Rejection::Binding);
        }
        Ok(())
    }
}

pub fn frame(bytes: &[u8]) -> Result<Value, Rejection> {
    serde_json::from_slice(&canonical(bytes)?).map_err(|_| Rejection::Frame)
}

#[cfg(test)]
mod tests {
    use super::*;
    fn admission() -> Value {
        json!({"sessionId":"a".repeat(32),"_meta":{
            "neko.task":{"version":1,"mode":"fixed-active-task","id":"b".repeat(32),"root":"/task","activationEpoch":1,"activationId":"c".repeat(32)},
            "neko.execution":{"version":1,"taskId":"b".repeat(32),"root":"/task","activationEpoch":1,"activationId":"c".repeat(32),"authorityId":"host","policyId":"restricted","bashTarget":"host","capabilities":{"computer":"unavailable","allowedTools":TOOLS}}
        }})
    }
    #[test]
    fn reject_other_root_or_tool_ceiling() {
        let mut v = admission();
        assert!(Binding::admit(&v, "/task").is_ok());
        assert!(Binding::admit(&v, "/other").is_err());
        v["_meta"]["neko.execution"]["capabilities"]["allowedTools"] = json!(["bash"]);
        assert!(Binding::admit(&v, "/task").is_err());
    }
    #[test]
    fn reject_stale_operation_and_spoofed_echo() {
        let b = Binding::admit(&admission(), "/task").unwrap();
        let mut op = json!({"version":1,"operationId":"d".repeat(32),"sessionId":b.session,"taskId":b.task["id"],"root":"/task","activationEpoch":1,"activationId":b.task["activationId"]});
        assert!(b.operation(&op).is_ok());
        op["activationEpoch"] = json!(2);
        assert!(b.operation(&op).is_err());
        assert!(b.echo(&json!({"_meta":b.meta()})).is_ok());
        assert!(b.echo(&json!({"_meta":{}})).is_err());
    }
}
