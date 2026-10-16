use neko_host::acp::{self, Binding};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{
    env, fs,
    io::{self, BufRead, BufReader, Read, Write},
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    sync::mpsc::{self, Receiver},
    thread,
    time::{Duration, Instant},
};

const CORE_PINS: &str = include_str!("../../native/internal/protocol/core-runtimes-v1.json");
type Result<T> = std::result::Result<T, &'static str>;
struct Process {
    child: Child,
    stopped: bool,
}
impl Process {
    fn stop(&mut self) -> bool {
        if self.stopped {
            return true;
        }
        self.child.stdin.take();
        self.stopped = matches!(self.child.try_wait(), Ok(Some(_)))
            || (self.child.kill().is_ok() && self.child.wait().is_ok());
        self.stopped
    }
}
impl Drop for Process {
    fn drop(&mut self) {
        self.stop();
    }
}
struct Bridge {
    child: Process,
    core: Receiver<Result<Value>>,
    host: Receiver<Result<Value>>,
    binding: Option<Binding>,
    seq: u64,
    next: u64,
    native: bool,
}
impl Drop for Bridge {
    fn drop(&mut self) {
        if let Some(binding) = self.binding.clone() {
            let params = json!({"sessionId":binding.session,"_meta":binding.meta()});
            let _ = self.send(json!({"jsonrpc":"2.0","method":"session/cancel","params":params}));
            let id = self.next + 1;
            let _ = self
                .send(json!({"jsonrpc":"2.0","id":id,"method":"session/close","params":params}));
            let deadline = Instant::now() + Duration::from_secs(6);
            while Instant::now() < deadline {
                match self.core.recv_timeout(Duration::from_millis(100)) {
                    Ok(Ok(v)) if v["id"] == id && v.get("method").is_none() => break,
                    Ok(Ok(v)) if v.get("method").is_some() && v.get("id").is_some() => {
                        let response = if v["method"] == "mcp/disconnect" {
                            json!({"jsonrpc":"2.0","id":v["id"],"result":{}})
                        } else {
                            json!({"jsonrpc":"2.0","id":v["id"],"error":{"code":-32000,"message":"Host shutting down; no new action admitted"}})
                        };
                        let _ = self.send(response);
                    }
                    Err(mpsc::RecvTimeoutError::Disconnected) => break,
                    _ => {}
                }
            }
        }
        if self.child.stop() && self.native {
            self.seq += 1;
            let session = self
                .binding
                .as_ref()
                .map(|b| b.session.as_str())
                .unwrap_or("");
            let mut output = io::stdout().lock();
            let _ = writeln!(
                output,
                "{}",
                json!({"sequence":self.seq,"kind":"runtime_stopped","data":{"version":1,"nativeProcessId":self.child.child.id(),"sessionId":session}})
            );
            let _ = output.flush();
        }
    }
}

fn lines(reader: impl Read + Send + 'static) -> Receiver<Result<Value>> {
    let (tx, rx) = mpsc::sync_channel(16);
    thread::spawn(move || {
        let mut reader = BufReader::new(reader);
        loop {
            let mut bytes = vec![];
            match (&mut reader).take(65537).read_until(b'\n', &mut bytes) {
                Ok(0) | Err(_) => break,
                Ok(_) => {
                    let v = if bytes.len() > 65536 || bytes.last() != Some(&b'\n') {
                        Err("frame_bound")
                    } else {
                        acp::frame(&bytes).map_err(|_| "frame_invalid")
                    };
                    let failed = v.is_err();
                    if tx.send(v).is_err() || failed {
                        break;
                    }
                }
            }
        }
    });
    rx
}
fn host_event(
    host: &Receiver<Result<Value>>,
    seq: &mut u64,
    kind: &str,
    data: Value,
) -> Result<Value> {
    *seq += 1;
    println!("{}", json!({"sequence":seq,"kind":kind,"data":data}));
    io::stdout().flush().map_err(|_| "host_closed")?;
    let ack = host
        .recv_timeout(Duration::from_secs(30))
        .map_err(|_| "host_ack_missing")??;
    if ack["sequence"] != *seq || ack["ok"] != true {
        return Err("host_rejected");
    }
    Ok(ack["result"].clone())
}
impl Bridge {
    fn send(&mut self, value: Value) -> Result<()> {
        let input = self.child.child.stdin.as_mut().ok_or("core_closed")?;
        writeln!(input, "{value}")
            .and_then(|_| input.flush())
            .map_err(|_| "core_closed")
    }
    fn event(&mut self, kind: &str, data: Value) -> Result<Value> {
        host_event(&self.host, &mut self.seq, kind, data)
    }
    fn call(&mut self, method: &str, params: Value) -> Result<Value> {
        self.next += 1;
        let id = self.next;
        self.send(json!({"jsonrpc":"2.0","id":id,"method":method,"params":params}))?;
        let deadline =
            Instant::now() + Duration::from_secs(if method == "session/prompt" { 120 } else { 25 });
        loop {
            if Instant::now() > deadline {
                return Err("core_deadline");
            }
            match self.host.try_recv() {
                Err(mpsc::TryRecvError::Disconnected) => return Err("host_closed"),
                Ok(_) => return Err("unexpected_host_frame"),
                Err(mpsc::TryRecvError::Empty) => {}
            }
            let v = match self.core.recv_timeout(Duration::from_millis(50)) {
                Ok(v) => v?,
                Err(mpsc::RecvTimeoutError::Timeout) => continue,
                Err(_) => return Err("core_closed"),
            };
            if v["jsonrpc"] != "2.0" {
                return Err("core_frame_invalid");
            }
            if v.get("method").is_some() {
                self.incoming(v)?;
                continue;
            }
            if v.get("error").is_some() {
                return Err(
                    match v["error"]["data"]["neko.taskError"]["kind"].as_str() {
                        Some("writer_unavailable") => "core_writer_unavailable",
                        Some("recovery_required") => "core_activation_recovery_required",
                        _ => "core_rpc_rejected",
                    },
                );
            }
            if v["id"] != id {
                return Err("core_rpc_rejected");
            }
            return v.get("result").cloned().ok_or("core_result_missing");
        }
    }
    fn incoming(&mut self, v: Value) -> Result<()> {
        let p = &v["params"];
        if v.get("id").is_none() {
            if v["method"] == "session/update" {
                let Some(b) = self.binding.as_ref() else {
                    return Ok(());
                };
                if p["sessionId"] != b.session {
                    return Err("session_mismatch");
                }
                b.echo(p).map_err(|_| "receipt_mismatch")?;
                if p["update"]["sessionUpdate"] == "agent_message_chunk"
                    && p["update"]["content"]["type"] == "text"
                {
                    let text = p["update"]["content"]["text"]
                        .as_str()
                        .ok_or("text_invalid")?;
                    self.event("text", json!({"text":text}))?;
                }
            }
            return Ok(());
        }
        let result = match v["method"].as_str().unwrap_or("") {
            "mcp/connect" if p["serverId"] == "nekobot:local" => {
                json!({"connectionId":"nekobot-local"})
            }
            "mcp/disconnect" => json!({}),
            "mcp/message" if p["connectionId"] == "nekobot-local" => {
                match p["method"].as_str().unwrap_or("") {
                    "initialize" => {
                        json!({"protocolVersion":"2025-11-25","capabilities":{"tools":{}},"serverInfo":{"name":"nekobot-native","version":"0.2.0"}})
                    }
                    "tools/list" => json!({"tools":[
                        {"name":"capabilities","description":"Discover actions allowed for this task.","inputSchema":{"type":"object","properties":{},"additionalProperties":false}},
                        {"name":"execute_tool","description":"Execute one declared task action.","inputSchema":{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object"}},"required":["name","arguments"],"additionalProperties":false}}
                    ]}),
                    "tools/call" => {
                        let b = self.binding.as_ref().ok_or("operation_before_admission")?;
                        let args = &p["params"];
                        b.operation(&args["_meta"]["neko.operation"])
                            .map_err(|_| "operation_mismatch")?;
                        match args["name"].as_str().unwrap_or("") {
                            "capabilities" if args["arguments"] == json!({}) => {
                                json!({"content":[{"type":"text","text":json!({"tools":[{"name":"artifact.write_text","description":"Save a UTF-8 report for this task and return its download receipt.","inputSchema":{"type":"object","properties":{"filename":{"type":"string","maxLength":80},"text":{"type":"string","maxLength":24000}},"required":["filename","text"],"additionalProperties":false}}]}).to_string()}]})
                            }
                            "execute_tool" => self.event("tool", args.clone())?,
                            _ => {
                                json!({"isError":true,"content":[{"type":"text","text":"Action denied by host"}]})
                            }
                        }
                    }
                    _ => return Err("mcp_method_denied"),
                }
            }
            _ => return Err("core_host_method_denied"),
        };
        self.send(json!({"jsonrpc":"2.0","id":v["id"],"result":result}))
    }
}
fn wsl_path(path: &Path) -> Result<String> {
    let path = path
        .to_str()
        .ok_or("path_invalid")?
        .trim_start_matches("\\\\?\\")
        .replace('\\', "/");
    let bytes = path.as_bytes();
    if bytes.len() < 3 || !bytes[0].is_ascii_alphabetic() || &path[1..3] != ":/" {
        return Err("local_drive_required");
    }
    Ok(format!(
        "/mnt/{}/{}",
        (bytes[0] as char).to_ascii_lowercase(),
        &path[3..]
    ))
}
fn run() -> Result<()> {
    let args: Vec<_> = env::args().skip(1).collect();
    if args.len() != 8
        || args[0] != "--runtime"
        || args[2] != "--supervisor"
        || args[4] != "--state-root"
        || args[6] != "--fixture-endpoint"
    {
        return Err("launch_arguments_invalid");
    }
    let binary = fs::canonicalize(&args[1]).map_err(|_| "runtime_missing")?;
    let bytes = fs::read(&binary).map_err(|_| "runtime_missing")?;
    let core_sha = format!("{:x}", Sha256::digest(&bytes));
    let pins: Value = serde_json::from_str(CORE_PINS).map_err(|_| "runtime_pin_invalid")?;
    let native = bytes.starts_with(b"MZ");
    let platform = if native { "windows_x64" } else { "linux_x64" };
    if pins[platform]["sha256"] != core_sha {
        return Err("runtime_digest_mismatch");
    }
    let supervisor = fs::canonicalize(&args[3]).map_err(|_| "supervisor_missing")?;
    if fs::read(&supervisor).map_err(|_| "supervisor_missing")?
        != include_bytes!("../../scripts/core_supervisor.py")
    {
        return Err("supervisor_digest_mismatch");
    }
    let endpoint = &args[7];
    let port = endpoint
        .strip_prefix("http://127.0.0.1:")
        .and_then(|s| s.strip_suffix("/v1"))
        .and_then(|s| s.parse::<u16>().ok())
        .filter(|p| *p > 0)
        .ok_or("fixture_endpoint_invalid")?;
    let host = lines(io::stdin());
    let spec = host
        .recv_timeout(Duration::from_secs(10))
        .map_err(|_| "request_missing")??;
    let id = spec["requestId"].as_str().ok_or("request_invalid")?;
    if id.len() != 36
        || !id.starts_with("req-")
        || !id[4..]
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    {
        return Err("request_invalid");
    }
    let text = spec["text"]
        .as_str()
        .filter(|s| !s.trim().is_empty() && s.len() <= 48000)
        .ok_or("request_invalid")?;
    let base = fs::canonicalize(&args[5]).map_err(|_| "state_root_missing")?;
    let root = base.join(id);
    fs::create_dir_all(&root).map_err(|_| "task_root_unavailable")?;
    let root = fs::canonicalize(root).map_err(|_| "task_root_unavailable")?;
    if root.parent() != Some(base.as_path()) {
        return Err("task_root_escape");
    }
    let task_home = root.join(".core-home");
    fs::create_dir_all(&task_home).map_err(|_| "task_home_unavailable")?;
    let physical_root = if native {
        root.to_str()
            .ok_or("path_invalid")?
            .trim_start_matches("\\\\?\\")
            .to_lowercase()
    } else {
        wsl_path(&root)?
    };
    if native
        && fs::canonicalize(&physical_root).map_err(|_| "task_root_canonical_mismatch")? != root
    {
        return Err("task_root_canonical_mismatch");
    }
    let mut child = if native {
        Command::new(&binary)
    } else {
        Command::new(PathBuf::from("C:\\Windows\\System32\\wsl.exe"))
    };
    if native {
        child.args(["acp", "--host-profile", "nekobot"]);
    } else {
        child.args([
            "-d",
            "Ubuntu",
            "--exec",
            "/usr/bin/python3",
            &wsl_path(&supervisor)?,
            &wsl_path(&binary)?,
            &wsl_path(&task_home)?,
            &physical_root,
            &format!("http://127.0.0.1:{port}/v1"),
        ]);
    }
    child.env_clear();
    for key in ["SystemRoot", "WINDIR", "TEMP", "TMP", "USERPROFILE"] {
        if let Ok(value) = env::var(key) {
            child.env(key, value);
        }
    }
    if native {
        child.current_dir(&root).env(
            "USERPROFILE",
            task_home
                .to_str()
                .ok_or("path_invalid")?
                .trim_start_matches("\\\\?\\"),
        );
        for (key, value) in [
            ("NEKO_PROVIDER", "openai_compat"),
            ("NEKO_BASE_URL", endpoint.as_str()),
            ("NEKO_MODEL", "neko-controlled-fixture"),
            ("NEKO_API_KEY", "fixture-only"),
            ("NEKO_AUTO_UPDATE", "0"),
            ("NEKO_AUTO_UPDATE_CHECK", "false"),
            ("NEKO_MODE", "auto"),
            ("NEKO_MAX_STEPS", "6"),
            ("NEKO_MAX_RETRIES", "0"),
            ("NEKO_OFFLINE_RETRY_SECONDS", "0"),
            ("NEKO_TIMEOUT_SECONDS", "20"),
            ("NEKO_COMPLETION_SOUND", "false"),
        ] {
            child.env(key, value);
        }
    }
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        child.creation_flags(0x08000000);
    }
    child
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    let mut child = child.spawn().map_err(|_| "core_launch_failed")?;
    let core = lines(child.stdout.take().ok_or("core_pipe_missing")?);
    let mut bridge = Bridge {
        child: Process {
            child,
            stopped: false,
        },
        core,
        host,
        binding: None,
        seq: 0,
        next: 0,
        native,
    };
    let init=bridge.call("initialize",json!({"protocolVersion":1,"clientInfo":{"name":"nekobot-native","version":"0.2.0"},"clientCapabilities":{}}))?;
    acp::initialize(&init).map_err(|_| "core_capability_mismatch")?;
    let resume = spec.get("resume");
    let mut params = json!({"cwd":physical_root,"mcpServers":[{"type":"acp","name":"nekobot","serverId":"nekobot:local"}],"_meta":{"neko.taskProtocol":{"version":1,"mode":"fixed-active-task"},"neko.executionProtocol":{"version":1},"neko.hostEffectProtocol":{"version":1},"neko.taskLabel":format!("NekoBot {id}")}});
    if let Some(saved) = resume {
        if saved["coreSha256"] != core_sha {
            return Err("resume_runtime_changed");
        }
        params["sessionId"] = saved["sessionId"].clone();
        params["_meta"]["neko.taskExpected"] = saved["task"].clone();
        params["_meta"]["neko.executionExpected"] = saved["execution"].clone();
    }
    let mut admission = bridge.call(
        if resume.is_some() {
            "session/resume"
        } else {
            "session/new"
        },
        params,
    )?;
    if let Some(saved) = resume {
        admission["sessionId"] = saved["sessionId"].clone();
    }
    let binding = Binding::admit(&admission, &physical_root).map_err(|_| "admission_mismatch")?;
    if let Some(saved) = resume {
        if binding.task["id"] != saved["task"]["id"]
            || binding.task["activationEpoch"].as_u64()
                != saved["task"]["activationEpoch"]
                    .as_u64()
                    .and_then(|n| n.checked_add(1))
            || binding.execution["authorityId"] != saved["execution"]["authorityId"]
        {
            return Err("resume_authority_mismatch");
        }
    }
    binding
        .effects_clear(&admission)
        .map_err(|_| "core_effects_unresolved")?;
    bridge.binding = Some(binding.clone());
    bridge.event("admitted",json!({"sessionId":binding.session,"task":binding.task,"execution":binding.execution,"coreSha256":core_sha,"nativeProcessId":if native {bridge.child.child.id()}else{0}}))?;
    let result=bridge.call("session/prompt",json!({"sessionId":binding.session,"prompt":[{"type":"text","text":text}],"_meta":binding.meta()}))?;
    binding
        .echo(&result)
        .map_err(|_| "prompt_receipt_mismatch")?;
    binding
        .effects_clear(&result)
        .map_err(|_| "core_effects_unresolved")?;
    if result["stopReason"] != "end_turn" {
        return Err("core_turn_incomplete");
    }
    bridge.event("completed", json!({"stopReason":"end_turn"}))?;
    Ok(())
}
fn main() {
    let result = if matches!(
        env::args().nth(1).as_deref(),
        Some("--vault-seal" | "--vault-match")
    ) {
        vault_seal()
    } else {
        run()
    };
    if let Err(code) = result {
        eprintln!("{code}");
        std::process::exit(1);
    }
}
fn vault_seal() -> Result<()> {
    let args: Vec<_> = env::args().skip(1).collect();
    if args.len() != 7
        || args[1] != "--state-root"
        || args[3] != "--binding"
        || args[5] != "--secret-ref"
    {
        return Err("vault_arguments_invalid");
    }
    let state = fs::canonicalize(&args[2]).map_err(|_| "vault_state_unavailable")?;
    let mut raw = vec![];
    io::stdin()
        .take(8193)
        .read_to_end(&mut raw)
        .map_err(|_| "vault_input_invalid")?;
    let mut value = acp::frame(&raw).map_err(|_| "vault_input_invalid")?;
    raw.fill(0);
    if value.as_object().is_none_or(|o| o.len() != 1) {
        return Err("vault_input_invalid");
    }
    let secret = value
        .as_object_mut()
        .ok_or("vault_input_invalid")?
        .remove("secret")
        .ok_or("vault_input_invalid")?;
    let Value::String(secret) = secret else {
        return Err("vault_input_invalid");
    };
    let mut bytes = secret.into_bytes();
    if args[0] == "--vault-match" {
        let same = neko_host::vault::matches(&state, &args[4], &args[6], &mut bytes)?;
        println!("{}", json!({"matches":same}));
    } else {
        neko_host::vault::seal(&state, &args[4], &args[6], &mut bytes)?;
        println!("{}", json!({"sealed":true}));
    }
    Ok(())
}
