# Bot profile appearance (avatar + color) — local slice 2026-10-19

Thay đổi: mở rộng `BotProfile` bằng trường tùy chọn `avatar` (≤16 byte, không ký tự điều khiển) và `color` (`#RRGGBB` hoặc rỗng). Con trỏ null trong input giữ nguyên giá trị hiện tại, nên trình ghi cũ không biết appearance không bao giờ xóa nó. Appearance đi qua đúng chu trình hiện có: CAS `expectedRevision`, `bot.profile_updated` event, replay, snapshot, frozen pin trong work intake, và receipt nonce.

Phạm vi: REQ-PB 003 "Avatar/color editor" — phần editor local. Không phải: memory-off/forget, destructive delete, mọi nghiệm thu provider/thiết bị thật.

## Bằng chứng manual (môi trường: Windows 10 x64, Go 1.27.2, Node v24.22.0, Python 3.10)

| Lệnh (từ `app/native`) | Kết quả |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./...` | PASS — 15/15 package ok (ledger 53s; toàn bộ test cũ giữ nguyên, không nới assertion) |
| `go test -count=1 ./internal/ledger -run Appearance -v` | PASS — 2 test mới: edit/persist/replay + validation |
| `go test -count=1 ./internal/api -run Appearance -v` | PASS — endpoint: lưu, pin vào intake, duplicate nonce, nil-keep, 400/409 validation |
| `node --check internal/web/assets/app.js` | PASS (chỉ cú pháp) |
| `python tools/check_docs.py` | PASS — 12 documents, 92 links, errors=0 |

Test mới:
- `internal/ledger/bot_profile_appearance_test.go` — chỉnh sửa/clear/giữ-giá-trị/đổi-màu-riêng/no-op conflict, duplicate nonce, replay từ đĩa khôi phục appearance.
- `internal/api/bot_profile_appearance_test.go` — POST `/v1/bots/{id}/profile` với `avatar`/`color`: persist, pin `botProfilePin` vào intake mới, duplicate nonce trả profile hiện tại, sửa instructions-only giữ appearance, sai định dạng màu/avatar/trường lạ/sai CAS → 400/409.

UI: dialog "Hướng dẫn cho bot" thêm ô avatar (text ≤16) và color picker + nút về màu mặc định; sidebar bot avatar ưu tiên `profile.avatar`/`profile.color`, fallback chữ cái đầu và `bot.color`.

## Gate chưa chạy

- Fixture provider / model thật: không liên quan slice này và không chạy.
- Secret scan diff: chỉ chứa nonce test không nhạy cảm và mã màu; không có giá trị bí mật.

## Gate browser interaction — ĐÃ CHẠY và PASS (cùng ngày, sau khi sửa 2 lỗi tích hợp)

Môi trường: trình duyệt thật điều khiển qua automation tới `http://127.0.0.1:8766` (service local, state riêng), viewport 1280×800, session owner thường (không fixture).

Kịch bản đã chạy: mở sidebar → menu "Loại yêu cầu và tệp đính kèm" → "Hướng dẫn & lịch sử của bot" → dialog hiện đủ ô Hình đại diện / Màu nhận diện / nút về màu mặc định → điền `🎯`, `#22c55e`, hướng dẫn mới → "Lưu cho yêu cầu mới" → dialog tự đóng, sidebar avatar cập nhật live (`🎯`, nền `rgb(34,197,94)`), metadata journal sạch ("0 thay đổi metadata chờ xác nhận"), không thông báo lỗi.

Gate này bắt được 2 lỗi tích hợp mà test Go/`node --check` không phát hiện, đã sửa trong cùng slice:

1. `metadataInput` (app.js) dùng `strictKeys` không cho `avatar`/`color` → body gửi qua đường durable metadata journal bị từ chối normalize. Sửa: cho phép + validate đúng ràng buộc (≤16 byte, không ký tự điều khiển, màu `#RRGGBB` hoặc rỗng).
2. Receipt `bot_profile` trả appearance dưới con trỏ nil (`"avatar":null`) trong khi bản chờ phía UI luôn mang chuỗi tường minh → `canonicalJSON` đối soát lệch. Sửa: receipt echo appearance **đã resolve** dưới dạng chuỗi tường minh; hai test hợp đồng (`metadata_receipts_test.go`, `review_metadata_receipts_probe_test.go`) cập nhật kỳ vọng sang biến thể resolved — hợp đồng mới vẫn khóa "original typed input" cho mọi trường cũ.

Sau sửa: `go vet ./...` PASS, `go test -count=1 ./...` 15/15 PASS, `node --check app.js` PASS, tái build `neko-control.exe` và chạy lại toàn bộ kịch bản trên = PASS.

## Ghi chú hợp đồng

- Receipt (`CommandReceipt` cho `bot_profile`) vẫn trả input với con trỏ appearance null — giữ nguyên hợp đồng "original typed input" mà `metadata_receipts_test.go` khóa lại; appearance đã giải resolve nằm trong event profile.
- Sự kiện cũ (trước slice) replay với avatar/color rỗng và qua validation; không cần migration.
