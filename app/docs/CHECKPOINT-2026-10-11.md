# Checkpoint Neko Bot - 2026-10-11

Bản ghi lịch sử của lát cắt intake đầu tiên. Trạng thái hiện tại và các kiểm thử Core/DPAPI/Tauri nằm ở [checkpoint Core Windows](CHECKPOINT-2026-10-11-CORE-WINDOWS.md).

Code được kiểm chứng: 51bd0f51c5bd4d0d768723e6cf5ea901cb47dd8e.
Mốc baseline: 92b095d2dac4045b0451b5cb5acc419d2e3feb60.
Đây là clean-start từ thư mục Windows trống, chưa phải sản phẩm hoàn chỉnh.

## Kết quả code

Go nhận và ghi bền vững message/request/event trong một transaction journal có hash-chain, khóa độc quyền writer và fsync trước khi ACK. Cùng bot/nonce/nội dung trả cùng request; nonce khác nội dung bị từ chối. Foreground và background có request/message riêng. Cancel chỉ tác động yêu cầu được chỉ định và tăng epoch. Restart phát lại lịch sử và bỏ đuôi ghi dở chưa ACK; corruption có newline bị từ chối.

UI gốc có Meiiie và Davin, lịch sử theo bot, gửi foreground/background, trạng thái chờ Core, SSE reconnect, bố cục adaptive, textContent cho transcript, keyboard và reduced motion. Go API chỉ bind loopback và kiểm session/origin/host/payload. Đây là local-only một owner; không phải auth hoặc sync đa thiết bị production.

Rust có parser bounded, canonical JSON/SHA-256 cùng fixture Go và kiểm envelope identity/capability/epoch/fence/command conflict. Đây là giao thức native đề xuất, không phải adapter ACP đã tích hợp Core. CommandGate chỉ là preflight trong RAM; chưa có dispatch/effect runtime.

Tauri build Windows và smoke-start process đã pass. Shell mở UI loopback, không có custom IPC hoặc native capability. Native window interaction chưa được nghiệm thu.

## Bằng chứng đúng code commit

| Kiểm tra | Trạng thái | Phạm vi |
| --- | --- | --- |
| go test -count=1 -v ./... | PASS | 13 test cấp cao, gồm 11 shared canonical cases |
| go vet ./... và go build | PASS | Go 1.27.2 Windows amd64 |
| cargo test --offline --locked | PASS | 7 Rust conformance tests |
| cargo fmt --check | PASS | Host crate |
| e2e_local.py --commit 51bd0f5... | PASS | Hai client local, kill/restart, nonce/cursor/cancel |
| ui_smoke.mjs | PASS | 7 checks trên Chrome headless; desktop và 390px |
| cargo build --offline --locked | PASS | Tauri 2.11.1 Windows debug build |
| Tauri process startup | PASS | Process còn sống sau 3 giây; không khẳng định nội dung window |
| Core agent execution và task artifact | CHƯA TRIỂN KHAI | Yêu cầu ở waiting_host; không model giả thay Core |
| Điện thoại thật / cloud sync / provider thật | CHƯA NGHIỆM THU | Không có deployment hoặc provider call |

Logs ở docs/evidence/2026-10-11. Screenshot fixture nội bộ ở .build/evidence, không đưa vào Library. Công cụ CUA launcher lỗi 0x80070003 sau reset/retry; kiểm UI web đã được thực hiện bằng Chrome headless với profile riêng, không chạm browser người dùng.

Go binary SHA-256: d8c111618a59a14d0154b13d61e323c1c6059b089caeaf8d4482bbdd53af42c1.
Tauri binary SHA-256: 4227cca8324cf8a3814b466549c991374cc3fa7b8399472d29ad7bffaff94544.
Node 25.9.0; Rust/Cargo 1.97.1; Git 2.47.1.windows.2. Native binary ở .build; không nằm trong Git.

## Inputs đã bảo toàn

Cả 5 file Library đã được cài trực tiếp vào filesystem Windows bằng helper chính thức không sửa, chạy qua Ubuntu WSL có sẵn. Metadata Library đã được đọc kiểm tra trên tệp cuối; Windows đọc được nội dung. Lỗi os.setxattr của native Python đã được giải quyết bằng route này.

ZIP Grok: d31f6a0342693803510de086fae71a619ae07d20da9ea965e32d061c26f832e5. CRC pass; 654 file trong SHA256SUMS.txt đều khớp. 15 ZIP cũ giữ nguyên dưới reference/archive/original, hash và CRC đều pass. Source App c9dbbd7 có 2.199 file và Core 3d5725f có 772 file khớp toàn bộ manifest. Xem reference/inventory.json.

Chỉ đọc phân tích và các lát code client liên quan; không chạy Grok hoặc import code/assets. License/NOTICE gốc giữ nguyên. Các source App/Core đã khôi phục là reference riêng, chưa tích hợp vào app mới; không chuyển test lịch sử thành kết quả hiện tại. Bản App d61a45c và native changes cũ vẫn chưa tìm lại được.

## Phần tiếp theo

Nối Rust với ACP initialize/session receipt thật của Core 3d5725f, pin executable/profile/root và môi trường, rồi chạy provider fixture xác định qua Core thực. Đích là foreground trả lời trước background hoàn tất, mục tiêu background còn nguyên, tệp thật có hash và receipt bền vững, crash không tạo effect lần hai. Hợp đồng/raw schema đã ghi ở docs/next-core-adapter.md.

Chưa có secure account cards, project/delegation/context ACL, PostgreSQL, durable command/operation receipts, Core checkpoint restore hoặc cloud/device executor. Không gọi phần intake hiện tại là hoàn tất stage 1. Provider thật, deployment, credentials và grants vẫn hoãn theo yêu cầu.

## Phục hồi checkpoint

Private Git bundle được lưu ở .checkpoints cùng manifest SHA-256, và được clone/fsck trong thư mục task sạch trước khi bàn giao. Bundle chứa code/fixtures/docs; không chứa ZIP, credential, .state hoặc build dependencies. Các ZIP reference phải giữ riêng. Restore code không khôi phục dữ liệu sản phẩm. Build lại cần Go, Rust và dependency cache hoặc nguồn dependency chính thức.
