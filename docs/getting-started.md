# Bắt đầu với Neko Aurora

[← Tổng quan](../README.md) · [Trạng thái hỗ trợ](project-status.md) · [Đóng góp](../CONTRIBUTING.md)

Đây là hướng dẫn build development từ source trên Windows, không phải installer hoặc cam kết production. Mọi lệnh dưới đây chạy từ root của checkout mới; không ghép vào Wiii hay state đang dùng.

## Build và chạy local trên Windows

Hướng dẫn sau dùng PowerShell 7, từ root repository. Cần Git, Go **1.27+** theo `app/native/go.mod`, Rustup và Microsoft C++ Build Tools x64 kèm Windows SDK. `rust-toolchain.toml` pin Rust **1.97.1**; helper `tools/cargo-windows.ps1` tìm MSVC/SDK đã cài và cấu hình môi trường riêng cho lệnh Cargo. Helper không cài compiler hay đổi PATH toàn máy. Tauri cần Microsoft Edge WebView2 Runtime. Dependency Rust có thể cần tải ở lần build đầu; Go module hiện không có dependency bên thứ ba.

```powershell
git clone https://github.com/meiiie/neko-aurora.git
Set-Location neko-aurora
rustup toolchain install 1.97.1 --profile minimal --component rustfmt --component clippy
New-Item -ItemType Directory -Force app/.build | Out-Null

Push-Location app/native
try {
    go build -o ../.build/neko-aurora.exe ./cmd/neko-aurora
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
} finally { Pop-Location }

pwsh -NoProfile -File tools/cargo-windows.ps1 build --locked --manifest-path app/host/Cargo.toml --target-dir app/.build/host-target
if ($LASTEXITCODE -ne 0) { throw 'Host build failed' }
Copy-Item app/.build/host-target/debug/neko-host.exe app/.build/neko-host.exe
```

Chạy Go và mở `http://127.0.0.1:8765` trong trình duyệt:

```powershell
./app/.build/neko-aurora.exe --state ./app/.state/local --listen 127.0.0.1:8765
```

Port phải còn trống; Ctrl+C dừng service. Dữ liệu được ghi dưới state đã chọn. Khi không có cấu hình Core được chấp thuận, tin nhắn được lưu và chờ runtime (`waiting_host`), không tự gọi provider. Lưu tài khoản không đồng nghĩa với cấp quyền hoặc xác minh tài khoản đó. Không đưa state, receipt chứa challenge hay credentials vào Git.

### Tauri desktop

Build shell:

```powershell
pwsh -NoProfile -File tools/cargo-windows.ps1 build --locked --manifest-path app/desktop/Cargo.toml --target-dir app/.build/desktop-target
if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
Copy-Item app/.build/desktop-target/debug/neko-desktop.exe app/.build/neko-desktop.exe
```

Shell hiện tại yêu cầu startup receipt và xác minh process/port/hash/state, nên không mở chỉ bằng double-click hoặc launcher `run-local.ps1 -Desktop` cũ. Tạo manifest hash **development** cho chính các binary vừa build, rồi dùng launcher hiện tại:

```powershell
function Get-LocalBinaryEntry([string]$Name) {
    $path = Join-Path 'app/.build' $Name
    @{ path = ".build/$Name"; sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant() }
}
$localManifest = @{
    version = 1
    scope = 'local-journal'
    appCommit = (git rev-parse HEAD).Trim()
    desktopLaunchContract = 1
    control = Get-LocalBinaryEntry 'neko-aurora.exe'
    host = Get-LocalBinaryEntry 'neko-host.exe'
    desktop = Get-LocalBinaryEntry 'neko-desktop.exe'
}
$localManifest | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 app/.build/dev-launch.json
./app/scripts/run-prebuilt-local.ps1 -Manifest app/.build/dev-launch.json -Desktop -CheckOnly
./app/scripts/run-prebuilt-local.ps1 -Manifest app/.build/dev-launch.json -Desktop
```

Manifest này chỉ ràng buộc hash của build local do bạn chọn; nó không phải chữ ký, installer hay chứng nhận release. Launcher tạo startup receipt mới, kiểm tra Go child trước khi mở WebView và chỉ dừng process do nó sở hữu. Chạy trình duyệt và desktop lần lượt, hoặc dùng `-Port` khác để tránh xung đột. Core/helper được pin bằng các hợp đồng riêng; build shell không tự kích hoạt agent runtime.

### Kiểm tra code

```powershell
Push-Location app/native
try {
    go test -count=1 ./...
    go vet ./...
} finally { Pop-Location }
$env:FIXTURE_SAVED_CORE_MODULE = (Resolve-Path core/src/adapters/saved-account-bridge.ts).Path
# Trỏ tới bun.exe thật, không dùng shim bun.ps1/bun.cmd cho child process Rust.
$env:FIXTURE_BUN_EXE = (Resolve-Path "$env:APPDATA/npm/node_modules/bun/bin/bun.exe").Path
pwsh -NoProfile -File tools/cargo-windows.ps1 test --locked --manifest-path app/host/Cargo.toml
```

Ví dụ đường dẫn Bun trên áp dụng cho cài đặt npm; thay bằng `bun.exe` của bản cài đang dùng nếu khác. Test host tạo Vault và khóa giả lập trong thư mục tạm, không dùng tài khoản thật. Fixture Railway trên Windows cần Git for Windows (`bash.exe`, `tar`) và Python trên PATH; có thể chỉ định shell bằng `AURORA_TEST_SHELL`. Lệnh sandbox dài được chuyển qua stdin để tránh giới hạn command line Windows. Các runner native/Core/browser khác cần binary, fixture và pin đúng hợp đồng. Không chạy một runner có đường dẫn/pin lịch sử rồi coi đó là kiểm thử bản hiện tại. Chỉ dùng `--offline` cho Cargo khi dependency đã có trong cache.

### Phát triển Neko Core

Core là project độc lập trong `core/`. CI được lưu kèm source pin Bun **1.4.0**; dùng lockfile thay vì tự nâng dependency:

```powershell
Push-Location core
try {
    bun install --frozen-lockfile
    bun run typecheck
    bun test
    # Build binary và các smoke gate do package.json định nghĩa:
    # bun run build
} finally { Pop-Location }
```

Đọc [Core working rules](../core/AGENTS.md) và [quy trình kiểm thử](../core/docs/process/TESTING.md) trước khi thay runtime. Không dùng tài khoản/provider thật để thay thế fixture kiểm thử.

## Khi gặp lỗi

- Không tìm thấy Go/Cargo: cài toolchain phù hợp và mở lại PowerShell có PATH đúng. Go tối thiểu theo `app/native/go.mod`; không hạ phiên bản module để né lỗi.
- Cargo offline thiếu package: bỏ `--offline` ở lần chuẩn bị dependency đã được cho phép; giữ `--locked`. Không thay lockfile chỉ để chạy build.
- Rust `stable` báo thiếu component hoặc `link.exe` không có trong PATH: chạy từ root để dùng bản pin, kiểm tra `rustc --version`, rồi dùng helper Cargo ở trên. Helper báo thiếu MSVC/SDK thì cần cài đúng C++ Build Tools; không đổi target sang GNU để che lỗi MSVC.
- Port 8765 bận: dừng process do bạn sở hữu hoặc chọn port khác. Với launcher desktop dùng `-Port`; không tự kết nối tới một service không xác minh.
- `waiting_host`: mặc định chưa có runtime được cấu hình, không phải bằng chứng provider đang chạy.
- Desktop từ chối receipt/hash/state: tạo lại manifest từ đúng các binary vừa build, dùng startup receipt mới qua launcher; không bỏ bước xác minh.
- Wiii không build standalone: thư mục `wiii/` chỉ là overlay. Xem [phạm vi Wiii](../README.md#wiii-integration).

Không đính kèm account store, state/journal thật, startup challenge, token hoặc log chứa chat khi báo lỗi. Dùng [hướng dẫn hỗ trợ](../SUPPORT.md).
