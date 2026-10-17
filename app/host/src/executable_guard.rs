//! Retained executable identity. Windows holds deny-write/delete handles through
//! process teardown; POSIX retains the previous read/hash behavior, not a lock.
use sha2::{Digest, Sha256};
use std::{fs::File, io::Read, path::Path};
type Result<T> = std::result::Result<T, &'static str>;
pub struct ExecutableGuard {
    digest: String,
    native: bool,
    #[cfg(windows)]
    _files: Vec<File>,
}
impl ExecutableGuard {
    pub fn open(path: &Path) -> Result<Self> {
        #[cfg(windows)]
        let (mut file, mut files) = windows::open(path)?;
        #[cfg(not(windows))]
        let mut file = File::open(path).map_err(|_| "executable_guard_open")?;
        let mut hash = Sha256::new();
        let mut buffer = [0u8; 65536];
        let mut prefix: Vec<u8> = Vec::with_capacity(2);
        loop {
            let n = file
                .read(&mut buffer)
                .map_err(|_| "executable_guard_read")?;
            if n == 0 {
                break;
            }
            prefix.extend(buffer[..n].iter().copied().take(2 - prefix.len()));
            hash.update(&buffer[..n]);
        }
        #[cfg(windows)]
        {
            windows::verify(&file, path, false)?;
            files.push(file);
        }
        Ok(Self {
            digest: format!("{:x}", hash.finalize()),
            native: prefix == b"MZ",
            #[cfg(windows)]
            _files: files,
        })
    }
    pub fn sha256(&self) -> &str {
        &self.digest
    }
    pub fn native(&self) -> bool {
        self.native
    }
}

#[cfg(windows)]
mod windows {
    use super::*;
    use std::{
        ffi::c_void,
        fs::OpenOptions,
        os::windows::{fs::OpenOptionsExt, io::AsRawHandle},
        path::PathBuf,
    };
    #[repr(C)]
    #[derive(Default)]
    struct Time {
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
        fn GetFileInformationByHandle(handle: *mut c_void, info: *mut Info) -> i32;
        fn GetFinalPathNameByHandleW(
            handle: *mut c_void,
            path: *mut u16,
            size: u32,
            flags: u32,
        ) -> u32;
        fn GetFileType(handle: *mut c_void) -> u32;
    }
    fn local_path(path: &Path) -> Result<PathBuf> {
        let raw = path.to_str().ok_or("executable_guard_path")?;
        let raw = raw.strip_prefix(r"\\?\").unwrap_or(raw);
        let b = raw.as_bytes();
        if b.len() < 4
            || !b[0].is_ascii_alphabetic()
            || b[1] != b':'
            || b[2] != b'\\'
            || raw[2..].contains(':')
            || raw
                .chars()
                .any(|c| c.is_control() || matches!(c, '/' | '?' | '*' | '"' | '<' | '>' | '|'))
            || raw[3..].split('\\').any(|c| {
                c.is_empty() || c == "." || c == ".." || c.ends_with('.') || c.ends_with(' ')
            })
        {
            return Err("executable_guard_path");
        }
        Ok(PathBuf::from(raw))
    }
    pub(super) fn verify(file: &File, path: &Path, directory: bool) -> Result<()> {
        let mut info = Info::default();
        // SAFETY: File owns the handle; Info mirrors BY_HANDLE_FILE_INFORMATION.
        if unsafe { GetFileType(file.as_raw_handle()) } != 1
            || unsafe { GetFileInformationByHandle(file.as_raw_handle(), &mut info) } == 0
            || info.attrs & 0x400 != 0
            || (info.attrs & 0x10 != 0) != directory
            || (!directory && info.links != 1)
        {
            return Err("executable_guard_identity");
        }
        let mut buffer = vec![0u16; 4097];
        // SAFETY: The live handle and writable UTF-16 buffer remain valid.
        let n = unsafe {
            GetFinalPathNameByHandleW(
                file.as_raw_handle(),
                buffer.as_mut_ptr(),
                buffer.len() as u32,
                0,
            )
        } as usize;
        if n == 0 || n >= buffer.len() {
            return Err("executable_guard_path");
        }
        let final_path = String::from_utf16(&buffer[..n]).map_err(|_| "executable_guard_path")?;
        let expected = path
            .to_str()
            .ok_or("executable_guard_path")?
            .strip_prefix(r"\\?\")
            .unwrap_or(path.to_str().unwrap());
        if !final_path
            .strip_prefix(r"\\?\")
            .is_some_and(|p| p.eq_ignore_ascii_case(expected))
        {
            return Err("executable_guard_path");
        }
        Ok(())
    }
    fn hold(path: &Path, directory: bool) -> Result<File> {
        let mut options = OpenOptions::new();
        options
            .read(true)
            .share_mode(1)
            .custom_flags(0x00200000 | 0x02000000);
        if directory {
            options.access_mode(0);
        }
        let file = options.open(path).map_err(|_| "executable_guard_open")?;
        verify(&file, path, directory)?;
        Ok(file)
    }
    pub(super) fn open(path: &Path) -> Result<(File, Vec<File>)> {
        let path = local_path(path)?;
        // Lock root-to-leaf before resolving each next component. OPEN_REPARSE_POINT
        // alone protects only the final component; held ancestors close that gap.
        let mut parents = Vec::new();
        for parent in path
            .ancestors()
            .skip(1)
            .collect::<Vec<_>>()
            .into_iter()
            .rev()
        {
            parents.push(hold(parent, true)?);
        }
        let file = hold(&path, false)?;
        Ok((file, parents))
    }
}
