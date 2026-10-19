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

- Kiểm tra interaction/focus/viewport trên trình duyệt cho dialog mới: CHƯA CHẠY trong môi trường này (chỉ `node --check`). Cần lần chạy Chrome headless/fixture theo quy trình hiện có trước khi tính là PASS UI.
- Fixture provider / model thật: không liên quan slice này và không chạy.
- Secret scan diff: chỉ chứa nonce test không nhạy cảm và mã màu; không có giá trị bí mật.

## Ghi chú hợp đồng

- Receipt (`CommandReceipt` cho `bot_profile`) vẫn trả input với con trỏ appearance null — giữ nguyên hợp đồng "original typed input" mà `metadata_receipts_test.go` khóa lại; appearance đã giải resolve nằm trong event profile.
- Sự kiện cũ (trước slice) replay với avatar/color rỗng và qua validation; không cần migration.
