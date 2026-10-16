use serde::Serialize;
use std::{ffi::OsString, fs, path::{Component, Path, PathBuf}};
use tauri::Manager;

const MARKER: &str = ".wiii-native-qa-profile-v1";
const MARKER_BYTES: &[u8] = b"wiii-native-qa-profile.v1\n";

#[derive(Clone, Default)]
pub struct NativeProfile {
    root: Option<PathBuf>,
    pub fixture_port: Option<u16>,
    pub check_only: bool,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct QaMetadata {
    pub active: bool,
    pub service_url: Option<String>,
}

fn error(code: &str) -> String { format!("wiii_qa_profile:{code}") }

fn linked(path: &Path) -> Result<bool, String> {
    let metadata = fs::symlink_metadata(path).map_err(|_| error("path_unavailable"))?;
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 { return Ok(true); }
    }
    Ok(metadata.file_type().is_symlink())
}

fn validate_root(path: &Path) -> Result<PathBuf, String> {
    if !path.is_absolute() || path.components().any(|c| matches!(c, Component::ParentDir | Component::CurDir)) {
        return Err(error("absolute_existing_root_required"));
    }
    #[cfg(windows)]
    if !matches!(path.components().next(), Some(Component::Prefix(prefix))
        if matches!(prefix.kind(), std::path::Prefix::Disk(b'E' | b'e') | std::path::Prefix::VerbatimDisk(b'E' | b'e'))) {
        return Err(error("owned_e_root_required"));
    }
    let mut part = PathBuf::new();
    for component in path.components() {
        part.push(component);
        if part.exists() && linked(&part)? { return Err(error("linked_root_denied")); }
    }
    if !path.is_dir() { return Err(error("absolute_existing_root_required")); }
    let canonical = path.canonicalize().map_err(|_| error("path_unavailable"))?;
    let entries = fs::read_dir(&canonical).map_err(|_| error("path_unavailable"))?
        .map(|e| e.map(|e| e.file_name()).map_err(|_| error("path_unavailable")))
        .collect::<Result<Vec<_>, _>>()?;
    if !entries.is_empty() {
        let marker = canonical.join(MARKER);
        if linked(&marker)? || fs::read(&marker).map_err(|_| error("unmarked_root_denied"))? != MARKER_BYTES {
            return Err(error("unmarked_root_denied"));
        }
        if entries.iter().any(|name| !matches!(name.to_str(), Some(MARKER | "state" | "webview" | "workspaces"))) {
            return Err(error("unexpected_root_entry"));
        }
    }
    let mut pending = vec![canonical.clone()];
    let mut count = 0usize;
    while let Some(directory) = pending.pop() {
        for entry in fs::read_dir(directory).map_err(|_| error("path_unavailable"))? {
            let entry = entry.map_err(|_| error("path_unavailable"))?;
            count += 1;
            if count > 100_000 { return Err(error("profile_inventory_limit")); }
            let path = entry.path();
            if linked(&path)? { return Err(error("linked_profile_entry_denied")); }
            if path.is_dir() { pending.push(path); }
            else if !path.is_file() { return Err(error("invalid_profile_entry")); }
        }
    }
    Ok(canonical)
}

impl NativeProfile {
    pub fn parse(args: impl IntoIterator<Item = OsString>, debug_allowed: bool) -> Result<Self, String> {
        let mut args = args.into_iter();
        let mut root = None;
        let mut port = None;
        let mut check_only = false;
        while let Some(arg) = args.next() {
            if !debug_allowed { return Err(error("qa_debug_build_required")); }
            match arg.to_str() {
                Some("--qa-profile-root") if root.is_none() => {
                    root = Some(PathBuf::from(args.next().ok_or_else(|| error("missing_root"))?));
                }
                Some("--qa-service-port") if port.is_none() => {
                    let raw = args.next().ok_or_else(|| error("missing_fixture_port"))?;
                    let raw = raw.to_str().ok_or_else(|| error("invalid_fixture_port"))?;
                    if raw.is_empty() || !raw.bytes().all(|b| b.is_ascii_digit()) { return Err(error("invalid_fixture_port")); }
                    let value = raw.parse::<u16>().map_err(|_| error("invalid_fixture_port"))?;
                    if value == 0 { return Err(error("invalid_fixture_port")); }
                    port = Some(value);
                }
                Some("--qa-profile-check") if !check_only => check_only = true,
                _ => return Err(error("unknown_or_duplicate_option")),
            }
        }
        if root.is_none() && (port.is_some() || check_only) { return Err(error("profile_root_required")); }
        let root = root.map(|p| validate_root(&p)).transpose()?;
        Ok(Self { root, fixture_port: port, check_only })
    }
    pub fn active(&self) -> bool { self.root.is_some() }
    pub fn metadata(&self) -> QaMetadata {
        QaMetadata { active: self.active(), service_url: self.fixture_port.map(|p| format!("http://127.0.0.1:{p}")) }
    }
    pub fn prepare(&self) -> Result<(), String> {
        if let Some(root) = &self.root {
            for name in ["state", "webview", "workspaces"] {
                let path = root.join(name);
                if path.exists() {
                    if linked(&path)? || !path.is_dir() { return Err(error("linked_profile_entry_denied")); }
                } else { fs::create_dir(&path).map_err(|_| error("profile_create_failed"))?; }
            }
            let stores = root.join("state/stores");
            if stores.exists() {
                if linked(&stores)? || !stores.is_dir() { return Err(error("linked_profile_entry_denied")); }
            } else { fs::create_dir(&stores).map_err(|_| error("profile_create_failed"))?; }
            if !root.join(MARKER).exists() {
                use std::io::Write;
                let mut marker = fs::OpenOptions::new().write(true).create_new(true).open(root.join(MARKER)).map_err(|_| error("marker_create_failed"))?;
                marker.write_all(MARKER_BYTES).map_err(|_| error("marker_create_failed"))?;
                marker.sync_all().map_err(|_| error("marker_create_failed"))?;
            }
        }
        Ok(())
    }
    pub fn resolve_state(&self, normal: impl FnOnce() -> Result<PathBuf, String>) -> Result<PathBuf, String> {
        match &self.root { Some(root) => Ok(root.join("state")), None => normal() }
    }
    pub fn webview_dir(&self) -> Option<PathBuf> { self.root.as_ref().map(|p| p.join("webview")) }
    pub fn store_path(&self, name: &str) -> Result<String, String> {
        if let Some(root) = &self.root {
            if name.is_empty() || name.len() > 160 || matches!(name, "." | "..")
                || !name.bytes().all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.')) {
                return Err(error("invalid_store_name"));
            }
            let path = root.join("state/stores").join(name);
            if path.exists() && linked(&path)? { return Err(error("linked_store_denied")); }
            return path.to_str().map(str::to_owned).ok_or_else(|| error("invalid_store_path"));
        }
        Ok(name.to_owned())
    }
    pub fn service_allowed(&self, port: Option<u16>, remote: bool) -> Result<(), String> {
        if self.active() && (remote || self.fixture_port.is_none() || self.fixture_port != port) {
            return Err(error("service_not_explicit_fixture"));
        }
        Ok(())
    }
    pub fn command_allowed(&self, command: &str) -> bool {
        !self.active() || matches!(command,
            "neko_qa_store_path" | "neko_qa_runtime_profile" | "close_splash"
            | "neko_bot_local_status" | "neko_bot_local_connect" | "neko_bot_local_disconnect"
            | "neko_bot_local_link" | "neko_bot_local_request"
            | "neko_wiii_service_fetch" | "neko_wiii_service_cancel"
            | "neko_create_quick_chat_workspace" | "neko_control_session_list" | "neko_control_events_read")
    }
    pub fn initialization_script(&self) -> Result<Option<String>, String> {
        if !self.active() { return Ok(None); }
        let json = serde_json::to_string(&self.metadata()).map_err(|_| error("metadata_failed"))?;
        Ok(Some(format!("Object.defineProperty(window,'__WIII_QA_NATIVE_PROFILE__',{{value:Object.freeze({json}),writable:false,configurable:false}});")))
    }
}

pub fn data_dir(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    app.state::<NativeProfile>().resolve_state(|| app.path().app_local_data_dir().map_err(|_| error("default_path_unavailable")))
}

pub const QA_CSP: &str = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' asset: http://asset.localhost blob: data:; connect-src 'self' ipc: http://ipc.localhost; worker-src 'self' blob:; frame-src 'self' blob:; media-src 'self' blob: data:; object-src 'none'; base-uri 'self'; form-action 'self'";

#[cfg(test)]
mod tests {
    use super::*;
    fn root() -> PathBuf {
        let root = std::env::temp_dir().join(format!("wiii-profile-unit-{}", uuid::Uuid::new_v4()));
        fs::create_dir(&root).unwrap(); root
    }
    fn args(root: &Path) -> Vec<OsString> { vec!["--qa-profile-root".into(), root.as_os_str().to_owned()] }
    fn cleanup(root: PathBuf) {
        let temp = std::env::temp_dir().canonicalize().unwrap();
        let resolved = root.canonicalize().unwrap();
        assert!(resolved.starts_with(temp));
        assert!(root.file_name().unwrap().to_string_lossy().starts_with("wiii-profile-unit-"));
        fs::remove_dir_all(resolved).unwrap();
    }
    #[test]
    fn no_args_preserves_default_paths_and_authority() {
        let profile = NativeProfile::parse([], false).unwrap();
        assert!(!profile.active());
        assert_eq!(profile.resolve_state(|| Ok(PathBuf::from("default-fixture"))).unwrap(), PathBuf::from("default-fixture"));
        assert_eq!(profile.store_path("settings.json").unwrap(), "settings.json");
        assert!(profile.service_allowed(None, true).is_ok());
        assert!(profile.command_allowed("neko_control_provider_sessions"));
        assert!(profile.initialization_script().unwrap().is_none());
    }
    #[test]
    fn invalid_options_fail_before_preparation() {
        for values in [vec!["--profile-root", "relative"], vec!["--qa-service-port", "1"], vec!["--qa-profile-check"], vec!["--qa-profile-root", "relative"], vec!["--qa-profile-root"]] {
            assert!(NativeProfile::parse(values.into_iter().map(OsString::from), true).is_err());
        }
        let root = root();
        assert!(NativeProfile::parse(args(&root), false).is_err());
        for port in ["0", "65536", "+12", "-1", "12.0"] {
            let mut input = args(&root); input.extend(["--qa-service-port".into(), port.into()]);
            assert!(NativeProfile::parse(input, true).is_err());
        }
        let mut duplicate = args(&root); duplicate.extend(args(&root));
        assert!(NativeProfile::parse(duplicate, true).is_err());
        assert_eq!(fs::read_dir(&root).unwrap().count(), 0);
        fs::remove_dir(root).unwrap();
    }
    #[test]
    fn owned_profile_isolated_and_reusable_without_default_resolver() {
        let root = root();
        let profile = NativeProfile::parse(args(&root), true).unwrap(); profile.prepare().unwrap();
        let state = profile.resolve_state(|| panic!("KnownFolder fallback must not be called")).unwrap();
        assert!(state.starts_with(root.canonicalize().unwrap()));
        assert!(PathBuf::from(profile.store_path("auth.json").unwrap()).starts_with(&state));
        assert!(profile.webview_dir().unwrap().starts_with(root.canonicalize().unwrap()));
        for name in ["../settings.json", "C:\\settings.json", ".", "..", "x:y", "file/name"] { assert!(profile.store_path(name).is_err()); }
        fs::write(state.join("stores/settings.json"), b"{\"owned\":true}").unwrap();
        let again = NativeProfile::parse(args(&root), true).unwrap(); again.prepare().unwrap();
        assert_eq!(fs::read(again.store_path("settings.json").unwrap()).unwrap(), b"{\"owned\":true}");
        cleanup(root);
    }
    #[test]
    fn unmarked_or_unexpected_state_is_rejected() {
        let root = root(); fs::write(root.join("user-data"), b"canary").unwrap();
        assert!(NativeProfile::parse(args(&root), true).is_err());
        assert_eq!(fs::read(root.join("user-data")).unwrap(), b"canary");
        cleanup(root);
    }
    #[test]
    fn actual_native_journals_open_only_beneath_owned_state() {
        let root = root();
        let profile = NativeProfile::parse(args(&root), true).unwrap(); profile.prepare().unwrap();
        let state = profile.resolve_state(|| panic!("default user data resolver must not run")).unwrap();
        let runtime = crate::neko::runtime::NekoRuntime::open(&state.join("neko-runtime-v1.sqlite3")).unwrap();
        let computer = crate::neko::computer::NekoComputerService::open(&state).unwrap();
        let coworker = crate::neko::coworker::CoworkerRecords::open(&state.join("neko-coworker-v1/records-v1.sqlite3")).unwrap();
        assert!(state.join("neko-runtime-v1.sqlite3").is_file());
        assert!(state.join("neko-computer-v1/computer-v1.sqlite3").is_file());
        assert!(state.join("neko-coworker-v1/records-v1.sqlite3").is_file());
        drop((runtime, computer, coworker));
        assert!(NativeProfile::parse(args(&root), true).is_ok());
        cleanup(root);
    }
    #[test]
    fn qa_authority_and_network_are_explicitly_bounded() {
        let root = root(); let mut input = args(&root); input.extend(["--qa-service-port".into(), "12345".into()]);
        let profile = NativeProfile::parse(input, true).unwrap();
        assert!(profile.service_allowed(Some(12345), false).is_ok());
        for (port, remote) in [(None,true),(Some(8000),false),(Some(12345),true)] { assert!(profile.service_allowed(port,remote).is_err()); }
        assert!(NativeProfile::parse(args(&root), true).unwrap().service_allowed(Some(12345),false).is_err());
        for command in ["neko_control_provider_list","neko_control_provider_profiles","neko_control_provider_sessions","neko_control_session_start","neko_computer_project_grant","check_server_reachable"] { assert!(!profile.command_allowed(command)); }
        assert!(profile.command_allowed("neko_bot_local_status"));
        assert!(!QA_CSP.contains("https:")); assert!(!QA_CSP.contains("ws:"));
        let script = profile.initialization_script().unwrap().unwrap();
        assert!(script.contains("writable:false")); assert!(!script.contains("root"));
        cleanup(root);
    }
}
