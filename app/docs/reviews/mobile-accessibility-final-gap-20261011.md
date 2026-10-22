# Neko Aurora — bounded final mobile/accessibility gap check

Audit độc lập ngày 2026-10-11 trên UI commit **`12ef2f5078d40b483c9d3e67489f51845a3a9f9c`**. Kết quả chính: **11 PASS / 2 FAIL / 0 BLOCKED**; gồm 8 ca hành vi mới (6 pass, 2 fail) và 5 gate nguồn/syntax/startup/exception. Hai nhóm lỗi được xác nhận bằng một lượt riêng: **5 PASS / 2 FAIL / 0 BLOCKED**, cùng hai nhóm, không cộng thành bốn lỗi. Exit code cả hai lượt: `1`, do defect assertion.

Không chạy lại các bộ regression/context/light đầy đủ. Kết quả `28+28+19 PASS` do auditor khác báo là evidence của owner, không phải lượt kiểm tra này. Báo cáo trước ở `mobile-accessibility-light-20261011.md` giữ nguyên lịch sử `2cda67d` và sáu nhóm phát hiện.

## Phạm vi và môi trường

- Windows `win32`, Node `v25.9.0`, Chrome headless `154.0.8037.98`; loopback fixture do auditor sở hữu, profile Chrome riêng trong `%TEMP%`.
- Nguồn đọc bằng `git show <commit>:native/internal/web/assets/<file>` từ `C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f`. Bốn UI assets, ba Geist WOFF2 và hai notice/provenance files đều khớp SHA-256 raw bytes của exact commit. `HEAD` ở hai thời điểm chạy cũng là exact commit này.
- Fixture Node phục vụ font với `font-src 'self'` và MIME `font/woff2`. Không xác nhận routing/CSP/font của Go hoặc native Tauri. Go typography proof tại integration `05e87e4` do UI owner báo là evidence riêng, chưa được auditor này chạy.
- Ca resize dùng CDP metrics, viewport `320x320`, `390x260`, `390x844`, `844x390` và coarse pointer/maxTouchPoints 5. `390x260` là mô phỏng co layout/visual viewport. Chưa kiểm thử bàn phím vật lý, bàn phím OS điện thoại, visualViewport chỉ co chiều cao mà layout giữ nguyên, panning khi nhập, IME, safe-area thật hoặc orientation/lifecycle của thiết bị thật.
- Không chạy Rust/Go/Core/provider, không có durable store hoặc sync nhiều máy trong lượt fixture này. Không tạo grant, dùng credentials, deploy/push, chiếm native window/team browser/mouse hoặc đổi network settings. UI/runtime/Git index không được sửa.

## Hai defect còn mở

**GAP-MA-01 — P2: focus bị mất khi sidebar trở thành inert sau đổi breakpoint.** Exact source: `native/internal/web/assets/app.js:93`, handler `mobile.addEventListener("change",...)`; `closeNavigation` ở dòng 91.

Repro: tại `390x844`, mở drawer, focus ô tìm bot rồi nhập chuỗi không có kết quả; resize sang `844x390`, rồi về `390x844`. Landscape giữ `bot-search`; portrait chuyển focus sang `BODY` khi sidebar đóng/inert. Mở lại drawer vẫn focus đúng ô tìm. Lượt xác nhận riêng tái hiện cả khi danh sách có kết quả. Tab khôi phục về `nav-open`.

Expected: trước khi breakpoint handler ẩn/inert sidebar, ghi nhận focus cũ; nếu control đang được focus sẽ không còn dùng được, chuyển focus tới trigger `nav-open` sau khi layout cập nhật. Giữ chính sách đóng drawer hiện tại. Không đưa focus về trigger nếu người dùng đang ở một control khác còn dùng được.

Evidence gate: `gap_empty_drawer_breakpoint_crossing_focus_and_reopen`; confirmation: `confirm_orientation_focus_loss_with_empty_and_nonempty_roster`. Screenshot: `evidence/gap-12ef/gap-drawer-return.png`; vị trí activeElement được ghi trong JSON vì ảnh đơn thuần không chứng minh focus.

**GAP-MA-02 — P2: disable/hide nút Dừng đang được focus làm focus rơi khỏi work dialog.** Exact source: `native/internal/web/assets/app.js:81` (`cancelWork`), dòng 72 (task cancel disable/hide); conversation cancel cũng được render ở dòng 40, chưa mở rộng kiểm tra riêng.

Repro: seed một work `running`; ở `390x844` mở work dialog; focus nút Dừng trong task bằng bàn phím và Enter; giữ HTTP `/v1/cancel` chưa trả lời. `cancelPending` render làm nút disabled, activeElement thành `BODY` trong khi work dialog còn mở và `main.inert=true`. Terminal SSE rồi HTTP ACK tiếp tục giữ `BODY`.

Lượt xác nhận thêm: chỉ focus nút Dừng, không gửi cancel POST; terminal SSE `completed` tự ẩn nút và cũng làm focus thành `BODY`. Tab khôi phục `work-close`. Sau khi focus đã chuyển tới `work-close`, ACK lỗi giữ focus tại đó; Escape đóng panel và trả về `work-open`.

Expected: trước khi disable/hide/remove một control đang được focus, chuyển tới control còn hiển thị trong cùng panel, ví dụ `work-close`. Cần bao phủ cả `cancelWork` và render do SSE. Dùng guard panel/navigation generation để không kéo focus về panel đã đóng hoặc control cũ sau khi người dùng chuyển nơi khác. Ca rời dialog, nhập draft mới rồi nhận SSE/ACK đang pass và phải tiếp tục được bảo toàn.

Evidence gate: `gap_keyboard_cancel_pending_and_terminal_focus`; confirmation: `confirm_cancel_focus_loss_pending_and_unsolicited_terminal_event`. Screenshot: `evidence/gap-12ef/gap-cancel-terminal.png`.

## Những gap đã pass

| Ca độc lập | Kết quả | Điều đã quan sát |
| --- | --- | --- |
| Mention mở qua resize/orientation | PASS | 14 bot; option thứ 10 giữ `aria-activedescendant`, nằm trong popup và hit-test được qua bốn viewport; draft/focus giữ nguyên; không POST |
| Lane popup mở qua resize/orientation | PASS | Radio đã chọn và focus vẫn dùng được; popup nằm trong viewport; Escape trả focus tới nút mở |
| Mention có focus, co viewport rồi mở rộng | PASS | Option cuối giữ selection và cuộn đúng tại `390x260`; Enter chọn alias sau khi phục hồi; không submit |
| Drawer không có kết quả, resize trong breakpoint mobile | PASS | Search giữ focus, main inert, reverse Tab còn trong drawer; Escape trả focus |
| Cancel ACK/SSE đến sau khi rời panel | PASS | Draft mới và selection `[4,9]` giữ nguyên; focus ở prompt; panel đóng; main hết inert |
| Event cuối đến khi mention/lane đang mở | PASS | Popup, selected option, draft và focus được giữ trong các vị trí còn dùng được |

## Lệnh và evidence chính xác

Từ `C:\Users\Admin\Documents\Codex\2026-10-11\task-5`, cả hai lệnh chạy với quyền sandbox escalation được chấp thuận cho fixture/browser riêng. Không có auto-review rejection.

```powershell
& 'C:\Program Files\nodejs\node.exe' 'C:\Users\Admin\Documents\Codex\2026-10-11\task-5\tests\mobile-audit-20261011\gap-audit.mjs' --source 'C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f' --output 'C:\Users\Admin\Documents\Codex\2026-10-11\task-5\tests\mobile-audit-20261011\evidence\gap-12ef' --commit 12ef2f5078d40b483c9d3e67489f51845a3a9f9c --committed true
& 'C:\Program Files\nodejs\node.exe' 'C:\Users\Admin\Documents\Codex\2026-10-11\task-5\tests\mobile-audit-20261011\gap-confirm-audit.mjs' --source 'C:\Users\Admin\Documents\Codex\2026-10-11\task-3\ui-work-3cb9b6f' --output 'C:\Users\Admin\Documents\Codex\2026-10-11\task-5\tests\mobile-audit-20261011\evidence\gap-12ef-confirmed' --commit 12ef2f5078d40b483c9d3e67489f51845a3a9f9c --committed true
```

| Run | UTC kết thúc | Harness SHA-256 | Cases SHA-256 |
| --- | --- | --- | --- |
| gap-12ef | `2026-10-11T12:31:02.412Z` | `010871fce3fba194833a08935b1338d908dc822baecb7e2d4a303b82d524d310` | `c3a8731b000a4dde3b36e4a5d7a924eb6368580bc29b82634d8079441c1002ef` |
| gap-12ef-confirmed | `2026-10-11T12:33:24.197Z` | `e30f839863286d1a9d1736d62d13d6197938ada8e8e56def39f50bddfee285d1` | `784dff3dee6040aac74b48a11d9d3b6720a6aa8088ca1728c46fd7e48c90772a` |

Full results, command arguments, source asset hashes, owned profile/ports và measurements: `tests/mobile-audit-20261011/evidence/gap-12ef/results.json` và `evidence/gap-12ef-confirmed/results.json`. Bốn screenshot chỉ chứa fixture data, đã xem trực tiếp. Scan và manifest của handoff là `gap-secret-scan.json` và `gap-deliverables-manifest.json`; không tạo Git checkpoint.

## Handoff và nghiệm thu còn lại

Integrator thread `01a1299d-c1f3-72db-9e36-eafba745401c` đã nhận hai phát hiện và ca xác nhận bổ sung bằng message tool. Gửi trực tiếp UI thread `01a12a58-38b6-77de-a3d4-7f44118a7022` gặp lỗi routing `AbsolutePathBuf deserialized without a base path`, nên chưa xác nhận UI nhận trực tiếp; integrator đã được yêu cầu chuyển tiếp.

- Sửa hai focus groups trong UI, đóng checkpoint immutable mới rồi chạy lại 8 bounded cases; không sửa runtime để chữa defect focus.
- Kiểm tra native Tauri build đã tích hợp: keyboard Tab/ShiftTab/Enter/Escape, work panel, drawer, hidden control sau SSE; giữ evidence exact integration commit.
- Nghiệm thu điện thoại thật: portrait/landscape khi popup/drawer đang mở; OS keyboard xuất hiện/ẩn và panning; textarea/IME; safe-area; Back/cancel; AT announcement/focus; background/foreground/offline/reconnect.
- Sync nhiều thiết bị, duplicate nonce/late replay và provider execution cần environment được người dùng cấp phép riêng; fixture này không đóng các hạng mục đó.
- Visual approval của hướng light vẫn là quyết định riêng của user. Kết quả hành vi không xác nhận phê duyệt thẩm mỹ.
