# Neko Bot — bản chuyển máy 2026-10-16

Bản sao mã nguồn riêng tư từ DESKTOP-PLLO4HG, để tiếp tục công việc trên LAPTOP-LNGFG0JP. Đây là snapshot chuyển máy, chưa phải bản phát hành hoàn chỉnh. Không triển khai khi clone.

| Thư mục | Source gốc |
|---|---|
| `app/` | Neko Bot `90bdcefcc21908144d4c390810724876a1a4319b`; production code giống `9c245cce4eaa3f43506a2fbc3af296ffe21009b6` |
| `core/` | Neko Core `93d6c01fdf500a71b09ecfb32aee6532861248e8`, gồm foundation saved-account bridge; runtime base `973a3bd93466baa4689e01c4050dfc59f5a7f7c4` |
| `wiii/` | Wiii ADE integration `d2e8284b34d2431e21e4ca8df41c82b3270d4a50`; không ghi đè repo Wiii khác |

## Tiếp tục trên laptop

Clone vào thư mục MỚI, ví dụ `git clone https://github.com/meiiie/nekobot-transfer-20261016.git NekoBot-transfer-20261016`. Giữ nguyên Wiii và các workspace hiện có trên laptop. Đọc `app/AGENTS.md`, tài liệu định hướng/release của app và `core/AGENTS.md` trước khi sửa. Đọc manifest và các nhánh WIP trước khi chọn source để tiếp tục. Cài dependency trong từng component bằng lockfile của nó; không sao chép profile/credentials từ desktop.

Mã nguồn dùng Tauri UI, Rust trusted host và Go control plane; Neko Core sở hữu agent loop. `wiii/` là ADE bridge riêng. Thư mục `wiii/` là đúng 61 file overlay liên quan, chưa phải toàn bộ repo Wiii; base được lưu trong hai ZIP Wiii tại desktop, metadata gốc ghi commit `6b1c30b15b4c67e3c841ae870e6989577fc228f8`. Xem `migration/wiii-integration.json` trước khi áp vào một bản Wiii riêng. Runtime state, account store, chat thật, binary, cache và dependency chưa đi kèm. Các đường dẫn tuyệt đối trong tài liệu/runners lịch sử cần cấu hình lại trên máy mới; chúng không chứng minh file có trên laptop.

## Phạm vi và phần còn dở

- Main gốc `E:/Sach/Sua/NekoBot` còn ở `7f80d55948d4e9847fdbe68a9557e29c88025014`. Không tìm thấy receipt import `90bdcef`; không import lại hay sửa Main trong lượt backup.
- Nhánh `wip/saved-account-witness` giữ 5 file Go chưa commit của witness. Bản dở đã compile, nhưng Windows Issue test hợp lệ đầu tiên từng FAIL (`saved account authority unavailable`). Core bootstrap/CLI wiring chưa hoàn tất. Không gọi đây là đường provider đã hoạt động.
- Nhánh `wip/main-local-scripts` giữ 3 script Main đang sửa, dựa trên đúng baseline Main gốc. Chưa nghiệm thu lại trong lượt backup.
- Nhánh `archive/ui-integration-f6` giữ snapshot code nhánh UI cũ chưa nằm trong ancestry `90bdcef`; đây là tham khảo lịch sử, không phải ứng viên hiện hành.
- Các gate native37 trước đây thuộc evidence lịch sử scoped của `9c245cc/90bdcef`, không được chạy lại cho việc sao lưu này. Chưa nghiệm thu phone/AT speech, model/provider thật hay cloud. Không tạo release/tag/deployment trong lượt backup.

Lịch sử Git gốc, 17 ZIP, Grok/reference, ảnh/video capture, journal/account homes và các repository cũ được giữ nguyên trên desktop. Chúng không được đưa vào lịch sử snapshot mới. `migration/source-manifest.json` ghi hash từng source blob, file được xuất và file loại cùng lý do. Workflow gốc được giữ tại `migration/disabled-workflows/`, không có workflow tự chạy khi push.

## Giấy phép

Giữ nguyên các LICENSE/NOTICE và provenance theo component. Core có AGPL-3.0-only, ranh giới `sdk/` Apache-2.0 và các ngoại lệ được ghi trong `core/LICENSING.md`. Wiii giữ LICENSE gốc. Neko Peek artwork giữ AGPL và Geist giữ OFL như trong `app/native/internal/web/assets/`. App chưa có LICENSE tổng thể tại root snapshot; bản chuyển máy không tự cấp giấy phép hoặc đổi giấy phép. Không sao chép Grok bundle/code/assets vào bản này.
