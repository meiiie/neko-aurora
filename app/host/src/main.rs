mod profile_permissions;
use neko_host::acp::{self, Binding};
use neko_host::context_admission::{AdmittedContext, LaunchContext};
use neko_host::core_profile::{CoreProfileLaunch, PrivateProfileSource};
use neko_host::executor::{self, Companion};
use neko_host::runtime_stop::{self, StopOutbox};
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
    generation: String,
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
    profile_mode: bool,
    profile_permission: Option<profile_permissions::Pending>,
    context: Option<AdmittedContext>,
    child: Process,
    core: Receiver<Result<Value>>,
    host: Receiver<Result<Value>>,
    binding: Option<Binding>,
    seq: u64,
    next: u64,
    native: bool,
    companion: Option<Companion>,
    stop_outbox: Option<StopOutbox>,
    outbox_binding: Option<Value>,
}
impl Drop for Bridge {
    fn drop(&mut self) {
        if let Some(binding) = self.binding.clone() {
            let meta = self
                .context
                .as_ref()
                .map(|c| c.meta(&binding))
                .transpose()
                .ok()
                .flatten()
                .unwrap_or_else(|| binding.meta());
            let params = json!({"sessionId":binding.session,"_meta":meta});
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
            let mut receipt = json!({"version":1,"nativeProcessId":self.child.child.id(),"nativeProcessGeneration":self.child.generation,"sessionId":session});
            if let Some(c) = &self.companion {
                receipt["executorProof"] = c.proof()
            }
            if let (Some(outbox), Some(binding)) = (&self.stop_outbox, &self.outbox_binding) {
                if outbox.publish(binding, &receipt).is_err() {
                    eprintln!("native_stop_outbox_unavailable");
                }
            }
            let mut output = io::stdout().lock();
            let _ = writeln!(
                output,
                "{}",
                json!({"sequence":self.seq,"kind":"runtime_stopped","data":receipt})
            );
            let _ = output.flush();
        }
    }
}

static CORE_IO_LOG: std::sync::OnceLock<std::sync::Mutex<fs::File>> = std::sync::OnceLock::new();
fn core_io_init(root: &Path) {
    if let Ok(f) = fs::File::create(root.join(".host-core-io.log")) {
        let _ = CORE_IO_LOG.set(std::sync::Mutex::new(f));
    }
}
fn core_io(direction: &str, v: &Value) {
    if let Some(m) = CORE_IO_LOG.get() {
        if let Ok(mut f) = m.lock() {
            use std::io::Write as _;
            let _ = writeln!(f, "{} {}", direction, v);
            let _ = f.flush();
        }
    }
}
fn lines(reader: impl Read + Send + 'static) -> Receiver<Result<Value>> {
    let (tx, rx) = mpsc::sync_channel(16);
    thread::spawn(move || {
        let mut reader = BufReader::new(reader);
        loop {
            let mut bytes = vec![];
            match (&mut reader).take((8 << 20) + 1).read_until(b'\n', &mut bytes) {
                Ok(0) | Err(_) => break,
                Ok(_) => {
                    let v = if bytes.len() > (8 << 20) || bytes.last() != Some(&b'\n') {
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
        core_io("HOST>TO>CORE", &value);
        let input = self.child.child.stdin.as_mut().ok_or("core_closed")?;
        writeln!(input, "{value}")
            .and_then(|_| input.flush())
            .map_err(|_| "core_closed")
    }
    fn event(&mut self, kind: &str, data: Value) -> Result<Value> {
        host_event(&self.host, &mut self.seq, kind, data)
    }
    fn approved_profile_call(&mut self, args: &Value) -> bool {
        let pending = self.profile_permission.take();
        match (pending, self.binding.as_ref(), self.context.as_ref()) {
            (Some(pending), Some(binding), Some(context)) => {
                pending.matches(args, binding, context)
            }
            _ => false,
        }
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
            core_io("CORE>TO>HOST", &v);
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
                if let Some(c) = &self.context {
                    c.echo(b, p).map_err(|_| "context_receipt_mismatch")?;
                }
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
            "session/request_permission" => {
                if !self.profile_mode || self.profile_permission.is_some() {
                    self.profile_permission = None;
                    profile_permissions::cancelled()
                } else {
                    let request = match (self.binding.as_ref(), self.context.as_ref()) {
                        (Some(binding), Some(context)) => {
                            profile_permissions::parse(p, binding, context)
                        }
                        _ => None,
                    };
                    if let Some(request) = request {
                        let decision =
                            self.event("profile_tool_permission", request.data.clone())?;
                        self.profile_permission =
                            match (self.binding.as_ref(), self.context.as_ref()) {
                                (Some(binding), Some(context)) => {
                                    request.approved(&decision, binding, context)
                                }
                                _ => None,
                            };
                        if self.profile_permission.is_some() {
                            profile_permissions::allowed_once()
                        } else {
                            profile_permissions::cancelled()
                        }
                    } else {
                        profile_permissions::cancelled()
                    }
                }
            }
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
                        let mut args = p["params"].clone();
                        b.operation(&args["_meta"]["neko.operation"])
                            .map_err(|_| "operation_mismatch")?;
                        if args["_meta"].as_object().is_none_or(|o| o.len() != 1) {
                            return Err("operation_metadata_mismatch");
                        }
                        if let Some(c) = &self.context {
                            let meta = c.meta(b).map_err(|_| "context_binding_mismatch")?;
                            args["_meta"]["neko.contextAdmission"] =
                                meta["neko.contextAdmission"].clone();
                        }
                        match args["name"].as_str().unwrap_or("") {
                            "capabilities" if args["arguments"] == json!({}) => {
                                self.event("capabilities", args.clone())?
                            }
                            "execute_tool" => {
                                if self.profile_mode && !self.approved_profile_call(&args) {
                                    json!({"isError":true,"content":[{"type":"text","text":"Exact single-use profile tool permission required"}]})
                                } else {
                                    self.event("tool", args.clone())?
                                }
                            }
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
fn native_writer_descriptor(binary: &Path, pin: &Value) -> Result<Option<(PathBuf, String)>> {
    let Some(descriptor) = pin.get("writerHelper") else {
        return Ok(None);
    };
    if descriptor["filename"] != "neko-writer-lease.exe" {
        return Err("writer_helper_pin_invalid");
    }
    let digest = descriptor["sha256"]
        .as_str()
        .filter(|s| {
            s.len() == 64
                && s.bytes()
                    .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
        })
        .ok_or("writer_helper_pin_invalid")?;
    let path = binary
        .parent()
        .ok_or("writer_helper_missing")?
        .join("neko-writer-lease.exe");
    if !fs::symlink_metadata(&path)
        .map_err(|_| "writer_helper_missing")?
        .file_type()
        .is_file()
    {
        return Err("writer_helper_missing");
    }
    let helper = fs::canonicalize(&path).map_err(|_| "writer_helper_missing")?;
    let bytes = fs::read(&helper).map_err(|_| "writer_helper_missing")?;
    if format!("{:x}", Sha256::digest(&bytes)) != digest {
        return Err("writer_helper_digest_mismatch");
    }
    Ok(Some((helper, digest.to_owned())))
}
fn run() -> Result<()> {
    let args: Vec<_> = env::args().skip(1).collect();
    let configured = args.len() == 10 && args[8] == "--install-state-root";
    let fixture =
        args.len() == 8 && env::var("NEKO_EXECUTOR_TEST_UNCONFIGURED").as_deref() == Ok("1");
    if !configured && !fixture {
        return Err("executor_unconfigured");
    }
    if (args.len() != 8 && args.len() != 10)
        || args[0] != "--runtime"
        || args[2] != "--supervisor"
        || args[4] != "--state-root"
        || (args[6] != "--fixture-endpoint" && args[6] != "--core-profile")
    {
        return Err("launch_arguments_invalid");
    }
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
        .filter(|s| !s.trim().is_empty() && s.len() <= 64000)
        .ok_or("request_invalid")?;
    let binary = fs::canonicalize(&args[1]).map_err(|_| "runtime_missing")?;
    let bytes = fs::read(&binary).map_err(|_| "runtime_missing")?;
    let core_sha = format!("{:x}", Sha256::digest(&bytes));
    let pins: Value = serde_json::from_str(CORE_PINS).map_err(|_| "runtime_pin_invalid")?;
    let native = bytes.starts_with(b"MZ");
    let platform = if native { "windows_x64" } else { "linux_x64" };
    if pins[platform]["sha256"] != core_sha {
        return Err("runtime_digest_mismatch");
    }
    let resume = spec.get("resume");
    let context = LaunchContext::parse(&spec).map_err(|_| "context_launch_invalid")?;
    if pins[platform]["contextProtocol"] == 1 && context.is_none() {
        return Err("context_admission_required");
    }
    if context.is_some() && pins[platform]["contextProtocol"] != 1 {
        return Err("context_runtime_unsupported");
    }
    let private_profile = if args[6] == "--core-profile" {
        if !native || !matches!(pins[platform]["profileBridgeVersion"].as_u64(), Some(1 | 2)) {
            return Err("core_profile_runtime_unsupported");
        }
        let limits = env::var("NEKO_PROFILE_LIMITS").map_err(|_| "core_profile_binding_missing")?;
        Some(PrivateProfileSource {
            home: PathBuf::from(
                env::var("NEKO_CREDENTIAL_HOME").map_err(|_| "core_profile_binding_missing")?,
            ),
            key: env::var("NEKO_PROFILE_REVISION_KEY")
                .map_err(|_| "core_profile_binding_missing")?,
            source_id: env::var("NEKO_BRIDGE_SOURCE_ID")
                .map_err(|_| "core_profile_binding_missing")?,
            source_revision: env::var("NEKO_BRIDGE_SOURCE_REVISION")
                .ok()
                .and_then(|s| s.parse().ok())
                .ok_or("core_profile_binding_missing")?,
            profile_id: args[7].clone(),
            profile_revision: env::var("NEKO_PROFILE_REVISION")
                .map_err(|_| "core_profile_binding_missing")?,
            limits: acp::frame(limits.as_bytes()).map_err(|_| "core_profile_limits_invalid")?,
            fixture_only: match env::var("NEKO_BRIDGE_FIXTURE_ONLY").as_deref() {
                Ok("1") => true,
                Ok("0") => false,
                _ => return Err("core_profile_binding_missing"),
            },
            capsule_path: env::var("NEKO_PROFILE_CAPSULE_PATH")
                .ok()
                .map(PathBuf::from),
            snapshot_id: env::var("NEKO_PROFILE_SNAPSHOT_ID").ok(),
        })
    } else {
        None
    };
    let profile = CoreProfileLaunch::parse(
        &spec,
        &core_sha,
        private_profile.as_ref(),
        executor::now_ms()?,
    )?;
    if context.is_some()
        && resume.is_none()
        && spec["text"] != spec["contextManifest"]["primaryMessage"]["text"]
    {
        return Err("context_primary_prompt_mismatch");
    }
    if let Some(saved) = resume {
        if saved["coreSha256"] != core_sha {
            return Err("resume_runtime_changed");
        }
    }
    let writer_helper = if native {
        native_writer_descriptor(&binary, &pins[platform])?
    } else {
        None
    };
    let supervisor = fs::canonicalize(&args[3]).map_err(|_| "supervisor_missing")?;
    if fs::read(&supervisor).map_err(|_| "supervisor_missing")?
        != include_bytes!("../../scripts/core_supervisor.py")
    {
        return Err("supervisor_digest_mismatch");
    }
    let endpoint = &args[7];
    let port = if profile.is_some() {
        0
    } else {
        endpoint
            .strip_prefix("http://127.0.0.1:")
            .and_then(|s| s.strip_suffix("/v1"))
            .and_then(|s| s.parse::<u16>().ok())
            .filter(|p| *p > 0)
            .ok_or("fixture_endpoint_invalid")?
    };
    let base = fs::canonicalize(&args[5]).map_err(|_| "state_root_missing")?;
    let companion = if configured {
        if !native {
            return Err("executor_native_required");
        }
        let install = fs::canonicalize(&args[9]).map_err(|_| "executor_state_root_missing")?;
        if base != install.join("tasks") {
            return Err("executor_task_base_mismatch");
        }
        let actual = executor::descriptor(core_sha.clone(), &install)?;
        Some(Companion::validate(
            &spec["executorPin"],
            &spec["attemptLease"],
            &actual,
            id,
            executor::now_ms()?,
        )?)
    } else {
        if spec.get("executorPin").is_some() || spec.get("attemptLease").is_some() {
            return Err("executor_companion_invalid");
        };
        None
    };
    let stop_outbox = if companion.is_some() && context.is_some() {
        Some(StopOutbox::prepare(
            Path::new(&args[9]),
            id,
            &spec["executorPin"],
            &spec["attemptLease"],
        )?)
    } else {
        None
    };
    if serde_json::to_vec(&spec)
        .map_err(|_| "frame_invalid")?
        .len()
        > 64000
    {
        return Err("frame_bound");
    }
    let mut launch_guard = LaunchGuard {
        companion: companion.clone(),
        sequence: 0,
        armed: companion.is_some(),
    };
    if let Some(c) = &companion {
        host_event(&host, &mut launch_guard.sequence, "launch_ready", c.proof())?;
        c.live(executor::now_ms()?)?;
    }
    let root = base.join(id);
    launch_guard.armed = false;
    fs::create_dir_all(&root).map_err(|_| "task_root_unavailable")?;
    let root = fs::canonicalize(root).map_err(|_| "task_root_unavailable")?;
    core_io_init(&root);
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
        if context.is_some() {
            child.args(["--context-memory", "--context-admission"]);
        }
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
        if let Some((helper, digest)) = writer_helper {
            child.env(
                "NEKO_TASK_WRITER_HELPER",
                helper
                    .to_str()
                    .ok_or("path_invalid")?
                    .trim_start_matches("\\\\?\\"),
            );
            child.env("NEKO_TASK_WRITER_HELPER_SHA256", digest);
        }
        child.current_dir(&root).env(
            "USERPROFILE",
            task_home
                .to_str()
                .ok_or("path_invalid")?
                .trim_start_matches("\\\\?\\"),
        );
        if let Some(profile) = &profile {
            profile.apply(&mut child)?;
        } else {
            for (key, value) in [
                ("NEKO_PROVIDER", "openai_compat"),
                ("NEKO_BASE_URL", endpoint.as_str()),
                ("NEKO_MODEL", "neko-controlled-fixture"),
                ("NEKO_API_KEY", "fixture-only"),
            ] {
                child.env(key, value);
            }
        }
        for (key, value) in [
            ("NEKO_AUTO_UPDATE", "0"),
            ("NEKO_AUTO_UPDATE_CHECK", "false"),
            ("NEKO_MODE", "auto"),
            ("NEKO_MAX_RETRIES", "0"),
            ("NEKO_OFFLINE_RETRY_SECONDS", "0"),
            ("NEKO_COMPLETION_SOUND", "false"),
        ] {
            child.env(key, value);
        }
        if profile.is_none() {
            child
                .env("NEKO_MAX_STEPS", "6")
                .env("NEKO_TIMEOUT_SECONDS", "20");
        }
    }
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        child.creation_flags(0x08000000);
    }
    let core_stderr = fs::File::create(root.join(".core-stderr.log")).map(|f| Stdio::from(f)).unwrap_or(Stdio::null());
    child
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(core_stderr);
    let child = child.spawn().map_err(|_| "core_launch_failed")?;
    let mut process = Process {
        child,
        stopped: false,
        generation: String::new(),
    };
    if native {
        process.generation = runtime_stop::process_generation(&process.child)?;
    }
    let core = lines(process.child.stdout.take().ok_or("core_pipe_missing")?);
    let mut bridge = Bridge {
        profile_mode: profile.is_some(),
        profile_permission: None,
        context: None,
        child: process,
        core,
        host,
        binding: None,
        seq: launch_guard.sequence,
        next: 0,
        native,
        companion: companion.clone(),
        stop_outbox,
        outbox_binding: None,
    };
    let init=bridge.call("initialize",json!({"protocolVersion":1,"clientInfo":{"name":"nekobot-native","version":"0.2.0"},"clientCapabilities":{}}))?;
    acp::initialize(&init).map_err(|_| "core_capability_mismatch")?;
    if context.is_some() && init["_meta"]["neko.contextProtocol"] != json!({"version":1}) {
        return Err("core_context_capability_mismatch");
    }
    let mut params = json!({"cwd":physical_root,"mcpServers":[{"type":"acp","name":"nekobot","serverId":"nekobot:local"}],"_meta":{"neko.taskProtocol":{"version":1,"mode":"fixed-active-task"},"neko.executionProtocol":{"version":1},"neko.hostEffectProtocol":{"version":1},"neko.taskLabel":format!("NekoBot {id}")}});
    if let Some(saved) = resume {
        params["sessionId"] = saved["sessionId"].clone();
        params["_meta"]["neko.taskExpected"] = saved["task"].clone();
        params["_meta"]["neko.executionExpected"] = saved["execution"].clone();
    }
    if let Some(c) = &context {
        for (key, value) in c
            .session_meta()
            .as_object()
            .ok_or("context_launch_invalid")?
        {
            params["_meta"][key.as_str()] = value.clone();
        }
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
            || !binding.valid_resume(
                saved,
                &admission,
                pins[platform]["priorReceiptRetry"] == true,
            )
            || binding.execution["authorityId"] != saved["execution"]["authorityId"]
        {
            return Err("resume_authority_mismatch");
        }
    }
    binding
        .effects_clear(&admission)
        .map_err(|_| "core_effects_unresolved")?;
    bridge.binding = Some(binding.clone());
    if let Some(c) = &context {
        bridge.context = Some(
            c.admit(&admission, &binding)
                .map_err(|_| "context_admission_mismatch")?,
        );
    }
    let mut receipt = json!({"sessionId":binding.session,"task":binding.task,"execution":binding.execution,"coreSha256":core_sha,"nativeProcessId":if native {bridge.child.child.id()}else{0}});
    if native {
        receipt["nativeProcessGeneration"] = json!(bridge.child.generation);
    }
    if let Some(proof) = admission["_meta"].get("neko.taskRecovery") {
        receipt["taskRecovery"] = proof.clone();
    }
    if let Some(c) = &companion {
        receipt["executorProof"] = c.proof()
    }
    if let Some(c) = &bridge.context {
        receipt["contextAdmission"] = c.receipt().clone();
    }
    if let Some(outbox) = &bridge.stop_outbox {
        let stopped = json!({"version":1,"nativeProcessId":bridge.child.child.id(),"nativeProcessGeneration":bridge.child.generation,
            "sessionId":binding.session,"executorProof":receipt["executorProof"]});
        outbox.encode(&receipt, &stopped)?;
        bridge.outbox_binding = Some(receipt.clone());
    }
    bridge.event("admitted", receipt.clone())?;
    if let Some(c) = &companion {
        c.live(executor::now_ms()?)?;
        bridge.event("prompt_ready", c.proof())?;
        c.live(executor::now_ms()?)?;
    }
    let meta = bridge
        .context
        .as_ref()
        .map(|c| c.meta(&binding))
        .transpose()
        .map_err(|_| "context_binding_mismatch")?
        .unwrap_or_else(|| binding.meta());
    let result = bridge.call(
        "session/prompt",
        json!({"sessionId":binding.session,"prompt":[{"type":"text","text":text}],"_meta":meta}),
    )?;
    if let Some(c) = &bridge.context {
        c.echo(&binding, &result)
            .map_err(|_| "context_prompt_receipt_mismatch")?;
    }
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
struct LaunchGuard {
    companion: Option<Companion>,
    sequence: u64,
    armed: bool,
}
impl Drop for LaunchGuard {
    fn drop(&mut self) {
        if self.armed {
            if let Some(c) = &self.companion {
                self.sequence += 1;
                println!(
                    "{}",
                    json!({"sequence":self.sequence,"kind":"runtime_stopped","data":{"version":2,"kind":"never_launched","nativeProcessId":0,"sessionId":"","executorProof":c.proof()}})
                );
                let _ = io::stdout().flush();
            }
        }
    }
}
fn executor_probe() -> Result<()> {
    let args: Vec<_> = env::args().skip(1).collect();
    if args.len() != 7
        || args[0] != "--executor-probe"
        || args[1] != "--runtime"
        || args[3] != "--supervisor"
        || args[5] != "--install-state-root"
    {
        return Err("executor_probe_arguments_invalid");
    }
    let binary = fs::canonicalize(&args[2]).map_err(|_| "runtime_missing")?;
    let bytes = fs::read(&binary).map_err(|_| "runtime_missing")?;
    if !bytes.starts_with(b"MZ") {
        return Err("executor_native_required");
    };
    let core = format!("{:x}", Sha256::digest(&bytes));
    let pins: Value = serde_json::from_str(CORE_PINS).map_err(|_| "runtime_pin_invalid")?;
    if pins["windows_x64"]["sha256"] != core {
        return Err("runtime_digest_mismatch");
    };
    native_writer_descriptor(&binary, &pins["windows_x64"])?;
    if fs::read(&args[4]).map_err(|_| "supervisor_missing")?
        != include_bytes!("../../scripts/core_supervisor.py")
    {
        return Err("supervisor_digest_mismatch");
    };
    println!(
        "{}",
        serde_json::to_string(&executor::descriptor(core, Path::new(&args[6]))?)
            .map_err(|_| "executor_probe_invalid")?
    );
    Ok(())
}
fn main() {
    let result = if env::args().nth(1).as_deref() == Some("--executor-probe") {
        executor_probe()
    } else if matches!(
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
