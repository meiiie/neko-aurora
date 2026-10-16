use neko_host::acp::Binding;
use neko_host::context_admission::AdmittedContext;
use neko_host::digest;
use serde_json::{json, Value};

pub struct Request {
    pub data: Value,
    arguments: Value,
    request_digest: String,
}

pub struct Pending {
    session: String,
    arguments: Value,
    task: Value,
    execution: Value,
    context: Value,
}

fn fields(value: &Value, expected: &[&str]) -> bool {
    value.as_object().is_some_and(|object| {
        object.len() == expected.len() && expected.iter().all(|key| object.contains_key(*key))
    })
}

fn bounded(value: &Value, size: usize) -> bool {
    value.as_str().is_some_and(|text| {
        !text.is_empty()
            && text.len() <= size
            && !text.chars().any(|c| c.is_control() || c == '\u{fffd}')
    })
}

fn request_digest(arguments: &Value) -> Option<String> {
    let raw = serde_json::to_vec(&json!({"name":"execute_tool","arguments":arguments})).ok()?;
    digest(&raw).ok()
}

pub fn parse(params: &Value, binding: &Binding, context: &AdmittedContext) -> Option<Request> {
    if !fields(params, &["sessionId", "_meta", "toolCall", "options"])
        || params["sessionId"] != binding.session
        || !fields(
            &params["_meta"],
            &["neko.task", "neko.execution", "neko.contextAdmission"],
        )
        || context.echo(binding, params).is_err()
        || !fields(
            &params["toolCall"],
            &["toolCallId", "title", "name", "kind", "status", "rawInput"],
        )
        || params["toolCall"]["name"] != "mcp__nekobot__execute_tool"
        || params["toolCall"]["kind"] != "other"
        || params["toolCall"]["status"] != "pending"
        || !bounded(&params["toolCall"]["toolCallId"], 128)
        || !bounded(&params["toolCall"]["title"], 512)
        || !fields(&params["toolCall"]["rawInput"], &["name", "arguments"])
        || !bounded(&params["toolCall"]["rawInput"]["name"], 64)
        || !params["toolCall"]["rawInput"]["arguments"].is_object()
    {
        return None;
    }
    let options = params["options"].as_array()?;
    if options.is_empty() || options.len() > 4 {
        return None;
    }
    let mut seen = std::collections::HashSet::new();
    for option in options {
        if !fields(option, &["optionId", "name", "kind"])
            || !bounded(&option["name"], 256)
            || option["optionId"] != option["kind"]
        {
            return None;
        }
        let identifier = option["optionId"].as_str()?;
        if !["allow_once", "allow_always", "reject_once", "reject_always"].contains(&identifier)
            || !seen.insert(identifier)
        {
            return None;
        }
    }
    if !seen.contains("allow_once") {
        return None;
    }
    let arguments = params["toolCall"]["rawInput"].clone();
    let request_digest = request_digest(&arguments)?;
    Some(Request {
        data: json!({"version":1,"sessionId":binding.session,"toolCallId":params["toolCall"]["toolCallId"],"name":"mcp__nekobot__execute_tool","arguments":arguments,"task":binding.task,"execution":binding.execution,"contextAdmission":context.receipt()}),
        arguments,
        request_digest,
    })
}

impl Request {
    pub fn approved(
        self,
        result: &Value,
        binding: &Binding,
        context: &AdmittedContext,
    ) -> Option<Pending> {
        if !fields(result, &["version", "allowed", "requestSha256"])
            || result["version"] != 1
            || result["allowed"] != true
            || result["requestSha256"] != self.request_digest
            || self.data["sessionId"] != binding.session
            || self.data["task"] != binding.task
            || self.data["execution"] != binding.execution
            || self.data["contextAdmission"] != *context.receipt()
        {
            return None;
        }
        Some(Pending {
            session: binding.session.clone(),
            arguments: self.arguments,
            task: binding.task.clone(),
            execution: binding.execution.clone(),
            context: context.receipt().clone(),
        })
    }
}

impl Pending {
    pub fn matches(self, args: &Value, binding: &Binding, context: &AdmittedContext) -> bool {
        args["name"] == "execute_tool"
            && binding.session == self.session
            && args["arguments"] == self.arguments
            && binding.task == self.task
            && binding.execution == self.execution
            && context.receipt() == &self.context
            && args["_meta"]["neko.contextAdmission"] == self.context
    }
}

pub fn cancelled() -> Value {
    json!({"outcome":{"outcome":"cancelled"}})
}

pub fn allowed_once() -> Value {
    json!({"outcome":{"outcome":"selected","optionId":"allow_once"}})
}

#[cfg(test)]
mod tests {
    use super::*;
    use neko_host::context_admission::LaunchContext;

    fn fixture() -> (Binding, AdmittedContext, Value) {
        let task = json!({"version":1,"mode":"fixed-active-task","id":"b".repeat(32),"root":"/task","activationEpoch":1,"activationId":"c".repeat(32)});
        let execution = json!({"version":1,"taskId":"b".repeat(32),"root":"/task","activationEpoch":1,"activationId":"c".repeat(32),"authorityId":"e".repeat(64),"policyId":"f".repeat(64),"bashTarget":"host","capabilities":{"computer":"unavailable","allowedTools":neko_host::acp::TOOLS}});
        let binding = Binding {
            session: "a".repeat(32),
            task,
            execution,
        };
        let receipt = json!({"version":1,"sessionId":binding.session,"contextPinSHA256":"1".repeat(64),"manifestSHA256":"2".repeat(64),"sourceIds":["3".repeat(64)],"task":binding.task,"execution":binding.execution});
        let raw = json!({"contextProtocol":1,"resume":{"sessionId":binding.session,"task":binding.task,"execution":binding.execution},"contextExpected":receipt});
        let mut admission = json!({"sessionId":binding.session,"_meta":binding.meta()});
        admission["_meta"]["neko.contextAdmission"] = receipt;
        let launch = LaunchContext::parse(&raw)
            .expect("launch fixture")
            .expect("context protocol");
        let context = launch
            .admit(&admission, &binding)
            .expect("admission fixture");
        let params = json!({"sessionId":binding.session,"_meta":context.meta(&binding).unwrap(),"toolCall":{"toolCallId":"owned-call","title":"mcp__nekobot__execute_tool","name":"mcp__nekobot__execute_tool","kind":"other","status":"pending","rawInput":{"name":"artifact.write_text","arguments":{"filename":"fixture.md","text":"Owned fixture"}}},"options":[{"optionId":"allow_once","name":"Allow once","kind":"allow_once"},{"optionId":"allow_always","name":"Always allow","kind":"allow_always"}]});
        (binding, context, params)
    }

    #[test]
    fn only_current_original_typed_request_and_fixed_once() {
        let (binding, context, params) = fixture();
        let request = parse(&params, &binding, &context).unwrap();
        let hash = request.request_digest.clone();
        assert_eq!(request.data["arguments"], params["toolCall"]["rawInput"]);
        let pending = request
            .approved(
                &json!({"version":1,"allowed":true,"requestSha256":hash}),
                &binding,
                &context,
            )
            .unwrap();
        let call = json!({"name":"execute_tool","arguments":params["toolCall"]["rawInput"],"_meta":context.meta(&binding).unwrap()});
        assert!(pending.matches(&call, &binding, &context));
        assert_eq!(allowed_once()["outcome"]["optionId"], "allow_once");
        assert_eq!(cancelled()["outcome"]["outcome"], "cancelled");
    }

    #[test]
    fn unknown_missing_foreign_permission_denies() {
        let (binding, context, params) = fixture();
        for field in ["sessionId", "_meta", "toolCall", "options"] {
            let mut changed = params.clone();
            changed.as_object_mut().unwrap().remove(field);
            assert!(parse(&changed, &binding, &context).is_none());
        }
        let mut foreign = params.clone();
        foreign["toolCall"]["name"] = json!("bash");
        assert!(parse(&foreign, &binding, &context).is_none());
        let mut missing = params.clone();
        missing["toolCall"]
            .as_object_mut()
            .unwrap()
            .remove("rawInput");
        assert!(parse(&missing, &binding, &context).is_none());
        let mut extra = params.clone();
        extra["_meta"]["grant"] = json!(true);
        assert!(parse(&extra, &binding, &context).is_none());
        let mut no_once = params.clone();
        no_once["options"] =
            json!([{"optionId":"allow_always","name":"Always","kind":"allow_always"}]);
        assert!(parse(&no_once, &binding, &context).is_none());
        let mut duplicated = params.clone();
        duplicated["options"] = json!([params["options"][0], params["options"][0]]);
        assert!(parse(&duplicated, &binding, &context).is_none());
    }

    #[test]
    fn decision_hash_and_swapped_args_never_approve() {
        let (binding, context, params) = fixture();
        let request = parse(&params, &binding, &context).unwrap();
        assert!(request
            .approved(
                &json!({"version":1,"allowed":true,"requestSha256":"f".repeat(64)}),
                &binding,
                &context
            )
            .is_none());
        let request = parse(&params, &binding, &context).unwrap();
        let hash = request.request_digest.clone();
        let pending = request
            .approved(
                &json!({"version":1,"allowed":true,"requestSha256":hash}),
                &binding,
                &context,
            )
            .unwrap();
        let call = json!({"name":"execute_tool","arguments":{"name":"artifact.write_text","arguments":{"filename":"swapped.md","text":"Owned fixture"}},"_meta":context.meta(&binding).unwrap()});
        assert!(!pending.matches(&call, &binding, &context));
    }

    #[test]
    fn consumed_once_and_foreign_session_cannot_rebind() {
        let (binding, context, params) = fixture();
        let request = parse(&params, &binding, &context).unwrap();
        let hash = request.request_digest.clone();
        let mut pending = Some(
            request
                .approved(
                    &json!({"version":1,"allowed":true,"requestSha256":hash}),
                    &binding,
                    &context,
                )
                .unwrap(),
        );
        let call = json!({"name":"execute_tool","arguments":params["toolCall"]["rawInput"],"_meta":context.meta(&binding).unwrap()});
        assert!(pending.take().unwrap().matches(&call, &binding, &context));
        assert!(pending.take().is_none());
        let request = parse(&params, &binding, &context).unwrap();
        let hash = request.request_digest.clone();
        let mut changed = binding.clone();
        changed.session = "f".repeat(32);
        assert!(request
            .approved(
                &json!({"version":1,"allowed":true,"requestSha256":hash}),
                &changed,
                &context
            )
            .is_none());
    }
}
