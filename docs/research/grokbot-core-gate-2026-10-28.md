# Neko Aurora — gate Neko Core trên Windows, 28/10/2026

Code commit: `bef6a7c508168a6335fe5ecdc0760c281f370b46`, phát triển từ `d453259b17038ca9281fd01e74495cbb9e076b33`. Đây là checkpoint cục bộ phục vụ backlog Grok Bot; chưa push, deploy hay phát hành. Tự thực hiện theo pstack, không dùng subagent.

## Sửa gì và vì sao

- [Saved account bridge](../../core/src/adapters/saved-account-bridge.ts): thay chuỗi ép kiểu qua `unknown` bằng miền JSON và narrowing cụ thể; giữ schema đóng, binding, deadline, authority và giới hạn ngân sách. Nội dung/reasoning/continuation được che credential; tool call và metadata chứa credential bị từ chối. Thêm regression cho cả câu trả lời cuối và tool call streaming. Lint ban đầu có 29 lỗi ở bridge và test liên quan; sau sửa không tắt rule.
- [Dò tần số màn hình](../../core/src/adapters/display.ts) và [UI lifecycle](../../core/src/ui/chat.tsx): probe PowerShell trước đây dùng PATH, kế thừa cwd của task và tiếp tục sống sau unmount. Trace subprocess xác nhận đây là tiến trình giữ thư mục gây `EBUSY`; Git child đã đóng. Nay executable Windows được giải từ System32, child chạy trong thư mục executable với môi trường tối thiểu, UI hủy probe khi đóng/đổi lựa chọn FPS. Cache gắn với home lúc bắt đầu; kết quả sau hủy không được lưu. Test shutdown từng thất bại độc lập nay qua trong khoảng 0,4 giây, không tăng timeout cleanup của fixture.
- [Resident UIA](../../core/src/core/windows-uia-host.ts): đo riêng hai lần thấy inbox `taskkill /T /F` thành công sau 5,35 và 5,63 giây, vượt giới hạn cũ 5 giây. Đổi giới hạn **bước teardown** thành 10 giây, không đổi deadline request hay ngưỡng phản hồi abort. Vẫn yêu cầu exit code 0 của tree-kill và close của helper trước khi cho phép helper mới. Regression xác nhận teardown không được xác nhận sẽ chặn mọi request thay thế; fixture WPF được đóng trong `finally` dù cleanup host thất bại.
- Test monorepo: provenance dùng Git repository tạm do fixture tạo, giữ nguyên ràng buộc root của launcher; installer/source bootstrap kiểm tra recipe Core đã lưu trong `migration/disabled-workflows/core/`. Không kích hoạt workflow hoặc nới quyền launcher benchmark. Assertion terminal chấp nhận xuống dòng giữa hai từ nhưng giữ kiểm tra nội dung và quyền resume/discard.

## Môi trường và bằng chứng

Windows amd64, PowerShell 7.6, Bun **1.4.0** đúng pin CI, Node **25.9.0** (khác Node 22 trong CI), Python 3.13.7. Child environment lấy theo allowlist, `USERPROFILE`/XDG config và npmrc tách riêng; không chuyển credential/provider environment vào test. Dependencies cài bằng `bun --no-env-file install --frozen-lockfile --ignore-scripts`; lockfile không đổi.

Manifest của toàn bộ source `app/`, `core/`, `tools/` và Rust pin được kiểm tra trước/sau từng lệnh và đối chiếu lại với code commit: **`7be96173cf008aab32c97db92fdffa6dbcef90fa78adcd0d895f2fec85fff764`**. Audit cục bộ (ignored): `.audit/2026-10-28-core-gate/validation.jsonl`, `code-checkpoint.json`; mỗi record có exact command, cwd, base, manifest, thời gian, exit code và PASS/FAIL. Log FAIL được giữ nguyên.

Lệnh Bun dưới đây chạy từ `core/`, bằng binary `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe`, với `--no-env-file`.

| Kiểm tra | Kết quả | Log cuối |
|---|---|---|
| `bun run typecheck` | PASS | `typecheck-145153.log` |
| `bun run lint` | PASS, không warning/error | `lint-145156.log` |
| `bun test --shard=1/4` | 474 pass, 19 skip, 0 fail | `shard-1-145316.log` |
| `bun test --shard=2/4` | 562 pass, 7 skip, 0 fail | `shard-2-145444.log` |
| `bun test --shard=3/4` | 591 pass, 2 skip, 0 fail | `shard-3-145629.log` |
| `bun test --shard=4/4` | 546 pass, 14 skip, 0 fail | `shard-4-145857.log` |
| `node <core>/bin/neko-source.cjs doctor` | Exit 0; WARN model/key chưa đặt, TTY, browser và mode auto không sandbox | `doctor-150115.log` |
| `node <core>/bin/neko-source.cjs policy` | Exit 0; verdict **WARN**, không tuyên bố confinement | `policy-150116.log` |
| `bun run build` | PASS: compile, production UI probe, ConPTY input, ACP initialize, startup lifecycle và 4 tình huống nhập sớm/normal, có/không yolo | `core-build-150118.log` |
| `pwsh -NoProfile -File tools/cargo-windows.ps1 test --locked --offline --manifest-path app/host/Cargo.toml --test saved_key` | 6 pass, 0 fail; có private stdin Rust → Core thật với credential fixture | `rust-saved-key-150211.log` |

Tổng Core: **2.173 pass, 42 skip, 0 fail trên 221 file**, chia bốn process tuần tự đúng cách chạy Windows CI, không bỏ file. Giữ nguyên các marker skip hiện có; 42 skip không được tính là pass. Hai CLI diagnostic chạy từ workspace tạm, `NEKO_BUN_PATH` trỏ binary tin cậy. Rust dùng toolchain 1.97.1, MSVC 14.43.34618, SDK 10.0.22621.0; bổ sung `CARGO_HOME=C:/Users/Admin/.cargo`, `RUSTUP_HOME=C:/Users/Admin/.rustup` để dùng cache/toolchain đã có, chạy offline.

Các regression đích cũng qua: 28 test saved-key/profile/filter; 12 test display/task-session; 11 test UIA gồm cửa sổ WPF thử nghiệm trên máy Windows. Scan trước code checkpoint: 11 candidate file và 137 blob mới reachable so với origin/main, không phát hiện mẫu key/token trong phạm vi scanner. Không coi scanner theo mẫu là phát hiện mọi bí mật.

## Phạm vi còn lại

Các lượt đầu đã FAIL ở lint, assertion xuống dòng, provenance/workflow monorepo, UIA teardown và handle của display probe. Kết quả cuối được lấy từ lượt bốn shard cùng manifest sau khi sửa nguyên nhân, không thay bằng retry im lặng.

Đây là kiểm chứng Core và primitive Windows cục bộ. Không chạy benchmark/model evaluation, không dùng tài khoản/provider thật, không kiểm chứng live SRT, macOS/Linux, installer phát hành, đồng bộ cloud hoặc thiết bị di động thật. Các recipe workflow được kiểm tra vẫn là bản archive, không phải CI đã chạy. Launcher evaluation vẫn giữ ràng buộc canonical repository root; không dùng test fixture như bằng chứng campaign hoạt động trong monorepo.

Gate này không tự hoàn tất thêm dòng changelog Grok: index vẫn có 10 dòng được kiểm chứng một phần, 1 dòng không tiếp nhận, 809 dòng chưa có bằng chứng tương ứng. Tiếp theo là lưu ảnh trước upload, phục hồi sau reload và retry không ghi đè ảnh cũ; kế hoạch chưa phải implementation.
