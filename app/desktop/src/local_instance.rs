use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::{Read, Write},
    net::{SocketAddr, TcpStream},
    path::{Path, PathBuf},
    time::{Duration, SystemTime, UNIX_EPOCH},
};

type Result<T> = std::result::Result<T, &'static str>;
#[derive(Clone, Deserialize, Serialize, PartialEq, Debug)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Binding {
    pub version: u8,
    pub endpoint: String,
    pub control_pid: u32,
    pub control_sha256: String,
    pub state_sha256: String,
    pub created_at_unix_ms: u64,
    pub expires_at_unix_ms: u64,
}
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Receipt {
    version: u8,
    endpoint: String,
    control_pid: u32,
    control_sha256: String,
    state_sha256: String,
    created_at_unix_ms: u64,
    expires_at_unix_ms: u64,
    challenge: String,
}
impl Receipt {
    fn binding(&self) -> Binding {
        Binding {
            version: self.version,
            endpoint: self.endpoint.clone(),
            control_pid: self.control_pid,
            control_sha256: self.control_sha256.clone(),
            state_sha256: self.state_sha256.clone(),
            created_at_unix_ms: self.created_at_unix_ms,
            expires_at_unix_ms: self.expires_at_unix_ms,
        }
    }
}
pub struct Config {
    pub receipt: PathBuf,
    pub state: PathBuf,
    pub endpoint: String,
    pub control_hash: String,
    pub verify_only: bool,
}
pub fn parse_args(args: &[String]) -> Result<Config> {
    let mut values = std::collections::HashMap::new();
    let mut verify_only = false;
    let mut i = 0;
    while i < args.len() {
        if args[i] == "--verify-only" {
            if verify_only {
                return Err("local_launch_arguments");
            };
            verify_only = true;
            i += 1;
            continue;
        }
        if ![
            "--launch-receipt",
            "--state",
            "--endpoint",
            "--control-sha256",
        ]
        .contains(&args[i].as_str())
            || i + 1 == args.len()
            || values
                .insert(args[i].as_str(), args[i + 1].as_str())
                .is_some()
        {
            return Err("local_launch_arguments");
        };
        i += 2;
    }
    if values.len() != 4 {
        return Err("local_launch_arguments");
    }
    let endpoint = values["--endpoint"];
    endpoint_port(endpoint)?;
    let hash = values["--control-sha256"];
    if !hex(hash) {
        return Err("local_launch_hash");
    }
    let receipt = PathBuf::from(values["--launch-receipt"]);
    let state = PathBuf::from(values["--state"]);
    if !receipt.is_absolute() || !state.is_absolute() {
        return Err("local_launch_path");
    }
    Ok(Config {
        receipt,
        state,
        endpoint: endpoint.into(),
        control_hash: hash.into(),
        verify_only,
    })
}
pub fn endpoint_port(endpoint: &str) -> Result<u16> {
    let p = endpoint
        .strip_prefix("http://127.0.0.1:")
        .ok_or("local_endpoint")?;
    let port = p.parse::<u16>().map_err(|_| "local_endpoint")?;
    if port == 0 || p != port.to_string() {
        return Err("local_endpoint");
    };
    Ok(port)
}
fn hex(s: &str) -> bool {
    s.len() == 64
        && s.bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
}
fn now() -> Result<u64> {
    Ok(SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|_| "local_clock")?
        .as_millis() as u64)
}
fn no_reparse(path: &Path) -> Result<()> {
    for p in path.ancestors() {
        let m = fs::symlink_metadata(p).map_err(|_| "local_path")?;
        #[cfg(windows)]
        {
            use std::os::windows::fs::MetadataExt;
            if m.file_attributes() & 0x400 != 0 {
                return Err("local_reparse");
            }
        }
        if m.file_type().is_symlink() {
            return Err("local_reparse");
        }
    }
    Ok(())
}
pub fn state_hash(path: &Path) -> Result<String> {
    no_reparse(path)?;
    if !path.is_dir() {
        return Err("local_state");
    }
    let raw = fs::canonicalize(path)
        .map_err(|_| "local_state")?
        .to_str()
        .ok_or("local_state")?
        .replace('\\', "/");
    Ok(format!(
        "{:x}",
        Sha256::digest(
            raw.trim_start_matches("//?/")
                .to_ascii_lowercase()
                .as_bytes()
        )
    ))
}
pub fn profile_directory(state: &Path) -> Result<PathBuf> {
    no_reparse(state)?;
    let directory = fs::canonicalize(state)
        .map_err(|_| "local_state")?
        .join("desktop-webview");
    fs::create_dir_all(&directory).map_err(|_| "local_profile_directory")?;
    no_reparse(&directory)?;
    Ok(directory)
}
pub struct Verified {
    pub binding: Binding,
    challenge: String,
    creation: u64,
}
impl Verified {
    pub fn live(&self) -> bool {
        native_process(
            self.binding.control_pid,
            &self.binding.control_sha256,
            endpoint_port(&self.binding.endpoint).unwrap_or(0),
        ) == Ok(self.creation)
            && query(&self.binding.endpoint, &self.challenge) == Ok(self.binding.clone())
    }
    pub fn allows(&self, url: &tauri::Url) -> bool {
        url.scheme() == "http"
            && url.host_str() == Some("127.0.0.1")
            && url.port() == endpoint_port(&self.binding.endpoint).ok()
            && url.username().is_empty()
            && url.password().is_none()
    }
}
pub fn verify(config: &Config) -> Result<Verified> {
    no_reparse(&config.receipt)?;
    if fs::metadata(&config.receipt)
        .map_err(|_| "local_receipt")?
        .len()
        > 4096
        || config.receipt.parent() != Some(config.state.as_path())
    {
        return Err("local_receipt");
    }
    let bytes = fs::read(&config.receipt).map_err(|_| "local_receipt")?;
    let canonical = neko_host::canonical(&bytes).map_err(|_| "local_receipt")?;
    let receipt: Receipt = serde_json::from_slice(&canonical).map_err(|_| "local_receipt")?;
    let clock = now()?;
    if receipt.version != 1
        || receipt.control_pid == 0
        || receipt.endpoint != config.endpoint
        || receipt.control_sha256 != config.control_hash
        || !hex(&receipt.control_sha256)
        || !hex(&receipt.state_sha256)
        || !hex(&receipt.challenge)
        || receipt.state_sha256 != state_hash(&config.state)?
        || receipt.created_at_unix_ms > clock
        || receipt.expires_at_unix_ms <= clock
        || receipt
            .expires_at_unix_ms
            .checked_sub(receipt.created_at_unix_ms)
            != Some(30000)
    {
        return Err("local_binding");
    }
    let binding = receipt.binding();
    let creation = native_process(
        binding.control_pid,
        &binding.control_sha256,
        endpoint_port(&binding.endpoint)?,
    )?;
    if query(&binding.endpoint, &receipt.challenge)? != binding {
        return Err("local_instance");
    }
    Ok(Verified {
        binding,
        challenge: receipt.challenge,
        creation,
    })
}
fn query(endpoint: &str, challenge: &str) -> Result<Binding> {
    let port = endpoint_port(endpoint)?;
    let address = SocketAddr::from(([127, 0, 0, 1], port));
    let mut stream = TcpStream::connect_timeout(&address, Duration::from_secs(2))
        .map_err(|_| "local_instance")?;
    stream
        .set_read_timeout(Some(Duration::from_secs(2)))
        .map_err(|_| "local_instance")?;
    stream
        .set_write_timeout(Some(Duration::from_secs(2)))
        .map_err(|_| "local_instance")?;
    write!(stream,"GET /v1/local-instance HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nX-Neko-Local-Launch: {challenge}\r\nConnection: close\r\n\r\n").map_err(|_|"local_instance")?;
    let mut data = Vec::new();
    stream
        .take(8193)
        .read_to_end(&mut data)
        .map_err(|_| "local_instance")?;
    if data.len() > 8192 {
        return Err("local_instance");
    }
    let split = data
        .windows(4)
        .position(|x| x == b"\r\n\r\n")
        .ok_or("local_instance")?;
    let header = std::str::from_utf8(&data[..split])
        .map_err(|_| "local_instance")?
        .to_ascii_lowercase();
    if !header.starts_with("http/1.1 200 ")
        || !header.contains("content-security-policy:")
        || !header.contains("connect-src 'self'")
        || !header.contains("content-type: application/json")
    {
        return Err("local_instance");
    }
    let raw = neko_host::canonical(&data[split + 4..]).map_err(|_| "local_instance")?;
    serde_json::from_slice(&raw).map_err(|_| "local_instance")
}

#[cfg(windows)]
fn native_process(pid: u32, hash: &str, port: u16) -> Result<u64> {
    use std::ffi::c_void;
    #[link(name = "iphlpapi")]
    unsafe extern "system" {
        fn GetExtendedTcpTable(
            table: *mut c_void,
            size: *mut u32,
            order: i32,
            family: u32,
            class: u32,
            reserved: u32,
        ) -> u32;
    }
    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn OpenProcess(access: u32, inherit: i32, pid: u32) -> *mut c_void;
        fn CloseHandle(handle: *mut c_void) -> i32;
        fn QueryFullProcessImageNameW(
            handle: *mut c_void,
            flags: u32,
            path: *mut u16,
            size: *mut u32,
        ) -> i32;
        fn GetProcessTimes(
            handle: *mut c_void,
            creation: *mut u64,
            exit: *mut u64,
            kernel: *mut u64,
            user: *mut u64,
        ) -> i32;
        fn GetExitCodeProcess(handle: *mut c_void, code: *mut u32) -> i32;
    }
    let mut size = 0;
    unsafe { GetExtendedTcpTable(std::ptr::null_mut(), &mut size, 0, 2, 3, 0) };
    if !(4..=1048576).contains(&size) {
        return Err("local_listener");
    }
    let mut table = vec![0u8; size as usize];
    if unsafe { GetExtendedTcpTable(table.as_mut_ptr().cast(), &mut size, 0, 2, 3, 0) } != 0 {
        return Err("local_listener");
    }
    let word = |i: usize| u32::from_ne_bytes(table[i..i + 4].try_into().unwrap());
    let count = word(0) as usize;
    if count > (table.len() - 4) / 24
        || !(0..count).any(|n| {
            let i = 4 + 24 * n;
            word(i) == 2
                && word(i + 4) == 0x0100007f
                && u16::from_be(word(i + 8) as u16) == port
                && word(i + 20) == pid
        })
    {
        return Err("local_listener");
    }
    let handle = unsafe { OpenProcess(0x1000, 0, pid) };
    if handle.is_null() {
        return Err("local_process");
    }
    let result = (|| {
        let mut path = vec![0u16; 32768];
        let mut len = path.len() as u32;
        let (mut creation, mut exit, mut kernel, mut user, mut code) =
            (0u64, 0u64, 0u64, 0u64, 0u32);
        if unsafe { QueryFullProcessImageNameW(handle, 0, path.as_mut_ptr(), &mut len) } == 0
            || unsafe { GetProcessTimes(handle, &mut creation, &mut exit, &mut kernel, &mut user) }
                == 0
            || unsafe { GetExitCodeProcess(handle, &mut code) } == 0
            || code != 259
        {
            return Err("local_process");
        }
        let name = String::from_utf16(&path[..len as usize]).map_err(|_| "local_process")?;
        if format!(
            "{:x}",
            Sha256::digest(fs::read(name).map_err(|_| "local_process")?)
        ) != hash
        {
            return Err("local_process_hash");
        };
        Ok(creation)
    })();
    unsafe { CloseHandle(handle) };
    result
}
#[cfg(not(windows))]
fn native_process(_: u32, _: &str, _: u16) -> Result<u64> {
    Err("local_windows_required")
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn local_endpoint_closed_grammar() {
        assert_eq!(endpoint_port("http://127.0.0.1:8767"), Ok(8767));
        for bad in [
            "http://localhost:8767",
            "https://127.0.0.1:8767",
            "http://127.0.0.2:8767",
            "http://127.0.0.1:0",
            "http://127.0.0.1:08767",
            "http://127.0.0.1:8767/",
            "http://127.0.0.1:8767?key=fixture",
            "http://user@127.0.0.1:8767",
            "http://127.0.0.1:8767#x",
            "http://[::1]:8767",
            "http://127.0.0.1:65536",
        ] {
            assert!(endpoint_port(bad).is_err(), "{bad}")
        }
    }
    #[test]
    fn arguments_require_explicit_binding_no_downgrade() {
        let valid = vec![
            "--launch-receipt".into(),
            "C:/fixture/launch.json".into(),
            "--state".into(),
            "C:/fixture".into(),
            "--endpoint".into(),
            "http://127.0.0.1:8767".into(),
            "--control-sha256".into(),
            "a".repeat(64),
            "--verify-only".into(),
        ];
        assert!(parse_args(&valid).unwrap().verify_only);
        assert!(parse_args(&[]).is_err());
        assert!(parse_args(&valid[..8]).is_ok());
        let mut duplicate = valid.clone();
        duplicate.push("--verify-only".into());
        assert!(parse_args(&duplicate).is_err());
        let mut wrong = valid.clone();
        wrong[7] = "A".repeat(64);
        assert!(parse_args(&wrong).is_err());
        let mut relative = valid.clone();
        relative[1] = "launch.json".into();
        assert!(parse_args(&relative).is_err());
        let mut unknown = valid;
        unknown.push("--unsafe-remote".into());
        assert!(parse_args(&unknown).is_err());
    }
}
