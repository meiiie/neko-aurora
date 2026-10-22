# Trạng thái và hỗ trợ

[← Tài liệu](README.md) · [Roadmap](ROADMAP.md)

Neko Aurora là development snapshot. Các binary và capture cũ không được phân phối cùng repo. Commit snapshot gom source khác commit nguồn; dùng `migration/source-manifest.json` để đối chiếu provenance.

| Bề mặt | Trạng thái | Chưa được suy ra |
|---|---|---|
| Windows local web + Go/Rust | Source/build path và bằng chứng local theo commit đã ghi | Installer stable, production soak hoặc mọi cấu hình Windows |
| Tauri desktop | Có shell và verified-instance launch contract | Chữ ký/phát hành, hoặc mọi GPU/WebView/thiết bị |
| Web UI tại viewport mobile | Có kiểm thử browser/geometry/focus theo scope | iOS/Android native, bàn phím thật, safe area thật, AT speech |
| macOS/Linux cho toàn app | Chưa nghiệm thu trong snapshot này | Không suy từ Tauri hoặc Go hỗ trợ OS sang app đã support |
| Core runtime | Có source/runtime adapters và controlled loopback evidence | Chất lượng/cost/provider thật hoặc saved-account witness đã hoàn tất |
| Wiii | 61 file overlay, cần đúng base archive | Full Wiii build hoặc tự động tương thích Wiii hiện hành |
| Cloud/multi-device | Roadmap | SSE/replica local không phải cloud sync |

## Cách đọc một kết quả kiểm tra

PASS chỉ áp dụng cho exact commit, command, môi trường và scope được ghi. FAIL giữ nguyên failure để tái hiện. BLOCKED nói gate chưa có bằng chứng, không phải PASS. Go build/CLI help hoặc Cargo metadata không thay cho full native/Core end-to-end. Ảnh source `2a74029` trong README là fixture web UI cũ đã xem pixels, không chứng minh checkout hiện tại hoặc app điện thoại thật.

## CI và release

Repo hiện không có CI tự động tại root; workflow Core gốc được giữ trong `migration/disabled-workflows/` để đối chiếu. Badge chỉ là nhãn tĩnh. Chưa có installer stable, release signature, support window hay SLA được cam kết. Không tự bật workflow có provider/cloud/host side effect chỉ để có badge xanh.

Tài liệu repo có thể được hoàn thiện trước khi sản phẩm hoàn tất. Xem [hỗ trợ](../SUPPORT.md) để báo vấn đề với fixture không nhạy cảm.
