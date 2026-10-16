use serde_json::{json, Value};
use std::{
    fs::{self, File, OpenOptions},
    io::{Read, Write},
    path::{Path, PathBuf},
    process::Child,
};
type Result<T> = std::result::Result<T, &'static str>;
pub const MAX_BYTES: usize = 16384;

fn identifier(s: &str, prefix: &str) -> bool {
    s.len() == prefix.len() + 32
        && s.starts_with(prefix)
        && s[prefix.len()..]
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn directory(path: &Path) -> Result<()> {
    let meta = fs::symlink_metadata(path).map_err(|_| "native_stop_directory")?;
    if !meta.is_dir() || meta.file_type().is_symlink() {
        return Err("native_stop_directory");
    }
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if meta.file_attributes() & 0x400 != 0 {
            return Err("native_stop_directory");
        }
    }
    Ok(())
}
#[derive(Clone)]
pub struct StopOutbox {
    state: PathBuf,
    request: String,
    attempt: String,
    pin: Value,
    lease: Value,
}
impl StopOutbox {
    pub fn prepare(state: &Path, request: &str, pin: &Value, lease: &Value) -> Result<Self> {
        let attempt = lease["attemptId"].as_str().ok_or("native_stop_scope")?;
        if !identifier(request, "req-")
            || !identifier(attempt, "attempt-")
            || lease["requestId"] != request
        {
            return Err("native_stop_scope");
        }
        directory(state)?;
        let state = fs::canonicalize(state).map_err(|_| "native_stop_directory")?;
        if state.to_string_lossy().len() > 2048 {
            return Err("native_stop_bound");
        }
        Ok(Self {
            state,
            request: request.into(),
            attempt: attempt.into(),
            pin: pin.clone(),
            lease: lease.clone(),
        })
    }
    pub fn encode(&self, binding: &Value, receipt: &Value) -> Result<Vec<u8>> {
        let generation = binding["nativeProcessGeneration"]
            .as_str()
            .ok_or("native_stop_binding")?;
        if !generation
            .parse::<u64>()
            .is_ok_and(|n| n > 0 && n.to_string() == generation)
        {
            return Err("native_stop_binding");
        }
        if binding["contextAdmission"].is_null()
            || binding["nativeProcessGeneration"].as_str().is_none()
            || binding["nativeProcessGeneration"] != receipt["nativeProcessGeneration"]
            || binding["nativeProcessId"] != receipt["nativeProcessId"]
            || binding["sessionId"] != receipt["sessionId"]
            || binding["executorProof"] != receipt["executorProof"]
        {
            return Err("native_stop_binding");
        }
        let raw = serde_json::to_vec(
            &json!({"version":1,"requestId":self.request,"attemptId":self.attempt,
            "installPin":self.pin,"attemptLease":self.lease,"binding":binding,"receipt":receipt}),
        )
        .map_err(|_| "native_stop_encode")?;
        if raw.len() > MAX_BYTES {
            return Err("native_stop_bound");
        }
        Ok(raw)
    }
    pub fn publish(&self, binding: &Value, receipt: &Value) -> Result<()> {
        let raw = self.encode(binding, receipt)?;
        let base = self.state.join("runtime-stops");
        for dir in [&base, &base.join(&self.request)] {
            match fs::create_dir(dir) {
                Ok(()) => {}
                Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => {}
                Err(_) => return Err("native_stop_directory"),
            }
            directory(dir)?;
            if fs::canonicalize(dir).map_err(|_| "native_stop_directory")? != *dir {
                return Err("native_stop_directory");
            }
        }
        let path = base
            .join(&self.request)
            .join(format!("{}.json", self.attempt));
        if path.exists() {
            return if read_existing(&path)? == raw {
                Ok(())
            } else {
                Err("native_stop_conflict")
            };
        }
        let pending = path.with_extension("pending");
        let mut options = OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(windows)]
        {
            use std::os::windows::fs::OpenOptionsExt;
            options.share_mode(0).custom_flags(0x00200000);
        }
        let mut file = options.open(&pending).map_err(|_| "native_stop_publish")?;
        file.write_all(&raw)
            .and_then(|_| file.sync_all())
            .map_err(|_| "native_stop_publish")?;
        drop(file);
        if publish_new(&pending, &path).is_err() {
            if path.exists() && read_existing(&path)? == raw {
                return Ok(());
            }
            return Err("native_stop_publish");
        }
        Ok(())
    }
}
fn read_existing(path: &Path) -> Result<Vec<u8>> {
    let mut options = OpenOptions::new();
    options.read(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options.share_mode(1).custom_flags(0x00200000);
    }
    let file = options.open(path).map_err(|_| "native_stop_existing")?;
    regular_file(&file)?;
    let mut bytes = vec![];
    file.take((MAX_BYTES + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(|_| "native_stop_existing")?;
    if bytes.len() > MAX_BYTES {
        return Err("native_stop_bound");
    }
    Ok(bytes)
}
#[cfg(windows)]
mod win {
    use super::*;
    use std::{ffi::c_void, os::windows::ffi::OsStrExt, os::windows::io::AsRawHandle};
    #[repr(C)]
    #[derive(Default)]
    pub struct Time {
        low: u32,
        high: u32,
    }
    #[repr(C)]
    #[derive(Default)]
    struct Info {
        attrs: u32,
        created: Time,
        access: Time,
        write: Time,
        volume: u32,
        size_hi: u32,
        size_lo: u32,
        links: u32,
        index_hi: u32,
        index_lo: u32,
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn GetProcessTimes(
            h: *mut c_void,
            c: *mut Time,
            e: *mut Time,
            k: *mut Time,
            u: *mut Time,
        ) -> i32;
        fn GetFileInformationByHandle(h: *mut c_void, i: *mut Info) -> i32;
        fn MoveFileExW(a: *const u16, b: *const u16, flags: u32) -> i32;
    }
    pub fn generation(child: &Child) -> Result<String> {
        let (mut c, mut e, mut k, mut u) = (
            Time::default(),
            Time::default(),
            Time::default(),
            Time::default(),
        );
        if unsafe { GetProcessTimes(child.as_raw_handle(), &mut c, &mut e, &mut k, &mut u) } == 0 {
            return Err("native_process_generation");
        }
        let n = ((c.high as u64) << 32) | c.low as u64;
        if n == 0 {
            return Err("native_process_generation");
        };
        Ok(n.to_string())
    }
    pub fn regular(file: &File) -> Result<()> {
        let mut info = Info::default();
        if unsafe { GetFileInformationByHandle(file.as_raw_handle(), &mut info) } == 0
            || info.links != 1
            || info.attrs & (0x400 | 0x10) != 0
        {
            return Err("native_stop_existing");
        };
        Ok(())
    }
    pub fn publish(from: &Path, to: &Path) -> Result<()> {
        let from: Vec<_> = from.as_os_str().encode_wide().chain(Some(0)).collect();
        let to: Vec<_> = to.as_os_str().encode_wide().chain(Some(0)).collect();
        if unsafe { MoveFileExW(from.as_ptr(), to.as_ptr(), 8) } == 0 {
            return Err("native_stop_publish");
        };
        Ok(())
    }
}
#[cfg(windows)]
pub fn process_generation(child: &Child) -> Result<String> {
    win::generation(child)
}
#[cfg(not(windows))]
pub fn process_generation(_child: &Child) -> Result<String> {
    Err("native_process_generation_unsupported")
}
fn regular_file(file: &File) -> Result<()> {
    let meta = file.metadata().map_err(|_| "native_stop_existing")?;
    if !meta.is_file() || meta.len() > MAX_BYTES as u64 {
        return Err("native_stop_existing");
    }
    #[cfg(windows)]
    {
        win::regular(file)?;
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::MetadataExt;
        if meta.nlink() != 1 {
            return Err("native_stop_existing");
        }
    }
    Ok(())
}
#[cfg(windows)]
fn publish_new(from: &Path, to: &Path) -> Result<()> {
    win::publish(from, to)
}
#[cfg(not(windows))]
fn publish_new(from: &Path, to: &Path) -> Result<()> {
    fs::hard_link(from, to)
        .and_then(|_| fs::remove_file(from))
        .map_err(|_| "native_stop_publish")
}

#[cfg(test)]
mod tests {
    use super::*;
    fn fixture() -> (StopOutbox, Value, Value) {
        let state = std::env::temp_dir().join(format!(
            "neko-stop-unit-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir(&state).unwrap();
        let request = format!("req-{}", "a".repeat(32));
        let lease = json!({"attemptId":format!("attempt-{}","b".repeat(32)),"requestId":request});
        let out = StopOutbox::prepare(&state, &request, &json!({}), &lease).unwrap();
        let binding = json!({"nativeProcessId":401,"nativeProcessGeneration":"134362178434013773","sessionId":"c".repeat(32),"executorProof":{},"contextAdmission":{}});
        let receipt = json!({"version":1,"nativeProcessId":401,"nativeProcessGeneration":"134362178434013773","sessionId":"c".repeat(32),"executorProof":{}});
        (out, binding, receipt)
    }
    #[test]
    fn immutable_duplicate_and_conflict() {
        let (out, b, r) = fixture();
        out.publish(&b, &r).unwrap();
        out.publish(&b, &r).unwrap();
        let mut other = b.clone();
        other["task"] = json!({"id":"different"});
        assert!(out.publish(&other, &r).is_err());
    }
    #[test]
    fn bound_generation_and_path_scope() {
        let (out, mut b, mut r) = fixture();
        r["nativeProcessGeneration"] = json!("different");
        assert!(out.encode(&b, &r).is_err());
        r["nativeProcessGeneration"] = b["nativeProcessGeneration"].clone();
        b["task"] = json!("x".repeat(MAX_BYTES));
        assert!(out.encode(&b, &r).is_err());
        assert!(StopOutbox::prepare(&out.state, "../escape", &json!({}), &out.lease).is_err());
    }
    #[test]
    fn partial_pending_and_linked_existing_are_not_proof() {
        let (out, b, r) = fixture();
        let dir = out.state.join("runtime-stops").join(&out.request);
        fs::create_dir_all(&dir).unwrap();
        let path = dir.join(format!("{}.json", out.attempt));
        let pending = path.with_extension("pending");
        fs::write(&pending, b"partial").unwrap();
        assert!(out.publish(&b, &r).is_err());
        assert!(!path.exists());
        assert_eq!(fs::read(&pending).unwrap(), b"partial");
        fs::remove_file(&pending).unwrap();
        let raw = out.encode(&b, &r).unwrap();
        fs::write(&path, &raw).unwrap();
        fs::hard_link(&path, out.state.join("alias.json")).unwrap();
        assert!(out.publish(&b, &r).is_err());
        assert_eq!(fs::read(&path).unwrap(), raw);
    }
}
