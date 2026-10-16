# Neko Bot: kết quả ba lát cắt local — 2026-10-13

Đã triển khai code cho ba luồng người dùng, dùng bốn đồng nghiệp với scope riêng và Root giữ quyền Git/tích hợp. `coordination-20261013.md` ghi owner, acceptance và kết quả tiếp nhận.

- Kết nối tự đọc mô tả nguồn Core an toàn khi đúng local owner/capability. Một nguồn tự chọn; nhiều nguồn giữ lựa chọn trống nếu chưa có lựa chọn hợp lệ. Profile inspection và execution consent vẫn cần hành động rõ; preload không đọc profile hoặc tạo grant.
- Nhóm mới giữ tối đa sáu published-report pins phân biệt đúng authority, dành hai slots cho kết quả các bước trước. Tám publication lặp không còn làm mất peer result. Nhóm Version1 và tin đơn giữ policy/pins/replay cũ; không thêm agent loop bên ngoài Neko Core.
- Retry giữ hàng tin chờ và trạng thái đang gửi; thao tác không kéo focus vào composer/mobile keyboard. ACK chỉ dọn tin/bản nháp thuộc nonce/body gốc, không xóa draft mới có cùng nội dung, không kéo navigation hoặc tự replay profile-bound request.

## Identity và bằng chứng

Base: `195a0ec990db0478d9f052bc0e79a83b12dc9d78`. Bulk code: `104bdf2e34b65f345d2b1ffb6a6014d1a7629b5c`. Final code: `53495231a06aac97c1092029fda35971bdbc08c5`; so với bulk chỉ thay Connections.js, runner onboarding và doc của nó. Source delivery sau đó chỉ thêm/cập nhật tài liệu; SHA exact nằm trong packet manifest. Host/Desktop/Core/supervisor không sửa; Core code vẫn `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`.

Environment: Windows amd64, PowerShell, Go1.27.2, Node25.9.0, Python3.13. Go offline với `GOPROXY=off`, `GOSUMDB=off`, `GOENV=off`, `CGO_ENABLED=0`; runner chỉ đọc environment keys được nêu tên, cache/temp/journal fixture ở E. Không enumerate credential environment hoặc dùng profile/provider thật. Formal `-race` chưa chạy vì không có C compiler.

| Gate | Commit / command | Status / evidence |
| --- | --- | --- |
| Go toàn bộ, vet, build | Clean104: `go test ./... -count=1 -json`; `go vet ./...`; `go build ... ./cmd/neko-control` | PASS509 top-level +510 subtests, zero fail. Hai optional test SKIP: DOCX reference và browser report seed. Receipt/log: `E:/CodexScratch/core-auth-bridge-20261012/root/parallel-product-104bdf2-full` |
| Nhóm policy/peer/authority/replay/byte limit | Bảy `TestGroupContext*` trong full104; worker scoped7/related60 có working hashes riêng | PASS. Runner Root sửa whitelist/UTF-8 trước checkpoint104; worker seals trước đó giữ nguyên, không gọi cùng hash |
| VM bộ ảnh hưởng | Clean104: onboarding21/reconnect28/published16/history19/manual13/file16/metadata17/principal6 +syntax2 | PASS, `E:/CodexScratch/parallel-product-20261013/root-vm-104bdf2` |
| Independent UX104 | Frozen25 + supplemental4, `--commit 104bdf2...` | RED24 PASS/1 FAIL; ownership4 PASS. Multi-source default selection được xác minh rồi sửa; evidence giữ lại |
| Core source chọn rõ | Clean534: `node scripts/ui_core_source_onboarding_local.mjs --source-commit 53495231...`; manual13 +syntax | PASS24 +13. `E:/CodexScratch/parallel-product-20261013/root-vm-5349523` và `core-source-onboarding-20261013/immutable-5349523` |
| API/build cuối | Clean534: `go test ./internal/api -run '^TestCoreProfiles' -count=1 -json`; Go build | PASS3 tests + build. Binary SHA256 `c2732194124fb989dc84b301fb06e910f64579e94f79e930bf57b1b91ffa337d`; VCS534, modified=false. `E:/CodexScratch/parallel-product-20261013/final-build-5349523` |
| Independent final | Nguyên frozen25 + supplemental4, `--commit 53495231...` | PASS25/25 +4/4, zero fail/blocked/harness error. `E:/CodexScratch/ux-independent-20261013/integration-534-{ux,draft-ownership}-v1.json`; không sửa assertions |
| Secret scan trước checkpoint | `python scripts/secret_scan.py <sourceRoot>` | PASS655 text files, zero findings; matched values không in |
| Native/browser/phone | CUA inventory / owned Chrome startup trước đợt song song | BLOCKED trước renderer: kernel0x80070003; Chrome chưa chứng minh profile cách ly/DevTools. Không retry hoặc dùng đường vòng; không có ảnh mới |

Các chứng cứ RED gốc, harness startup/correction failures và historical receipts được giữ nguyên. Legacy `readonly_restore_ui_test.mjs` thiếu capabilities trong adapter, fail trước UI assertions; legacy report extraction có session/helper gaps được giữ riêng. Không dùng các harness này làm bằng chứng PASS hoặc đổi source để chiều harness. Edit-away rồi trở lại text gốc mà không có pending nonce mới là observation về policy text ownership; không đưa vào counted defect mới.

## Giới hạn và bàn giao

Giới hạn ngữ cảnh8 KB vẫn giữ. Probe 7,416 bytes report +570-byte proposal tạo combined8,437 bytes, nên stage2 vẫn dừng `needs_owner/stage_context_unavailable`. Fix này giải quyết slot exhaustion khi còn byte budget; không bảo đảm mọi nhóm hoàn tất. Không truncate, rebind authority hoặc nới budget.

Nhóm mới ghi nested published-report context Version2. Control-plane binary cũ không replay được journal mới đó. Không hạ binary trên state mới; rollback dùng binary/journal cũ riêng. Main source nhập bằng FF immutable, receipt kiểm17 ZIP/1,509 protected files, installed Core hash và ba dirty script gốc. Việc cập nhật source không thay live service8765 hay state thật. Packet `E:/CodexScratch/parallel-product-20261013/delivery-5349523` có source patch/bundle và preservation receipt cuối.

Bản Desktop riêng cùng đợt dùng binary Go534 và các Desktop/Rust host/Core/helper đã pin của candidate c91e4de. Launcher CheckOnly không mở cửa sổ hay service; start bằng `Start-Neko-Desktop.cmd`, port8767/state riêng và từ chối port đã có chủ. Candidate mới và exact manifest path nằm trong packet summary. Người dùng có thể chỉ xem Kết nối/Tab/Escape trước khi quyết định consent. Cửa sổ chưa được nghiệm thu, provider chưa được gọi.

Còn gate: native open/close/focus/liveness/profile isolation, phone/IME/safe-area/AT speech, real model/account/OAuth renewal, Wiii bridge-v2, remote device/cloud và group Core execution với ngữ cảnh v2. VM/local journal không thay các gate này. Không claim toàn sản phẩm hoàn tất; không deploy, push, gửi secrets hoặc tạo grants thật.
