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

## Cập nhật cùng ngày (vòng 2): HỢP THỨC HOÁ TOÀN CHUỖI BẰNG HARNESS — AI THẬT ĐÃ TRẢ LỜI

Dựng `app/.build/bridge-harness.mjs` tái hiện nguyên trạng chuỗi host→Core (env DPAPI-unseal đúng entropy `neko-profile-revision-v1\0owner\0C:\Users\Admin`, `acp --host-profile nekobot`, JSON-RPC initialize/session-new/session-prompt, MCP in-band tối giản đủ 2 tool bắt buộc `capabilities` + `execute_tool`, echo `neko.task` + `neko.execution` đúng `matchesTaskEcho`). Kết quả:

```
[prompt] done in 7340 ms: {"stopReason":"end_turn",...}
=== TRẢ LỜI AI ===
Tôi là Neko Core, một trợ lý lập trình (coding agent) hoạt động ngay trong terminal,
do The Wiii Lab tại Việt Nam tạo ra, sẵn sàng đọc code, sửa lỗi, chạy lệnh và
xây dựng dự án cùng bạn.
```

Đây là lời gọi provider THẬT qua profile `zai` (glm-5.3-flash, api.z.ai) trong bridge profile v2 đầy đủ (budget `requests:1` tiêu đúng 1).

## Ba nguyên nhân đã tìm và sửa trên đường tới đó

1. **Core bundled thiếu fallback `NEKO_PROFILE`** (bản private 973a3bd có, bản bundled trong repo chưa): `loadProfileBridge` chỉ nhận `options.profile` từ `savedProfile ?? args.profile` trong khi NekoBot host chọn profile qua env → session/new fail "Profile auth bridge unavailable" NGAY TỪ ĐẦU, Core nuốt lỗi thành assistant rỗng. Vá: fallback `process.env.NEKO_PROFILE` tại `loadProfileBridge` (core/src/adapters/profile-bridge.ts).
2. **Pins bỏ hash cũ làm gãy replay journal**: event `core_profile.inspection_approved` seq=4 pin runtime `dbf776…`; khi thay hash hiện hành mà không giữ entry historical, `validReplay` từ chối → "journal_corrupt" khiến service không khởi động lại được sau kill. Đã thêm `windows_x64_previous_local_bridge_1_8_0` (historicalReplayOnly). Bài học: **mỗi lần đổi hash build phải append historical, không thay thế**.
3. **Scanner stdout của Go manager giới hạn 64KB/dòng** (`bufio.Scanner` 65536) trong khi session/update kèm context manifest có thể lớn — nâng lên 8MB (`internal/runtime/manager.go`).

Kèm theo: Rust host giờ ghi stderr của Core vào `<task-root>/.core-stderr.log` (từ `Stdio::null()`) để quan sát được; harness giữ lại làm công cụ tái hiện.

## Còn lại (chính xác, đã thu hẹp)

Qua app thật (có context-admission) turn vẫn kết thúc ~10s với assistant rỗng → `recovery_required`, DÙ harness (không context-admission) trả lời trọn vẹn. Delta còn lại nằm ở vòng đời turn trên nhánh `--context-memory --context-admission` giữa Rust host và Core (hợp đồng echo `neko.contextAdmission` trong `matchesTaskEcho`). Bước kế tiếp: bật log hai chiều host↔Core (file `.host-core-io.log` tương tự stderr log) và so với harness; hoặc chạy app với pins `contextProtocol: 0` tạm để xác nhận delta rồi mới sâu vào context admission.

## Gate đã chạy

| Lệnh | Kết quả |
|---|---|
| `go test -count=1 ./internal/protocol` (sau đổi pins) | PASS |
| `neko-host.exe --executor-probe …` (sau rebuild + supervisor đúng) | PASS — descriptor đầy đủ, coreSha khớp build |
| Executor registry sau reset | `"ready": true` |
| UI journey (trình duyệt thật, trình tự ở trên) | PASS tới bước gửi tin; request provider đã tiêu |
| Gọi API z.ai trực tiếp bằng key zai | HTTP 200, text trả về đủ |

Không đọc/in giá trị key; mọi thao tác trên state dev local (`app/.state/local`), không commit state/binary.
