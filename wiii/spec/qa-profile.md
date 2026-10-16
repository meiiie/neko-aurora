# Isolated native QA profile

This additive debug-only CLI follows frozen18ca6a5/runtimeab6923b. Environment
APPDATA/LOCALAPPDATA overrides do not isolate Windows KnownFolder paths. No
default production paths or capabilities are changed by a no-argument launch.

Use an explicit existing empty owned directory on the E drive:

```
wiii-native-qa.exe --qa-profile-root E:\owned\wiii-qa-profile
wiii-native-qa.exe --qa-profile-root E:\owned\wiii-qa-profile --qa-service-port 12345
```

Unknown/duplicate options, missing or relative roots, unmarked non-empty roots,
links/reparse points anywhere in the retained profile, non-E Windows roots,
invalid fixture ports and all QA arguments in a release
build fail before native setup. A created marker permits only subsequent reuse
of that same isolated state. `--qa-profile-check` validates/prepares and prints
nonsecret metadata then exits before creating Tauri/window/network state.

Native runtime, computer and coworker databases and quick-chat folders use
profile `state`; frontend plugin-store paths use `state/stores` through a
main-only name resolver. The renderer supplies no root. Failed resolution
does not fall back to a relative user store. Plugin-store still invokes the
Tauri KnownFolder path resolver internally; its actual file path is absolute
inside the profile, so resolving a directory name is not reading user files.

QA creates the main WebView explicitly with profile `webview` data directory;
theme/localStorage/sessionStorage/cookies therefore use that WebView profile.
It skips the inherited global single-instance handler and tray so no existing
user instance receives focus/arguments. Computer event polling is stopped in
QA. Provider discovery, launch, grant and computer actions are rejected
by a QA-only app-command guard. These features show unavailable behavior; no
provider catalog or successful action is fabricated. Normal launches preserve
all existing commands and paths.

The read-only injected QA marker contains only active/service URL metadata,
not root or credentials. Settings use the explicit fixture URL. Native Wiii
HTTP accepts only that literal fixture port; absent port means disabled. The
QA CSP blocks external HTTP/WS, remote scripts/fonts/images and viewer frames.
It permits bundled self/asset resources and IPC. Default CSP/viewer authority
remains unchanged. Neko Bot bridge connection still needs the native owner
confirmation; the QA profile is not confirmation and does not create grants.

This is a reversible test profile, not an OS sandbox. Existing generic plugin
authorities remain separate; current application startup/store/transport paths
are the validation scope. No actual user profile, service, provider, native
window or confirmation is used while preparing it. Root owns later native UI
acceptance. Preserve18ca/binary6768/dist and do not run the default profile for
QA. Rollback by closing the QA candidate and reverting this branch; retain the
owned profile/logs for diagnosis, never substitute existing user state. The
pre-setup inventory rejects over100000 entries instead of accepting an
unbounded retained profile. This is a task-local debug QA hook, not a general
portable release-profile feature.
