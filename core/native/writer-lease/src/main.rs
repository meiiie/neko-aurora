#[cfg(not(windows))]
fn main() {
    eprintln!("writer-lease: Windows kernel ownership is required");
    std::process::exit(1);
}

#[cfg(windows)]
mod windows {
    use serde::{Deserialize, Serialize};
    use sha2::{Digest, Sha256};
    use std::ffi::c_void;
    use std::fs::{File, OpenOptions};
    use std::io::{self, BufRead, Read, Seek, SeekFrom, Write};
    use std::os::windows::fs::OpenOptionsExt;
    use std::os::windows::io::AsRawHandle;
    use std::path::PathBuf;
    use std::sync::mpsc;
    use std::thread;

    type Handle = *mut c_void;
    const CONTRACT: &str = "neko.windows.writer-lease.v2";
    const LIMIT: u64 = 8192;
    const QUERY: u32 = 0x1000;
    const SYNCHRONIZE: u32 = 0x100000;
    const SIGNALED: u32 = 0;
    const TIMEOUT: u32 = 258;
    const REPARSE: u32 = 0x400;

    #[repr(C)]
    #[derive(Default)]
    struct FileTime {
        low: u32,
        high: u32,
    }
    #[repr(C)]
    #[derive(Default)]
    struct FileInfo {
        attributes: u32,
        created: FileTime,
        accessed: FileTime,
        written: FileTime,
        volume: u32,
        size_high: u32,
        size_low: u32,
        links: u32,
        index_high: u32,
        index_low: u32,
    }
    #[repr(C)]
    struct ProcessEntry {
        size: u32,
        usage: u32,
        pid: u32,
        heap: usize,
        module: u32,
        threads: u32,
        parent: u32,
        priority: i32,
        flags: u32,
        exe: [u16; 260],
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn OpenProcess(access: u32, inherit: i32, pid: u32) -> Handle;
        fn CloseHandle(handle: Handle) -> i32;
        fn WaitForSingleObject(handle: Handle, milliseconds: u32) -> u32;
        fn GetProcessTimes(
            handle: Handle,
            created: *mut FileTime,
            exit: *mut FileTime,
            kernel: *mut FileTime,
            user: *mut FileTime,
        ) -> i32;
        fn QueryFullProcessImageNameW(
            handle: Handle,
            flags: u32,
            path: *mut u16,
            length: *mut u32,
        ) -> i32;
        fn GetFileInformationByHandle(handle: Handle, info: *mut FileInfo) -> i32;
        fn CreateToolhelp32Snapshot(flags: u32, pid: u32) -> Handle;
        fn Process32FirstW(snapshot: Handle, entry: *mut ProcessEntry) -> i32;
        fn Process32NextW(snapshot: Handle, entry: *mut ProcessEntry) -> i32;
    }

    struct OwnedHandle(Handle);
    impl Drop for OwnedHandle {
        fn drop(&mut self) {
            unsafe {
                CloseHandle(self.0);
            }
        }
    }
    fn open_process(pid: u32) -> io::Result<OwnedHandle> {
        let handle = unsafe { OpenProcess(QUERY | SYNCHRONIZE, 0, pid) };
        if handle.is_null() {
            return Err(io::Error::last_os_error());
        }
        Ok(OwnedHandle(handle))
    }
    fn creation(handle: &OwnedHandle) -> io::Result<String> {
        let mut created = FileTime::default();
        let mut exit = FileTime::default();
        let mut kernel = FileTime::default();
        let mut user = FileTime::default();
        if unsafe { GetProcessTimes(handle.0, &mut created, &mut exit, &mut kernel, &mut user) }
            == 0
        {
            return Err(io::Error::last_os_error());
        }
        Ok((((created.high as u64) << 32) | created.low as u64).to_string())
    }
    fn actual_parent() -> io::Result<u32> {
        let snapshot = unsafe { CreateToolhelp32Snapshot(2, 0) };
        if snapshot == -1isize as Handle {
            return Err(io::Error::last_os_error());
        }
        let snapshot = OwnedHandle(snapshot);
        let mut entry = ProcessEntry {
            size: std::mem::size_of::<ProcessEntry>() as u32,
            usage: 0,
            pid: 0,
            heap: 0,
            module: 0,
            threads: 0,
            parent: 0,
            priority: 0,
            flags: 0,
            exe: [0; 260],
        };
        let mut available = unsafe { Process32FirstW(snapshot.0, &mut entry) } != 0;
        while available {
            if entry.pid == std::process::id() {
                return Ok(entry.parent);
            }
            available = unsafe { Process32NextW(snapshot.0, &mut entry) } != 0;
        }
        Err(io::Error::other("parent identity unavailable"))
    }
    fn parent_image_hash(handle: &OwnedHandle) -> io::Result<String> {
        let mut path = vec![0u16; 32768];
        let mut length = path.len() as u32;
        if unsafe { QueryFullProcessImageNameW(handle.0, 0, path.as_mut_ptr(), &mut length) } == 0 {
            return Err(io::Error::last_os_error());
        }
        path.truncate(length as usize);
        let path =
            String::from_utf16(&path).map_err(|_| io::Error::other("invalid parent path"))?;
        let mut file = File::open(path)?;
        let mut digest = Sha256::new();
        let mut block = [0; 65536];
        loop {
            let n = file.read(&mut block)?;
            if n == 0 {
                break;
            }
            digest.update(&block[..n]);
        }
        Ok(format!("{:x}", digest.finalize()))
    }
    fn owner_dead(pid: u32, created: &str) -> io::Result<bool> {
        match open_process(pid) {
            // Windows returns ERROR_INVALID_PARAMETER for a PID that no longer exists.
            Err(error) if error.raw_os_error() == Some(87) => Ok(true),
            Err(error) => Err(error),
            Ok(handle) => {
                if creation(&handle)? != created {
                    return Ok(true);
                }
                match unsafe { WaitForSingleObject(handle.0, 0) } {
                    SIGNALED => Ok(true),
                    TIMEOUT => Ok(false),
                    _ => Err(io::Error::other("owner liveness unavailable")),
                }
            }
        }
    }
    fn hex(value: &str, length: usize) -> bool {
        value.len() == length
            && value
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    }
    #[derive(Deserialize)]
    #[serde(deny_unknown_fields)]
    struct Command {
        version: u8,
        action: String,
        token: String,
    }
    #[derive(Serialize, Deserialize)]
    #[serde(deny_unknown_fields, rename_all = "camelCase")]
    struct Record {
        version: u8,
        contract: String,
        directory: String,
        session_id: String,
        parent_pid: u32,
        parent_created: String,
        parent_sha256: String,
        token: String,
        state: String,
    }
    impl Record {
        fn valid(&self, directory: &str, session: &str) -> bool {
            self.version == 2
                && self.contract == CONTRACT
                && self.directory == directory
                && self.session_id == session
                && self.parent_pid > 0
                && hex(&self.token, 32)
                && hex(&self.parent_sha256, 64)
                && self
                    .parent_created
                    .parse::<u64>()
                    .is_ok_and(|n| n > 0 && n.to_string() == self.parent_created)
                && matches!(self.state.as_str(), "held" | "released")
        }
    }
    fn command(line: &[u8], action: &str, token: Option<&str>) -> io::Result<Command> {
        let value: Command =
            serde_json::from_slice(line).map_err(|_| io::Error::other("invalid command"))?;
        if value.version != 2
            || value.action != action
            || !hex(&value.token, 32)
            || token.is_some_and(|token| token != value.token)
        {
            return Err(io::Error::other("invalid command"));
        }
        Ok(value)
    }
    fn read_line(input: &mut impl BufRead) -> io::Result<Vec<u8>> {
        let mut line = Vec::new();
        input.take(4097).read_until(b'\n', &mut line)?;
        if line.is_empty() || line.len() > 4096 || !line.ends_with(b"\n") {
            return Err(io::Error::other("bounded command unavailable"));
        }
        Ok(line)
    }
    fn write_record(file: &mut File, record: &Record) -> io::Result<()> {
        let bytes = serde_json::to_vec(record).map_err(io::Error::other)?;
        file.seek(SeekFrom::Start(0))?;
        file.write_all(&bytes)?;
        file.set_len(bytes.len() as u64)?;
        file.sync_all()
    }
    fn reply(record: &Record) -> io::Result<()> {
        let mut stdout = io::stdout().lock();
        // Owner token never enters stderr, arguments, transcripts or model context.
        serde_json::to_writer(&mut stdout, record).map_err(io::Error::other)?;
        stdout.write_all(b"\n")?;
        stdout.flush()
    }
    pub fn run() -> io::Result<()> {
        let args: Vec<String> = std::env::args().skip(1).collect();
        if args.len() != 8
            || args[0] != "--directory"
            || args[2] != "--session"
            || args[4] != "--parent"
            || args[6] != "--parent-sha256"
            || !hex(&args[3], 32)
            || !hex(&args[7], 64)
        {
            return Err(io::Error::other("invalid arguments"));
        }
        let input_path = PathBuf::from(&args[1]);
        if !input_path.is_absolute() {
            return Err(io::Error::other("directory must be absolute"));
        }
        let directory = std::fs::canonicalize(input_path)?;
        if !directory.is_dir() {
            return Err(io::Error::other("task store unavailable"));
        }
        let canonical = directory.to_string_lossy().to_lowercase();
        let pid: u32 = args[5]
            .parse()
            .map_err(|_| io::Error::other("invalid parent PID"))?;
        if pid == 0 || actual_parent()? != pid {
            return Err(io::Error::other("parent identity mismatch"));
        }
        let parent = open_process(pid)?;
        if unsafe { WaitForSingleObject(parent.0, 0) } != TIMEOUT
            || parent_image_hash(&parent)? != args[7]
        {
            return Err(io::Error::other("parent executable identity mismatch"));
        }
        let created = creation(&parent)?;
        let acquire = command(&read_line(&mut io::stdin().lock())?, "acquire", None)?;
        let path = directory.join(format!("{}.lock", args[3]));
        let options = || {
            let mut o = OpenOptions::new();
            o.read(true)
                .write(true)
                .share_mode(0)
                .custom_flags(0x00200000);
            o
        };
        let (mut file, fresh) = match options().create_new(true).open(&path) {
            Ok(file) => (file, true),
            Err(error) if error.kind() == io::ErrorKind::AlreadyExists => {
                (options().open(&path)?, false)
            }
            Err(error) => return Err(error),
        };
        let mut info = FileInfo::default();
        if unsafe { GetFileInformationByHandle(file.as_raw_handle(), &mut info) } == 0
            || info.attributes & REPARSE != 0
            || info.links != 1
        {
            return Err(io::Error::other(
                "lock must be a regular file with one link",
            ));
        }
        if !fresh {
            if file.metadata()?.len() == 0 || file.metadata()?.len() > LIMIT {
                return Err(io::Error::other(
                    "legacy or incomplete lock requires explicit recovery",
                ));
            }
            let mut bytes = Vec::new();
            (&mut file).take(LIMIT + 1).read_to_end(&mut bytes)?;
            let old: Record = serde_json::from_slice(&bytes).map_err(|_| {
                io::Error::other("legacy or corrupt lock requires explicit recovery")
            })?;
            if !old.valid(&canonical, &args[3])
                || (old.state == "held" && !owner_dead(old.parent_pid, &old.parent_created)?)
            {
                return Err(io::Error::other("writer active or ownership unavailable"));
            }
        }
        let mut record = Record {
            version: 2,
            contract: CONTRACT.into(),
            directory: canonical,
            session_id: args[3].clone(),
            parent_pid: pid,
            parent_created: created,
            parent_sha256: args[7].clone(),
            token: acquire.token,
            state: "held".into(),
        };
        write_record(&mut file, &record)?;
        reply(&record)?;
        let (sender, receiver) = mpsc::sync_channel(1);
        thread::spawn(move || {
            let result = read_line(&mut io::stdin().lock());
            let _ = sender.send(result);
        });
        loop {
            match unsafe { WaitForSingleObject(parent.0, 50) } {
                SIGNALED => return Ok(()),
                TIMEOUT => {}
                _ => return Err(io::Error::other("parent liveness unavailable")),
            }
            match receiver.try_recv() {
                Ok(Ok(line)) => {
                    command(&line, "release", Some(&record.token))?;
                    record.state = "released".into();
                    write_record(&mut file, &record)?;
                    drop(file);
                    reply(&record)?;
                    return Ok(());
                }
                Ok(Err(error)) => return Err(error),
                Err(mpsc::TryRecvError::Disconnected) => {
                    return Err(io::Error::other("owner pipe unavailable"))
                }
                Err(mpsc::TryRecvError::Empty) => {}
            }
        }
    }
}

#[cfg(windows)]
fn main() {
    if windows::run().is_err() {
        // Detailed file/process errors and owner records stay on the private side of the boundary.
        eprintln!("writer-lease: ownership unavailable; fail closed");
        std::process::exit(1);
    }
}
