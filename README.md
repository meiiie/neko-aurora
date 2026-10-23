# Neko Aurora

<picture>
  <source media="(max-width: 600px)" srcset="docs/assets/neko-bot-banner-mobile.svg">
  <img src="docs/assets/neko-bot-banner.svg" alt="Neko Aurora với logo Neko Peek: đa bot, đa kênh, ưu tiên dữ liệu cục bộ; bản đang phát triển" width="1200">
</picture>

![Đang phát triển](docs/assets/badges/development.svg) ![Tauri, Rust và Go](docs/assets/badges/stack.svg) ![Runtime: Neko Core](docs/assets/badges/runtime.svg)

Trợ lý đa bot, đa kênh theo hướng **local-first**. Neko Aurora hướng tới việc phối hợp nhiều bot có danh tính và chỉ dẫn riêng, giữ lịch sử công việc bền vững, nhận tin mới khi tác vụ cũ đang chạy và trả kết quả vào đúng kênh được phép.

**Trạng thái: bản phát triển, chưa phải sản phẩm production hoàn chỉnh.** Repository này tập hợp mã nguồn ứng dụng, Neko Core liên quan và một overlay tích hợp Wiii. Không kèm installer, tài khoản, credentials, dữ liệu chat hay runtime đã cấu hình sẵn.

Mốc source: [`v0.1.0-alpha.1`](https://github.com/meiiie/neko-aurora/tree/v0.1.0-alpha.1). Xem [ghi chú alpha](docs/releases.md) về version và các gate còn mở. Tag này không kèm binary hoặc installer.

[![Deploy on Railway](https://railway.com/button.svg)](https://railway.com/new/template?template=https%3A%2F%2Fgithub.com%2Fmeiiie%2Fneko-aurora) — máy chủ nhiều người dùng một chạm, xem [docs/railway-template.md](docs/railway-template.md).

[Bắt đầu](#build-và-chạy-local-trên-windows) · [Tài liệu](docs/README.md) · [Ảnh giao diện](#ảnh-giao-diện) · [Roadmap](#roadmap) · [Đóng góp](CONTRIBUTING.md)

## Ảnh giao diện

Ảnh ứng dụng web local đã ghi nhận ở **`2a74029`**, dùng Go service và Chrome headless với **dữ liệu fixture**, không có chat hoặc tài khoản người dùng thật. Ảnh giữ nguyên byte và không phải mockup; đây là bản cũ, không phải bằng chứng kiểm thử checkout hiện tại, Tauri native hay điện thoại thật.

<picture>
  <source media="(max-width: 600px)" srcset="docs/assets/screenshots/report-mobile-2a74029.png">
  <img src="docs/assets/screenshots/report-desktop-2a74029.png" alt="Ảnh kiểm thử local 2a74029: bot gửi báo cáo và file vào kênh chung, dữ liệu fixture" width="1440">
</picture>

[Xem ảnh desktop đầy đủ](docs/assets/screenshots/report-desktop-2a74029.png) · [Xem ảnh mobile viewport](docs/assets/screenshots/report-mobile-2a74029.png) · [Nguồn và giới hạn ảnh](docs/assets/README.md)

## Kiến trúc

```mermaid
flowchart LR
    UI["Neko Aurora UI · Tauri"] --> Go["Go · API, journal, điều phối"]
    Go --> Host["Rust · trusted host, process và effect boundary"]
    Host --> Core["Neko Core · agent loop"]
    Wiii["Wiii · ADE riêng, tùy chọn"] --> Go
```

Neko Core sở hữu vòng lặp agent. Go quản lý trạng thái và hợp đồng thực thi; Rust giữ các ranh giới tin cậy trên máy. Wiii là giao diện ADE riêng, không thay thế Neko Aurora UI.

| Thư mục | Vai trò |
|---|---|
| `app/native/` | Go control plane, HTTP/SSE local, journal, bot/kênh/project/task/file |
| `app/host/` | Rust trusted host, secure input, process và receipt |
| `app/desktop/` | Tauri desktop shell |
| `core/` | Neko Core, TypeScript/Bun; runtime và adapter liên quan |
| `wiii/` | 61 file overlay tích hợp Wiii; **không phải full Wiii repository** |
| `migration/` | Provenance, manifest source và workflow được lưu ở trạng thái không tự chạy |

## Hiện có và giới hạn kiểm chứng

Các chức năng dưới đây có source và bằng chứng kiểm thử local đã ghi trong tài liệu của từng lát cắt. Chúng không phải cam kết tương thích mọi provider, hệ điều hành hay thiết bị.

| Phạm vi | Hiện có | Giới hạn |
|---|---|---|
| Bot và hội thoại | Nhiều bot, ID/alias ổn định, chỉ dẫn riêng; hội thoại riêng và kênh chung local | Việc chia sẻ ngữ cảnh phụ thuộc scope/ACL; không có quyền đọc mọi kênh mặc định |
| Công việc bền vững | Journal, idempotency/receipt, phục hồi, hủy và các control có hợp đồng riêng | Một số trạng thái cần thao tác phục hồi tường minh; không tuyên bố exactly-once cho mọi effect bên ngoài |
| Tương tác đang chạy | Foreground/background, bổ sung/thay thế yêu cầu và nhóm cộng tác hữu hạn | Bằng chứng native/Core dùng provider loopback có kiểm soát, chưa là đánh giá model thật |
| Project và file | Nguồn UTF-8/DOCX có giới hạn, chọn nguồn liên quan, preview/download kết quả theo scope | Không phải dịch vụ lưu trữ hoặc chia sẻ file cloud hoàn chỉnh |
| Tài khoản và runtime | Secure input qua Rust/Windows DPAPI, metadata và pin runtime/profile | Tài khoản lưu vẫn có thể là `stored_unverified`; broker/OAuth và vòng đời provider thật chưa nghiệm thu |
| Giao diện | UI thích ứng, kiểm thử trình duyệt về interaction/focus và viewport | Chưa phải ứng dụng iOS/Android được phát hành; chưa nghiệm thu điện thoại thật hoặc AT speech |

HTTP/SSE local và mô phỏng hai client **không phải cloud sync**. Cloud/device pairing, đồng bộ nhiều máy, đánh giá provider/model thật, đóng gói/phát hành và các gate thiết bị vẫn là phần tiếp tục phát triển. Saved-account witness là WIP, không phải đường thực thi mặc định đã hoàn tất.

Xem [ma trận khoảng trống sản phẩm](app/docs/product-gap-matrix-20261011.md), [hợp đồng desktop instance](app/docs/desktop-local-instance-20261012.md) và các tài liệu delivery/review trong `app/docs/`. Các log/capture và binary của những lần kiểm thử lịch sử không được đưa vào repository này. Kết quả lịch sử chỉ áp dụng cho commit và môi trường đã ghi, không thay cho một lần kiểm thử checkout hiện tại.

## Build và chạy local trên Windows

Cần Git, Go **1.27+**, Rust/Cargo (MSVC + Windows SDK) và WebView2 cho Tauri. Hướng dẫn đầy đủ về [prerequisite, Tauri, Core và kiểm thử](docs/getting-started.md) nằm trong tài liệu setup.

```powershell
git clone https://github.com/meiiie/neko-aurora.git
Set-Location neko-bot
New-Item -ItemType Directory -Force app/.build | Out-Null
Push-Location app/native
try {
    go build -o ../.build/neko-aurora.exe ./cmd/neko-aurora
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
} finally { Pop-Location }
cargo build --locked --manifest-path app/host/Cargo.toml --target-dir app/.build/host-target
if ($LASTEXITCODE -ne 0) { throw 'Host build failed' }
Copy-Item app/.build/host-target/debug/neko-host.exe app/.build/neko-host.exe
./app/.build/neko-aurora.exe --state ./app/.state/local --listen 127.0.0.1:8765
```

Mở `http://127.0.0.1:8765`; Ctrl+C dừng service. Khi chưa cấu hình Core được chấp thuận, tin nhắn được lưu và chờ runtime, không tự gọi provider. Không đưa state hoặc credentials vào Git. Đây là đường chạy local qua trình duyệt; [Tauri dùng launcher/receipt riêng](docs/getting-started.md#tauri-desktop).

## Wiii integration

`wiii/` chứa đúng 61 file overlay từ checkpoint `d2e8284b34d2431e21e4ca8df41c82b3270d4a50`, gồm bridge và bounded-history repair. Nó thiếu phần còn lại của Wiii, nên không có lệnh install/build standalone hợp lệ ngay trong thư mục này.

Base là bản Wiii được khôi phục byte-bound từ hai archive source, có metadata gốc ghi commit `6b1c30b15b4c67e3c841ae870e6989577fc228f8`. Đây không phải lời đảm bảo rằng một checkout mới của commit đó hoặc nhánh Wiii hiện hành khớp toàn bộ archive working tree. Hash hai archive và phạm vi được ghi trong [Wiii integration manifest](migration/wiii-integration.json); archive không được phân phối ở đây. Cần kiểm tra base trong thư mục riêng trước khi ghép overlay. **Không ghi đè một checkout Wiii đang dùng.**

## Roadmap

- [x] Nền tảng bot/kênh/project và lịch sử local có hợp đồng theo scope.
- [x] Journal, receipt/idempotency và các luồng phục hồi đã có kiểm thử local.
- [ ] Hoàn thiện saved-account witness, Core bootstrap và nghiệm thu provider thật.
- [ ] Pairing thiết bị và đồng bộ cloud nhiều máy với quyền rõ ràng.
- [ ] Nghiệm thu điện thoại thật, bàn phím, safe area và AT speech.
- [ ] Đóng gói, ký và nghiệm thu một bản phát hành dành cho người dùng.

Các mục đã đánh dấu chỉ nói về nền tảng local trong phạm vi kiểm thử đã ghi; chúng không biến snapshot này thành production. Phần Wiii vẫn cần base tương ứng để ghép overlay.

## Hỗ trợ và đóng góp

Windows local là luồng development có bằng chứng được ghi; macOS/Linux và thiết bị di động chưa được nghiệm thu cho toàn ứng dụng. Xem [ma trận hỗ trợ và phạm vi kiểm chứng](docs/project-status.md).

- [CONTRIBUTING](CONTRIBUTING.md): chọn phần code, chuẩn bị thay đổi và ghi bằng chứng kiểm thử.
- [SUPPORT](SUPPORT.md): báo bug, lỗi setup hoặc đề xuất qua GitHub Issues.
- [SECURITY](SECURITY.md): quy tắc xử lý lỗ hổng và dữ liệu nhạy cảm; không đăng secrets vào issue công khai.
- [Roadmap chi tiết](docs/ROADMAP.md): thứ tự phụ thuộc và gate hoàn thành; chưa có lịch/SLA được hứa.

Repo hiện không có CI tự động ở root. Badge ở đầu trang là nhãn trạng thái/kiến trúc tĩnh, không phải kết quả CI.

## Các nhánh

| Nhánh | Mục đích |
|---|---|
| `main` | Snapshot phát triển tích hợp: app `90bdcef`, Core `93d6c01` và Wiii overlay `d2e8284`; không phải stable release |
| `wip/saved-account-witness` | Giữ 5 file witness chưa hoàn tất. Windows Issue test hợp lệ đầu tiên từng FAIL; Core bootstrap/CLI wiring còn thiếu |
| `wip/main-local-scripts` | Giữ 3 script đang sửa trên baseline app `7f80d55`; chưa nghiệm thu lại |
| `archive/ui-integration-f6` | Snapshot UI cũ phục vụ đối chiếu; không chọn làm runtime hiện hành |

Các nhánh WIP/archive có thể khác `main` cả code lẫn tài liệu. Provenance từng file nằm trong `migration/source-manifest.json`; không nhầm commit của snapshot gom source với commit gốc hoặc một GitHub Release. Workflow upstream được giữ dưới `migration/disabled-workflows/`, không tự triển khai khi push repo này.

## Attribution và giấy phép

Repository này không tạo một giấy phép chung mới cho mọi component.

- Neko Core giữ [LICENSE](core/LICENSE), [NOTICE](core/NOTICE) và [license map](core/LICENSING.md): original core material là AGPL-3.0-only, còn ranh giới `sdk/` và material bên ngoài theo notice tương ứng. Không suy rộng Apache-2.0 từ SDK sang toàn Core/app.
- Wiii overlay giữ [LICENSE](wiii/LICENSE), [license map](wiii/LICENSING.md) và [trademark notice](wiii/TRADEMARKS.md).
- Neko Peek artwork giữ AGPL-3.0 provenance; Geist giữ SIL OFL1.1. [Asset provenance và license](app/native/internal/web/assets/brand/neko-peek/PROVENANCE.md) đi kèm source và notice gốc.
- `app/` chưa có LICENSE tổng thể. Việc công khai repository không tự cấp một giấy phép mới cho các file chưa có grant tương ứng. Giữ nguyên attribution và kiểm tra phạm vi từng file trước khi tái sử dụng/phân phối.

Grok bundle chỉ được dùng làm reference UX đọc; bundle, source/assets không được phép phân phối và archive tham chiếu không nằm trong repo này.
