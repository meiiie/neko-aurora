//! Private native witness byte transport. This module never grants task authority.
use serde::Deserialize;
#[cfg(any(windows, test))]
use serde::Serialize;
use std::io::{BufRead, Read, Write};

const MAX_INPUT: u64 = 8192;
#[cfg(any(windows, test))]
const MAX_WITNESS: usize = 4096;
#[cfg(any(windows, test))]
const SAFE: u64 = 9_007_199_254_740_991;
type Result<T> = std::result::Result<T, &'static str>;
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Init {
    version: u32,
    path: String,
}
#[cfg(any(windows, test))]
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct ReadRequest {
    version: u32,
    sequence: u64,
}
#[cfg(any(windows, test))]
#[derive(Serialize)]
struct Reply<'a> {
    version: u32,
    sequence: u64,
    witness: &'a str,
}
fn frame(input: &mut impl BufRead) -> Result<Option<Vec<u8>>> {
    let mut bytes = Vec::new();
    let n = (&mut *input)
        .take(MAX_INPUT + 1)
        .read_until(b'\n', &mut bytes)
        .map_err(|_| "witness_reader_input")?;
    if n == 0 {
        return Ok(None);
    }
    if n as u64 > MAX_INPUT || bytes.last() != Some(&b'\n') {
        return Err("witness_reader_frame");
    }
    bytes.pop();
    Ok(Some(
        crate::canonical(&bytes).map_err(|_| "witness_reader_frame")?,
    ))
}
#[cfg(any(windows, test))]
fn serve_with<R: BufRead, W: Write, F: FnMut() -> Result<Vec<u8>>>(
    input: &mut R,
    output: &mut W,
    mut read: F,
) -> Result<()> {
    let mut expected = 1;
    while let Some(raw) = frame(input)? {
        let request: ReadRequest =
            serde_json::from_slice(&raw).map_err(|_| "witness_reader_frame")?;
        if request.version != 1 || request.sequence != expected || request.sequence > SAFE {
            return Err("witness_reader_sequence");
        }
        let bytes = read()?;
        if bytes.len() > MAX_WITNESS {
            return Err("witness_reader_bound");
        }
        let witness = std::str::from_utf8(&bytes).map_err(|_| "witness_reader_utf8")?;
        let reply = serde_json::to_vec(&Reply {
            version: 1,
            sequence: expected,
            witness,
        })
        .map_err(|_| "witness_reader_output")?;
        if reply.len() + 1 > 24576 {
            return Err("witness_reader_bound");
        }
        output
            .write_all(&reply)
            .and_then(|_| output.write_all(b"\n"))
            .and_then(|_| output.flush())
            .map_err(|_| "witness_reader_output")?;
        expected += 1;
    }
    Ok(())
}
/// No alternate reader or bypass is selectable through the command protocol.
pub fn serve(input: &mut impl BufRead, output: &mut impl Write) -> Result<()> {
    let raw = frame(input)?.ok_or("witness_reader_init")?;
    let init: Init = serde_json::from_slice(&raw).map_err(|_| "witness_reader_init")?;
    if init.version != 1 || init.path.is_empty() || init.path.len() > 4096 {
        return Err("witness_reader_init");
    }
    #[cfg(windows)]
    {
        let session = windows::Session::open(&init.path)?;
        serve_with(input, output, || session.read())
    }
    #[cfg(not(windows))]
    {
        let _ = output;
        Err("witness_reader_windows_required")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;
    #[test]
    fn replies_are_bounded_sequenced_original_bytes_and_eof_closes() {
        let mut input =
            Cursor::new(b"{\"version\":1,\"sequence\":1}\n{\"version\":1,\"sequence\":2}\n");
        let mut output = Vec::new();
        let mut reads = 0;
        serve_with(&mut input, &mut output, || {
            reads += 1;
            Ok(format!("{{\"revision\":{reads}}}").into_bytes())
        })
        .unwrap();
        let lines: Vec<serde_json::Value> = String::from_utf8(output)
            .unwrap()
            .lines()
            .map(|l| serde_json::from_str(l).unwrap())
            .collect();
        assert_eq!(reads, 2);
        assert_eq!(lines[0]["sequence"], 1);
        assert_eq!(lines[1]["witness"], "{\"revision\":2}");
    }
    #[test]
    fn bad_sequence_duplicate_keys_partial_frame_and_overflow_never_read() {
        for bytes in [
            b"{\"version\":1,\"sequence\":2}\n".to_vec(),
            b"{\"version\":1,\"sequence\":1,\"sequence\":1}\n".to_vec(),
            b"{\"version\":1,\"sequence\":1}".to_vec(),
            vec![b'x'; 8193],
        ] {
            let mut reads = 0;
            let mut output = Vec::new();
            assert!(serve_with(&mut Cursor::new(bytes), &mut output, || {
                reads += 1;
                Ok(vec![])
            })
            .is_err());
            assert_eq!(reads, 0);
            assert!(output.is_empty());
        }
    }
    #[test]
    fn reader_failure_or_invalid_bytes_never_emit_success() {
        for value in [Err("fixture_failed"), Ok(vec![b'x'; 4097]), Ok(vec![0xff])] {
            let mut output = Vec::new();
            assert!(serve_with(
                &mut Cursor::new(b"{\"version\":1,\"sequence\":1}\n"),
                &mut output,
                || value.clone()
            )
            .is_err());
            assert!(output.is_empty());
        }
    }
    #[test]
    fn changing_path_in_read_frame_is_not_supported() {
        let mut output = Vec::new();
        assert!(serve_with(
            &mut Cursor::new(b"{\"version\":1,\"sequence\":1,\"path\":\"elsewhere\"}\n"),
            &mut output,
            || panic!("must not read")
        )
        .is_err());
    }
    #[test]
    fn replay_after_one_response_stops_before_a_second_read() {
        let mut output = Vec::new();
        let mut calls = 0;
        let result = serve_with(
            &mut Cursor::new(b"{\"version\":1,\"sequence\":1}\n{\"version\":1,\"sequence\":1}\n"),
            &mut output,
            || {
                calls += 1;
                Ok(b"{}".to_vec())
            },
        );
        assert!(result.is_err());
        assert_eq!(calls, 1);
        assert_eq!(output.iter().filter(|b| **b == b'\n').count(), 1);
    }
    #[cfg(windows)]
    #[test]
    fn native_reader_renews_by_name_and_rejects_hardlink_and_parent_replacement() {
        use std::fs;
        let root = std::env::temp_dir().join(format!(
            "neko-witness-native-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir(&root).unwrap();
        let private = root.join("private");
        fs::create_dir(&private).unwrap();
        let path = private.join("witness.json");
        fs::write(&path, b"{\"revision\":1}").unwrap();
        let session = windows::Session::open(path.to_str().unwrap()).unwrap();
        assert_eq!(session.read().unwrap(), b"{\"revision\":1}");
        let next = private.join("next");
        fs::write(&next, b"{\"revision\":2}").unwrap();
        fs::rename(next, &path).unwrap();
        assert_eq!(session.read().unwrap(), b"{\"revision\":2}");
        let alias = private.join("alias");
        fs::hard_link(&path, &alias).unwrap();
        assert!(session.read().is_err());
        fs::remove_file(alias).unwrap();
        fs::rename(&private, root.join("old")).unwrap();
        fs::create_dir(&private).unwrap();
        fs::write(&path, b"{\"revision\":3}").unwrap();
        assert!(session.read().is_err());
        drop(session);
        fs::remove_dir_all(root).unwrap();
    }
    #[cfg(windows)]
    #[test]
    fn native_reader_rejects_directory_junction_ancestry() {
        use std::{fs, process::Command};
        let root = std::env::temp_dir().join(format!(
            "neko-witness-junction-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        fs::create_dir(&root).unwrap();
        let real = root.join("real");
        fs::create_dir(&real).unwrap();
        fs::write(real.join("witness.json"), b"{}").unwrap();
        let alias = root.join("alias");
        let result = Command::new("cmd.exe")
            .args(["/d", "/c", "mklink", "/J"])
            .arg(&alias)
            .arg(&real)
            .output()
            .unwrap();
        assert!(
            result.status.success(),
            "fixture junction creation must succeed for native acceptance"
        );
        assert!(windows::Session::open(alias.join("witness.json").to_str().unwrap()).is_err());
        fs::remove_dir(alias).unwrap();
        fs::remove_dir_all(root).unwrap();
    }
    #[cfg(not(windows))]
    #[test]
    fn production_command_has_no_posix_fallback() {
        let mut output = Vec::new();
        assert_eq!(
            serve(
                &mut Cursor::new(b"{\"version\":1,\"path\":\"/tmp/auth.json\"}\n"),
                &mut output
            ),
            Err("witness_reader_windows_required")
        );
        assert!(output.is_empty());
    }
}

// Native evidence must be executed on Windows before Core activation. Handle flags
// and identity semantics follow Microsoft CreateFile/GetFileInformationByHandle/
// GetFinalPathNameByHandleW, not Bun FFI or a best-effort filesystem fallback.
#[cfg(windows)]
mod windows {
    use super::*;
    use std::{
        ffi::c_void,
        fs::{File, OpenOptions},
        os::windows::{fs::OpenOptionsExt, io::AsRawHandle},
        path::{Path, PathBuf},
    };
    const REPARSE: u32 = 0x400;
    const DIRECTORY: u32 = 0x10;
    #[repr(C)]
    #[derive(Default, PartialEq, Eq)]
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
    fn open(path: &Path, content: bool) -> Result<File> {
        let mut options = OpenOptions::new();
        options
            .read(true)
            .share_mode(1 | 2 | 4)
            .custom_flags(0x00200000 | 0x02000000);
        if !content {
            options.access_mode(0);
        }
        options.open(path).map_err(|_| "witness_reader_open")
    }
    fn info(file: &File) -> Result<Info> {
        let mut info = Info::default();
        // SAFETY: File owns a live Windows handle; Info exactly matches the Win32 ABI.
        if unsafe { GetFileType(file.as_raw_handle()) } != 1
            || unsafe { GetFileInformationByHandle(file.as_raw_handle(), &mut info) } == 0
        {
            return Err("witness_reader_identity");
        }
        Ok(info)
    }
    fn same(a: &Info, b: &Info) -> bool {
        a.volume == b.volume && a.index_hi == b.index_hi && a.index_lo == b.index_lo
    }
    fn stable(a: &Info, b: &Info) -> bool {
        same(a, b)
            && a.attrs == b.attrs
            && a.created == b.created
            && a.write == b.write
            && a.size_hi == b.size_hi
            && a.size_lo == b.size_lo
            && a.links == b.links
    }
    fn final_path(file: &File, path: &Path) -> Result<()> {
        let mut buffer = vec![0u16; 4097];
        // SAFETY: Writable UTF-16 buffer length is supplied; file handle remains live.
        let count = unsafe {
            GetFinalPathNameByHandleW(
                file.as_raw_handle(),
                buffer.as_mut_ptr(),
                buffer.len() as u32,
                0,
            )
        } as usize;
        if count == 0 || count >= buffer.len() {
            return Err("witness_reader_path");
        }
        let actual = String::from_utf16(&buffer[..count]).map_err(|_| "witness_reader_path")?;
        let actual = actual.strip_prefix(r"\\?\").ok_or("witness_reader_path")?;
        if !actual.eq_ignore_ascii_case(path.to_str().ok_or("witness_reader_path")?) {
            return Err("witness_reader_path");
        }
        Ok(())
    }
    fn directory(file: &File, path: &Path) -> Result<Info> {
        let value = info(file)?;
        if value.attrs & REPARSE != 0 || value.attrs & DIRECTORY == 0 {
            return Err("witness_reader_directory");
        }
        final_path(file, path)?;
        Ok(value)
    }
    fn regular(file: &File, path: &Path) -> Result<Info> {
        let value = info(file)?;
        if value.attrs & (REPARSE | DIRECTORY) != 0
            || value.links != 1
            || value.size_hi != 0
            || value.size_lo as usize > MAX_WITNESS
        {
            return Err("witness_reader_file");
        }
        final_path(file, path)?;
        Ok(value)
    }
    pub(super) struct Session {
        path: PathBuf,
        parents: Vec<(PathBuf, File, Info)>,
    }
    impl Session {
        pub(super) fn open(raw: &str) -> Result<Self> {
            let bytes = raw.as_bytes();
            // Local DOS absolute paths only: no UNC/device namespace, ADS or lexical aliases.
            if bytes.len() < 4
                || !bytes[0].is_ascii_alphabetic()
                || bytes[1] != b':'
                || bytes[2] != b'\\'
                || raw
                    .chars()
                    .any(|c| c.is_control() || matches!(c, '/' | '?' | '*' | '"' | '<' | '>' | '|'))
                || raw[2..].contains(':')
                || raw[3..].split('\\').any(|c| {
                    c.is_empty() || c == "." || c == ".." || c.ends_with('.') || c.ends_with(' ')
                })
            {
                return Err("witness_reader_path");
            }
            let path = PathBuf::from(raw);
            let mut parents = Vec::new();
            for parent in path
                .ancestors()
                .skip(1)
                .collect::<Vec<_>>()
                .into_iter()
                .rev()
            {
                let file = open(parent, false)?;
                let snapshot = directory(&file, parent)?;
                parents.push((parent.to_path_buf(), file, snapshot));
            }
            if parents.is_empty() {
                return Err("witness_reader_path");
            }
            Ok(Self { path, parents })
        }
        fn parents_current(&self) -> Result<()> {
            for (path, held, prior) in &self.parents {
                let current = directory(held, path)?;
                let named = open(path, false)?;
                let named = directory(&named, path)?;
                if !same(prior, &current) || !same(&current, &named) {
                    return Err("witness_reader_directory");
                }
            }
            Ok(())
        }
        pub(super) fn read(&self) -> Result<Vec<u8>> {
            self.parents_current()?;
            let mut file = open(&self.path, true)?;
            let before = regular(&file, &self.path)?;
            let mut bytes = Vec::new();
            (&mut file)
                .take((MAX_WITNESS + 1) as u64)
                .read_to_end(&mut bytes)
                .map_err(|_| "witness_reader_read")?;
            let after = regular(&file, &self.path)?;
            let named = open(&self.path, true)?;
            let named = regular(&named, &self.path)?;
            if bytes.len() > MAX_WITNESS
                || bytes.len() != before.size_lo as usize
                || !stable(&before, &after)
                || !stable(&after, &named)
            {
                return Err("witness_reader_changed");
            }
            self.parents_current()?;
            Ok(bytes)
        }
    }
}
