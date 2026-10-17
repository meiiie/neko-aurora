//! Internal Windows preparation transport. Never selected by run()/CLI/scheduler.
//! Two seconds bounds acceptance, not OS teardown. Revocation precedes termination;
//! this call remains pending until the process tree and both I/O workers stop.
//! Synchronous OS setup/callbacks cannot be interrupted; elapsed setup time is
//! checked before resume/input. This is not a hard real-time revocation guarantee.
use crate::saved_account_bootstrap::ProtectedExecutables;
type Result<T> = std::result::Result<T, &'static str>;
const ERROR: &str = "saved_account_process_unavailable";
const INPUT_LIMIT: usize = 16384;
const OUTPUT_LIMIT: usize = 1024;

/// Invoke only from the existing preparation callback, retaining `images` through
/// the entire operation. `current` must check the live task authority; `revoke`
/// must synchronously revoke it on any failure. Callbacks must be nonblocking and
/// nonpanicking and must never log private input or include it in panic payloads.
/// catch_unwind preserves cleanup; it does not suppress the process panic hook.
/// No provider material is placed in argv/environment or forwarded to diagnostics.
pub fn prepare(
    images: &ProtectedExecutables,
    input: &mut [u8],
    current: &dyn Fn() -> bool,
    revoke: &dyn Fn(),
) -> Result<Vec<u8>> {
    let revoked = std::cell::Cell::new(false);
    let revoke_once = || {
        if !revoked.replace(true) {
            revoke_safely(revoke);
        }
    };
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        if input.is_empty() || input.len() > INPUT_LIMIT {
            Err(ERROR)
        } else {
            #[cfg(windows)]
            {
                windows::run(
                    images.core_path(),
                    &["saved-account", "prepare", "--json", "--broker-stdin"],
                    input,
                    current,
                    &revoke_once,
                )
            }
            #[cfg(not(windows))]
            {
                let _ = (images, current);
                Err(ERROR)
            }
        }
    }))
    .unwrap_or(Err(ERROR));
    input.fill(0);
    if result.is_err() {
        revoke_once();
    }
    result
}

fn revoke_safely(revoke: &dyn Fn()) {
    // Teardown must proceed even if an embedding authority callback panics.
    let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(revoke));
}
#[cfg(windows)]
struct RevokeOnFailure<'a> {
    revoke: &'a dyn Fn(),
    accepted: bool,
}
#[cfg(windows)]
impl Drop for RevokeOnFailure<'_> {
    fn drop(&mut self) {
        if !self.accepted {
            revoke_safely(self.revoke);
        }
    }
}
struct PrivateBytes(Vec<u8>);
impl Drop for PrivateBytes {
    fn drop(&mut self) {
        self.0.fill(0);
    }
}
fn receipt(bytes: &[u8]) -> Result<Vec<u8>> {
    if bytes.len() > OUTPUT_LIMIT {
        return Err(ERROR);
    }
    let canonical = PrivateBytes(crate::canonical(bytes).map_err(|_| ERROR)?);
    let r: crate::saved_key::BrokerReceipt =
        serde_json::from_slice(&canonical.0).map_err(|_| ERROR)?;
    let hex = |s: &str, n| {
        s.len() == n
            && s.bytes()
                .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
    };
    if r.version != 1
        || !r
            .capsule_id
            .strip_prefix("sa1_")
            .is_some_and(|s| hex(s, 32))
        || !hex(&r.binding_sha256, 64)
    {
        return Err(ERROR);
    }
    // Only this closed, typed, bounded receipt can leave the transport. Binding
    // equality/expiry/fixture policy remain checked by saved_key::prepare_internal.
    Ok(canonical.0.clone())
}

#[cfg(windows)]
mod windows {
    use super::*;
    use std::{
        ffi::{c_void, OsStr},
        fs::File,
        io::{Read, Write},
        mem::{size_of, zeroed},
        os::windows::{
            ffi::OsStrExt,
            io::{AsRawHandle, FromRawHandle, IntoRawHandle, OwnedHandle},
        },
        path::Path,
        ptr::{null, null_mut},
        thread,
        time::{Duration, Instant},
    };
    type Handle = *mut c_void;
    #[repr(C)]
    struct Security {
        size: u32,
        descriptor: Handle,
        inherit: i32,
    }
    #[repr(C)]
    struct Startup {
        size: u32,
        reserved: *mut u16,
        desktop: *mut u16,
        title: *mut u16,
        x: u32,
        y: u32,
        xsize: u32,
        ysize: u32,
        xchars: u32,
        ychars: u32,
        fill: u32,
        flags: u32,
        show: u16,
        reserved_size: u16,
        reserved_data: *mut u8,
        stdin: Handle,
        stdout: Handle,
        stderr: Handle,
    }
    #[repr(C)]
    struct StartupEx {
        startup: Startup,
        attributes: Handle,
    }
    #[repr(C)]
    struct ProcessInfo {
        process: Handle,
        thread: Handle,
        pid: u32,
        tid: u32,
    }
    #[repr(C)]
    #[derive(Default)]
    struct BasicLimit {
        process_time: i64,
        job_time: i64,
        flags: u32,
        min_working: usize,
        max_working: usize,
        active: u32,
        affinity: usize,
        priority: u32,
        scheduling: u32,
    }
    #[repr(C)]
    #[derive(Default)]
    struct ExtendedLimit {
        basic: BasicLimit,
        io: [u64; 6],
        process_memory: usize,
        job_memory: usize,
        peak_process: usize,
        peak_job: usize,
    }
    #[repr(C)]
    #[derive(Default)]
    struct Accounting {
        user: i64,
        kernel: i64,
        period_user: i64,
        period_kernel: i64,
        page_faults: u32,
        total: u32,
        active: u32,
        terminated: u32,
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn GetLastError() -> u32;
        fn CreatePipe(
            read: *mut Handle,
            write: *mut Handle,
            security: *const Security,
            size: u32,
        ) -> i32;
        fn SetHandleInformation(handle: Handle, mask: u32, flags: u32) -> i32;
        fn CreateFileW(
            name: *const u16,
            access: u32,
            share: u32,
            security: *const Security,
            creation: u32,
            flags: u32,
            template: Handle,
        ) -> Handle;
        fn CreateJobObjectW(security: *const Security, name: *const u16) -> Handle;
        fn SetInformationJobObject(job: Handle, class: u32, info: *const c_void, size: u32) -> i32;
        fn QueryInformationJobObject(
            job: Handle,
            class: u32,
            info: *mut c_void,
            size: u32,
            returned: *mut u32,
        ) -> i32;
        fn AssignProcessToJobObject(job: Handle, process: Handle) -> i32;
        fn TerminateJobObject(job: Handle, code: u32) -> i32;
        fn TerminateProcess(process: Handle, code: u32) -> i32;
        fn WaitForSingleObject(handle: Handle, ms: u32) -> u32;
        fn GetExitCodeProcess(process: Handle, code: *mut u32) -> i32;
        fn InitializeProcThreadAttributeList(
            list: Handle,
            count: u32,
            flags: u32,
            size: *mut usize,
        ) -> i32;
        fn UpdateProcThreadAttribute(
            list: Handle,
            flags: u32,
            attribute: usize,
            value: Handle,
            size: usize,
            previous: Handle,
            returned: *mut usize,
        ) -> i32;
        fn DeleteProcThreadAttributeList(list: Handle);
        fn CreateProcessW(
            application: *const u16,
            command: *mut u16,
            process_security: *const Security,
            thread_security: *const Security,
            inherit: i32,
            flags: u32,
            environment: Handle,
            cwd: *const u16,
            startup: *mut Startup,
            info: *mut ProcessInfo,
        ) -> i32;
        fn ResumeThread(thread: Handle) -> u32;
        fn GetWindowsDirectoryW(buffer: *mut u16, size: u32) -> u32;
    }
    fn own(h: Handle) -> Result<OwnedHandle> {
        if h.is_null() || h as isize == -1 {
            Err(ERROR)
        } else {
            Ok(unsafe { OwnedHandle::from_raw_handle(h) })
        }
    }
    fn wide(s: &OsStr) -> Result<Vec<u16>> {
        let mut v: Vec<_> = s.encode_wide().collect();
        if v.contains(&0) {
            return Err(ERROR);
        }
        v.push(0);
        Ok(v)
    }
    fn command(path: &Path, args: &[&str]) -> Result<Vec<u16>> {
        let mut out = Vec::new();
        for arg in std::iter::once(path.as_os_str()).chain(args.iter().map(OsStr::new)) {
            if !out.is_empty() {
                out.push(32);
            }
            let encoded: Vec<u16> = arg.encode_wide().collect();
            if !encoded.is_empty() && !encoded.iter().any(|c| matches!(c, 0 | 9 | 32 | 34)) {
                out.extend(encoded);
                continue;
            }
            out.push(34);
            let mut slashes = 0;
            for c in arg.encode_wide() {
                if c == 0 {
                    return Err(ERROR);
                }
                if c == 92 {
                    slashes += 1;
                    continue;
                }
                out.extend(std::iter::repeat_n(
                    92,
                    if c == 34 { slashes * 2 + 1 } else { slashes },
                ));
                slashes = 0;
                out.push(c);
            }
            out.extend(std::iter::repeat_n(92, slashes * 2));
            out.push(34);
        }
        if out.len() > 32766 {
            return Err(ERROR);
        }
        out.push(0);
        Ok(out)
    }
    fn pipe(parent_reads: bool) -> Result<(OwnedHandle, OwnedHandle)> {
        let sa = Security {
            size: size_of::<Security>() as u32,
            descriptor: null_mut(),
            inherit: 0,
        };
        let (mut r, mut w) = (null_mut(), null_mut());
        if unsafe { CreatePipe(&mut r, &mut w, &sa, 4096) } == 0 {
            return Err(ERROR);
        }
        let (r, w) = (own(r)?, own(w)?);
        let (parent, child) = if parent_reads { (r, w) } else { (w, r) };
        if unsafe { SetHandleInformation(child.as_raw_handle(), 1, 1) } == 0 {
            return Err(ERROR);
        }
        Ok((parent, child))
    }
    struct Attributes {
        storage: Vec<usize>,
        initialized: bool,
    }
    impl Attributes {
        fn new(handles: &mut [Handle]) -> Result<Self> {
            let mut bytes = 0;
            let sizing = unsafe { InitializeProcThreadAttributeList(null_mut(), 1, 0, &mut bytes) };
            if sizing != 0 || unsafe { GetLastError() } != 122 || bytes == 0 || bytes > 65536 {
                return Err(ERROR);
            }
            let mut a = Self {
                storage: vec![0; bytes.div_ceil(size_of::<usize>())],
                initialized: false,
            };
            if unsafe { InitializeProcThreadAttributeList(a.ptr(), 1, 0, &mut bytes) } == 0 {
                return Err(ERROR);
            }
            a.initialized = true;
            if unsafe {
                UpdateProcThreadAttribute(
                    a.ptr(),
                    0,
                    0x00020002,
                    handles.as_mut_ptr().cast(),
                    size_of_val(handles),
                    null_mut(),
                    null_mut(),
                )
            } == 0
            {
                return Err(ERROR);
            }
            Ok(a)
        }
        fn ptr(&mut self) -> Handle {
            self.storage.as_mut_ptr().cast()
        }
    }
    impl Drop for Attributes {
        fn drop(&mut self) {
            if self.initialized {
                unsafe {
                    DeleteProcThreadAttributeList(self.ptr());
                }
            }
        }
    }
    struct Child {
        process: OwnedHandle,
        primary: OwnedHandle,
        job: OwnedHandle,
        assigned: bool,
    }
    impl Child {
        fn active(&self) -> Result<u32> {
            let mut a = Accounting::default();
            if unsafe {
                QueryInformationJobObject(
                    self.job.as_raw_handle(),
                    1,
                    (&mut a as *mut Accounting).cast(),
                    size_of::<Accounting>() as u32,
                    null_mut(),
                )
            } == 0
            {
                return Err(ERROR);
            }
            Ok(a.active)
        }
        fn exited(&self) -> bool {
            (unsafe { WaitForSingleObject(self.process.as_raw_handle(), 0) }) == 0
        }
        fn success(&self) -> bool {
            let mut code = 1;
            self.exited()
                && unsafe { GetExitCodeProcess(self.process.as_raw_handle(), &mut code) } != 0
                && code == 0
        }
        fn stop(&self) {
            // Retain ownership on OS errors; never publish a false cleanup receipt.
            // The job also kills on last close if this host is terminated abruptly.
            loop {
                unsafe {
                    if self.assigned {
                        TerminateJobObject(self.job.as_raw_handle(), 1);
                    } else {
                        TerminateProcess(self.process.as_raw_handle(), 1);
                    }
                }
                if self.exited() && (!self.assigned || self.active() == Ok(0)) {
                    break;
                }
                thread::sleep(Duration::from_millis(5));
            }
        }
    }
    impl Drop for Child {
        fn drop(&mut self) {
            self.stop();
        }
    }
    fn spawn(path: &Path, args: &[&str], revoke: &dyn Fn()) -> Result<(Child, File, File)> {
        let app = wide(path.as_os_str())?;
        let mut cmd = command(path, args)?;
        let (parent_in, child_in) = pipe(false)?;
        let (parent_out, child_out) = pipe(true)?;
        let sa = Security {
            size: size_of::<Security>() as u32,
            descriptor: null_mut(),
            inherit: 1,
        };
        let nul = wide(OsStr::new("NUL"))?;
        let stderr =
            own(unsafe { CreateFileW(nul.as_ptr(), 0x40000000, 3, &sa, 3, 0, null_mut()) })?;
        let job = own(unsafe { CreateJobObjectW(null(), null()) })?;
        let mut limits = ExtendedLimit::default();
        limits.basic.flags = 0x2000; // KILL_ON_JOB_CLOSE, no breakaway.
        if unsafe {
            SetInformationJobObject(
                job.as_raw_handle(),
                9,
                (&limits as *const ExtendedLimit).cast(),
                size_of::<ExtendedLimit>() as u32,
            )
        } == 0
        {
            return Err(ERROR);
        }
        let mut handles = [
            child_in.as_raw_handle(),
            child_out.as_raw_handle(),
            stderr.as_raw_handle(),
        ];
        let mut attributes = Attributes::new(&mut handles)?;
        let mut startup: StartupEx = unsafe { zeroed() };
        startup.startup.size = size_of::<StartupEx>() as u32;
        startup.startup.flags = 0x100;
        startup.startup.stdin = handles[0];
        startup.startup.stdout = handles[1];
        startup.startup.stderr = handles[2];
        startup.attributes = attributes.ptr();
        let mut environment: Vec<u16> = "SystemRoot=".encode_utf16().collect();
        let mut dir = [0u16; 32768];
        let count = unsafe { GetWindowsDirectoryW(dir.as_mut_ptr(), dir.len() as u32) } as usize;
        if count == 0 || count >= dir.len() {
            return Err(ERROR);
        }
        environment.extend_from_slice(&dir[..count]);
        environment.extend([0, 0]);
        let cwd = wide(path.parent().ok_or(ERROR)?.as_os_str())?;
        let mut info: ProcessInfo = unsafe { zeroed() };
        if unsafe {
            CreateProcessW(
                app.as_ptr(),
                cmd.as_mut_ptr(),
                null(),
                null(),
                1,
                0x4 | 0x80000 | 0x8000000 | 0x400,
                environment.as_mut_ptr().cast(),
                cwd.as_ptr(),
                &mut startup.startup,
                &mut info,
            )
        } == 0
        {
            return Err(ERROR);
        }
        let mut child = Child {
            process: own(info.process)?,
            primary: own(info.thread)?,
            job,
            assigned: false,
        };
        let mut failure = RevokeOnFailure {
            revoke,
            accepted: false,
        };
        // These duplicates must close before any worker: otherwise EOF/pipe
        // shutdown can never prove that all child owners have exited.
        drop(child_in);
        drop(child_out);
        drop(stderr);
        if unsafe {
            AssignProcessToJobObject(child.job.as_raw_handle(), child.process.as_raw_handle())
        } == 0
        {
            return Err(ERROR);
        }
        child.assigned = true;
        failure.accepted = true;
        Ok((
            child,
            unsafe { File::from_raw_handle(parent_in.into_raw_handle()) },
            unsafe { File::from_raw_handle(parent_out.into_raw_handle()) },
        ))
    }
    pub(super) fn run(
        path: &Path,
        args: &[&str],
        input: &mut [u8],
        current: &dyn Fn() -> bool,
        revoke: &dyn Fn(),
    ) -> Result<Vec<u8>> {
        let started = Instant::now();
        if !current() {
            return Err(ERROR);
        }
        let (child, mut stdin, mut stdout) = spawn(path, args, revoke)?;
        // Move child INTO the scoped closure: panic/unwind kills the job before
        // scope's implicit joins. Keeping it outside would deadlock on I/O.
        thread::scope(move |scope| {
            let child = child;
            let mut failure = RevokeOnFailure {
                revoke,
                accepted: false,
            };
            if started.elapsed() >= Duration::from_secs(2) || !current() {
                revoke_safely(revoke);
                return Err(ERROR);
            }
            if unsafe { ResumeThread(child.primary.as_raw_handle()) } != 1 {
                revoke_safely(revoke);
                return Err(ERROR);
            }
            let writer = thread::Builder::new()
                .spawn_scoped(scope, move || {
                    let result = stdin.write_all(input).map_err(|_| ERROR);
                    drop(stdin);
                    result
                })
                .map_err(|_| ERROR)?;
            let reader = thread::Builder::new()
                .spawn_scoped(scope, move || {
                    let mut bytes = PrivateBytes(vec![0; OUTPUT_LIMIT + 1]);
                    let mut used = 0;
                    loop {
                        match stdout.read(&mut bytes.0[used..]) {
                            Ok(0) => {
                                bytes.0.truncate(used);
                                return Ok(bytes);
                            }
                            Ok(n) => {
                                used += n;
                                if used > OUTPUT_LIMIT {
                                    return Err(ERROR);
                                }
                            }
                            Err(_) => return Err(ERROR),
                        }
                    }
                })
                .map_err(|_| ERROR)?;
            let mut accepted = false;
            loop {
                if started.elapsed() >= Duration::from_secs(2) || !current() {
                    break;
                }
                if writer.is_finished() && reader.is_finished() && child.exited() {
                    accepted = child.success() && child.active() == Ok(0);
                    break;
                }
                thread::sleep(Duration::from_millis(5));
            }
            if !accepted {
                revoke_safely(revoke);
            }
            child.stop(); // leader + every descendant, before either join
            let written = writer.join().map_err(|_| ERROR)?;
            let read = reader.join().map_err(|_| ERROR)?;
            written?;
            let bytes = read?;
            if !accepted || started.elapsed() >= Duration::from_secs(2) || !current() {
                revoke_safely(revoke);
                return Err(ERROR);
            }
            let result = receipt(&bytes.0);
            if started.elapsed() >= Duration::from_secs(2) || !current() {
                if let Ok(mut bytes) = result {
                    bytes.fill(0);
                }
                return Err(ERROR);
            }
            failure.accepted = result.is_ok();
            result
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn valid() -> Vec<u8> {
        format!(r#"{{"version":1,"capsuleId":"sa1_{}","bindingSHA256":"{}","expiresAtUnixMS":1,"fixtureOnly":true}}"#,"a".repeat(32),"b".repeat(64)).into_bytes()
    }
    #[test]
    fn closed_receipt_only() {
        assert!(receipt(&valid()).is_ok());
        for v in [
            b"null".as_slice(),
            b"{}",
            b"{\"secret\":\"synthetic\"}",
            b"[]",
        ] {
            assert!(receipt(v).is_err());
        }
        let mut v: serde_json::Value = serde_json::from_slice(&valid()).unwrap();
        v["secret"] = serde_json::json!("synthetic");
        assert!(receipt(&serde_json::to_vec(&v).unwrap()).is_err());
    }
    #[test]
    fn receipt_bounds_and_identity() {
        assert!(receipt(&vec![b' '; OUTPUT_LIMIT + 1]).is_err());
        let mut v: serde_json::Value = serde_json::from_slice(&valid()).unwrap();
        v["capsuleId"] = serde_json::json!("synthetic-private-output");
        assert!(receipt(&serde_json::to_vec(&v).unwrap()).is_err());
    }
}

#[cfg(all(test, windows))]
mod native_tests {
    use super::*;
    use std::{
        cell::Cell,
        fs,
        path::PathBuf,
        sync::atomic::{AtomicU64, Ordering},
        time::Instant,
    };
    static NEXT: AtomicU64 = AtomicU64::new(0);
    struct Fixture {
        directory: PathBuf,
        cmd: PathBuf,
        guard: Option<crate::executable_guard::ExecutableGuard>,
    }
    impl Fixture {
        fn new(script: &str) -> Self {
            let cmd =
                PathBuf::from(std::env::var_os("SystemRoot").unwrap()).join("System32/cmd.exe");
            let directory = std::env::temp_dir().join(format!(
                "neko broker transport {} {}",
                std::process::id(),
                NEXT.fetch_add(1, Ordering::SeqCst)
            ));
            fs::create_dir(&directory).unwrap();
            let source = cmd;
            let cmd = directory.join("cmd.exe");
            fs::copy(source, &cmd).unwrap();
            let guard = crate::executable_guard::ExecutableGuard::open(&cmd).unwrap();
            fs::write(
                directory.join("fixture.cmd"),
                format!("@echo off\r\n{script}\r\n"),
            )
            .unwrap();
            Self {
                directory,
                cmd,
                guard: Some(guard),
            }
        }
        fn run(&self, current: &dyn Fn() -> bool, revoke: &dyn Fn()) -> Result<Vec<u8>> {
            let script = self.directory.join("fixture.cmd");
            let mut synthetic = b"synthetic-private-fixture".to_vec();
            let result = windows::run(
                &self.cmd,
                &["/d", "/c", script.to_str().unwrap()],
                &mut synthetic,
                current,
                revoke,
            );
            synthetic.fill(0);
            result
        }
    }
    impl Drop for Fixture {
        fn drop(&mut self) {
            self.guard.take();
            fs::remove_dir_all(&self.directory).unwrap();
        }
    }
    fn valid() -> String {
        format!(
            r#"{{"version":1,"capsuleId":"sa1_{}","bindingSHA256":"{}","expiresAtUnixMS":1,"fixtureOnly":true}}"#,
            "a".repeat(32),
            "b".repeat(64)
        )
    }
    #[test]
    fn native_receipt_and_discarded_stderr() {
        let f = Fixture::new(&format!(
            "set /p ignored=\r\necho synthetic-diagnostic-must-not-forward 1>&2\r\necho {}",
            valid()
        ));
        assert!(f
            .run(&|| true, &|| panic!("successful preparation revoked"))
            .is_ok());
    }
    #[test]
    fn native_oversize_and_nonzero_exit_are_rejected() {
        for script in [
            "for /L %%i in (1,1,200) do @echo synthetic-fixture-output".to_owned(),
            format!("echo {}\r\nexit /b 7", valid()),
        ] {
            let f = Fixture::new(&script);
            let revoked = Cell::new(false);
            assert!(f.run(&|| true, &|| revoked.set(true)).is_err());
            assert!(revoked.get());
        }
    }
    #[test]
    fn native_deadline_terminates_descendant_pipe_owners() {
        let f = Fixture::new("rem replaced below");
        fs::write(
            f.directory.join("descendant.cmd"),
            "@echo off\r\n:again\r\ngoto again\r\n",
        )
        .unwrap();
        fs::write(
            f.directory.join("fixture.cmd"),
            format!(
                "@echo off\r\nstart \"\" /b \"{}\" /d /c \"{}\"\r\n:again\r\ngoto again\r\n",
                f.cmd.display(),
                f.directory.join("descendant.cmd").display()
            ),
        )
        .unwrap();
        let revoked = Cell::new(false);
        let started = Instant::now();
        assert!(f.run(&|| true, &|| revoked.set(true)).is_err());
        assert!(revoked.get());
        // No upper cleanup assertion: OS termination has no hard latency bound.
        assert!(started.elapsed() >= std::time::Duration::from_secs(2));
    }
    #[test]
    fn native_cancellation_revokes_and_reaps_tree() {
        let f = Fixture::new(":again\r\ngoto again");
        let calls = Cell::new(0);
        let revoked = Cell::new(false);
        assert!(f
            .run(
                &|| {
                    calls.set(calls.get() + 1);
                    calls.get() < 4
                },
                &|| revoked.set(true)
            )
            .is_err());
        assert!(revoked.get());
    }
    #[test]
    fn native_blocked_input_worker_is_joined_after_cancellation() {
        let f = Fixture::new(":again\r\ngoto again");
        let script = f.directory.join("fixture.cmd");
        let mut input = vec![b'x'; INPUT_LIMIT];
        let checks = Cell::new(0);
        let revoked = Cell::new(false);
        assert!(windows::run(
            &f.cmd,
            &["/d", "/c", script.to_str().unwrap()],
            &mut input,
            &|| {
                checks.set(checks.get() + 1);
                checks.get() < 8
            },
            &|| revoked.set(true)
        )
        .is_err());
        input.fill(0);
        assert!(revoked.get());
    }
}
