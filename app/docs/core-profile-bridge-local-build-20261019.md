# Core profile bridge — chạy với Core build local, 2026-10-19

Mục tiêu vòng này: đưa Neko Bot chạy **AI thật** qua Core profile bridge dùng credential home `~/.neko-core` đã có trên máy (profile `zai` anthropic glm-5.3-flash). Kết quả: **toàn bộ pipeline đã chạy đến mức gọi provider thật**; chặng cuối (ghi nhận câu trả lời vào journal) còn một lỗi đang định vị chính xác bên dưới.

## Những gì đã làm (runbook tái lập được)

1. **Build Core tương thích từ cây core/ bundled trong repo** (bun 1.4.3 portable, `bun install` + `bun scripts/build.ts`): `app/.build/bridge-neko.exe`, SHA-256 `dbf7764a3d76ffd23bee9cc25fa830c5ab224335c2c6bb27b30eefccbc464f08`. Binary này **nhận diện `acp --host-profile nekobot`** (binary công khai 1.6.2/1.8.0 từ release chỉ biết `nekocut`).
2. **Build writer helper**: `cargo build --release --locked` tại `core/native/writer-lease` → `b9698fbece2801ed6cb5ae461ce19f670d24a59e4225ca67acc0e48cda23e5cf`, đặt cạnh bridge-neko.exe (host yêu cầu cùng thư mục theo pin `writerHelper`).
3. **Cập nhật pins** `app/native/internal/protocol/core-runtimes-v1.json`: đưa build local lên mục `windows_x64` hiện hành (giữ nguyên `profileBridgeVersion: 2`, provenance ghi rõ `bunVersion 1.4.3`, sourceRevision = commit cây core/ `7ff2110f459e30033b1643d09c4d5615f3798c89`); chuyển pin private cũ (`356247…`/`f9f404a…`) sang mục `windows_x64_previous_private_1_8_0` với `historicalReplayOnly: true`. **Rebuild cả neko-host.exe** vì host nhúng cùng file pins bằng `include_str!`.
4. **Supervisor đúng**: `--supervisor` phải là bản sao byte của `app/scripts/core_supervisor.py` (không phải writer-lease). Đã copy sang `app/.build/core_supervisor.py`.
5. **Executor registry reset** khi đổi hash host: xóa `app/.state/local/executors/` (pin cũ khóa descriptor). Sau restart: `"ready": true`.
6. **Khởi động**: `./app/.build/neko-control.exe --state ./app/.state/local --listen 127.0.0.1:8766 --core-profile-config ./app/.build/core-profiles.json` với sources `[{id:"owner", kind:"owner_home"}]`.
7. **UI journey chạy thật** (trình duyệt): Kết nối → "Đã phát hiện Neko Core" → Cho phép xem profile → catalog **30 profile thật** từ `~/.neko-core` → chọn `zai` → duyệt phạm vi (glm-5.3-flash · api.z.ai · 64000 byte / 1024 token / 1 request / 1 bước / 120s) → gửi tin.

## Bằng chứng pipeline đã chạm provider thật

- Ledger: `work.accepted → claimed → runtime_bound → admitted(running)`.
- Task dir `app/.state/local/tasks/req-1668fc62…/` có `.core-home` isolate; tiến trình `bridge-neko.exe` + `neko-host.exe` chạy thật.
- `.core-home/.neko-core/profile-bridge-budget-v1.json`: `"requests":1` — **một request provider thật đã được tiêu**.
- Session Core: context manifest → user message → assistant **rỗng** → capabilities → stop (9 giây), stop receipt v1 hợp lệ.

## Lỗi đang mở (chặng cuối)

Assistant message rỗng ⇒ không có event `text`/`completed` ⇒ `work.runtime_stopped → recovery_required → suspended`. Dữ liệu tách lỗi:

- Gọi **trực tiếp** `https://api.z.ai/api/anthropic/v1/messages` bằng key profile `zai`: HTTP 200, `content: [thinking, text]`, `stop_reason: end_turn`, **32 giây** cho câu ngắn (glm-5.3-flash là model reasoning — trả block `thinking` trước).
- Session bridge chỉ sống ~9 giây rồi stop sạch (không phải deadline 120s của `session/prompt` trong host; không phải budget 120s).

Giả thuyết cần kiểm tiếp (vòng sau): (a) adapter anthropic của Core khi nhận chỉ `thinking` (text chưa kịp tới) xử lý thế nào — đặc biệt khi response bị cắt sớm vì lý do mạng/env của bridge; (b) env capsule của bridge có thiếu biến TLS/proxy nào khiến request trong task-home fail nhanh rồi Core kết thúc sạch; (c) thử profile `openai_compat` (`bai`) để loại trừ nhánh anthropic. Cần bật thêm stderr/log của Core trong task-home khi tái hiện (mặc định bị tắt vì privacy).

## Gate đã chạy

| Lệnh | Kết quả |
|---|---|
| `go test -count=1 ./internal/protocol` (sau đổi pins) | PASS |
| `neko-host.exe --executor-probe …` (sau rebuild + supervisor đúng) | PASS — descriptor đầy đủ, coreSha khớp build |
| Executor registry sau reset | `"ready": true` |
| UI journey (trình duyệt thật, trình tự ở trên) | PASS tới bước gửi tin; request provider đã tiêu |
| Gọi API z.ai trực tiếp bằng key zai | HTTP 200, text trả về đủ |

Không đọc/in giá trị key; mọi thao tác trên state dev local (`app/.state/local`), không commit state/binary.
