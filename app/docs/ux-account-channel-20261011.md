# Account và nhóm local — UI 2026-10-11

Nguồn UI: `7f850658f4dcd8e628e3529d1e05c1e78f24784c`, nhánh `ui-account-channel-84f7db7`, base `84f7db770f44137cbc6942344a5f83fa2549bf0c`. Chỉ bốn assets và hai UI runners được sửa. Giữ light direction đã được chấp nhận ngày 2026-10-11, NekoPeek identity và Geist có license; không thêm tài nguyên từ Grok bundle.

## Hành vi sản phẩm

- Chọn account ngay trên composer. Chỉ metadata `id` và revision được lưu trong lựa chọn; bí mật không vào localStorage, chat, outbox hay transcript. Card vẫn là `stored_unverified`: chọn card local không xác minh hay gọi provider.
- Shared channel có danh sách riêng ở sidebar. Bot nhận có thể đổi trong cùng nhóm; `@alias` chỉ chọn người thực thi cho request đó. Reply và tệp vẫn ở kênh gốc. Hội thoại riêng của bot không được trộn vào transcript nhóm.
- Tạo/sửa tên và 1–8 bot qua native dialog dùng existing local ACL POST. Mỗi lần lưu có expected revision và nonce. Lỗi 409 yêu cầu tải và xem lại cấu hình. Đóng/mở form hay đổi view không cho late ACK đóng form mới hoặc đổi view.
- Đổi account hay bot nhận chỉ áp dụng cho tin mới. Work card ghi người thực thi và cấu hình gốc; thu hồi account, lease expiry và audience change có thông báo riêng. Context refresh bị ẩn khi quyền gốc đã bị fence; UI không renew/rebind/resume authority.
- ACK luôn được lưu vào store/outbox đúng request. Chỉ xóa draft hiện hành nếu còn cùng view, bot, account và generation phù hợp. Reply không kéo người dùng về hội thoại đã rời.
- Tin chưa nhận ACK giữ toàn bộ `botId/text/lane/channelId/accountId/accountRevision/expectedChannelRevision/clientNonce`. Thử lại giữ tuple gốc ngay cả khi lựa chọn hôm nay thay đổi. 400/404/409 có trạng thái bị từ chối và cho sửa thành tin mới hoặc bỏ; lỗi transport/5xx giữ cùng request. Ngừng thử lại trên máy không hủy work mà server có thể đã nhận.
- Tin chờ nằm trong vùng hội thoại cuộn. Viewport thấp giữ Send/touch targets 44px; label và note bình thường vẫn có tên/vùng status cho AT. Composer tính lại chiều cao khi đổi viewport, giữ 52px trên desktop khi trống. Sidebar thấp cuộn toàn bộ; Tab/Shift+Tab ở form nhóm được giữ trong dialog. Không chặn IME, Shift+Enter hay chuyển focus do người dùng chủ động.

## Contract và freshness

`local_channels_v1` và `account_lease_fixture_v1` gate controls; legacy capability absent ẩn controls tương ứng. `local_channel_intake_cas_v1` mới cho phép gửi optional JSON number `expectedChannelRevision`; omission giữ compatibility với runtime84.

Account catalog có revision riêng, không có account SSE. UI refresh lúc nối lại/focus/tab mutation, bỏ catalog cũ và lỗi của lần đọc đã bị supersede. SSE channel được phân nhánh trước Work; event metadata khác chỉ tiến sequence, không tạo Work rỗng. EventSource cũ và snapshot có cursor cũ không thay store mới. Context ACK có counter giảm bị bỏ/resync; Cancel/Recover ACK cũng giữ work mới hơn đã nhận.

Artifact links ở cả transcript và Files dùng pinned reply channel; owner-wide link chỉ là fallback cho work legacy không có origin. Selector option DOM được giữ qua streaming/SSE để không mất focus vì dựng lại cùng option.

Backend CAS immutable `ab60378d874b1232dec5ad536ccb7b05f6ad9605` được materialize từ Git archive, SHA256 `c8b0f1d38b417aa5a5f61f12494dfbfa905444623e757d1dddc45b0969d5e688`, rồi nhập UI-only commits trong checkout test riêng. Integration code commit `9721134b8aad8f3b6825798e328d8f921fdf3fae`; backend Go/Rust không bị sửa. Backend receipt `cf99b46d9b547e3a81df800c2594c8d676f15cfc64d0b56e8fba784c39c540be`, raw Go log `2f3ef2c0725c833b8de2119c7b3616c8415ea42cf4d7130597e0693205bc7811`. CAS nằm trong embedded SubmissionScope/ResourceScope và AcceptProduct; chỉ đọc server.go/channels.go không đủ để đánh giá contract.

## Kiểm tra và giới hạn

Runners: `scripts/ui_account_channel_local.mjs`, `scripts/test_ui_redesign.py --context-fixture`, `scripts/ui_light_pixels.mjs`; independent runner ở owned task-3 `.build/independent-account-channel-ui`. Mỗi JSON report ghi code commit, command, environment, trạng thái, harness/assets/binary SHA. Các báo cáo final và ảnh được tập hợp trong handoff manifest ngoài source.

Actual local runner kiểm bốn embedded assets khớp bytes source trước khi test, Go journal/DPAPI synthetic accounts, server pin, ACL revoke, optional CAS, original lost-ACK retry sau ACL edit và new stale nonce 409. ACK503 được CDP giữ có chủ đích; đây là fault injection quanh Go thật. Không dùng provider hay Core. Regression context controls dùng CDP endpoint/snapshot stub; không gọi đó là native stop proof. Independent fixture kết luận trong phạm vi state/navigation/layout, không thay actual backend/native evidence.

Trong quá trình tạo runner đã sửa readiness trước DOM, thứ tự bot được server normalize, wait theo nonce cũ thay vì Send ở view khác, fixture sequence và JSON object member order. Một binary build trước UI cuối đã bị loại khỏi exact evidence; final runner gate đối chiếu served bytes mới được dùng. Actual rich-state short viewport và retry draft lỗi được sửa riêng sau vòng independent28 đầu; evidence trước được giữ.

Chưa có phone vật lý, bàn phím điện thoại, nonzero safe-area, AT speech, native Tauri cho slice mới, provider evaluation hay multi-owner/cloud acceptance. Root giữ native slot và kiểm integration riêng. Light direction đã được chấp nhận; các account/channel controls mới và ảnh mới cần người dùng review. Không thêm project/upload/action controls trước contract/product work của runtime owner.

## Kết quả own final

Windows, PowerShell 7.6.5, Node v25.9.0, Chrome 154.0.8037.98, Go1.27.2 windows/amd64; host DPAPI/binary/harness/assets SHA được ghi trong JSON. Không cộng các suite chồng phạm vi thành một con số acceptance.

| Suite | Code commit | Kết quả | Evidence trong clone tương ứng |
| --- | --- | --- | --- |
| Actual Go CAS + UI + pixels | 9721134b8aad8f3b6825798e328d8f921fdf3fae | PASS 27 / FAIL 0 | .build/evidence-account-channel-972-final/results.json |
| Actual Go legacy84 + UI | 7f850658f4dcd8e628e3529d1e05c1e78f24784c | PASS 22 / FAIL 0 | .build/evidence-account-channel-7f-legacy/results.json |
| UI regression + context stubs | 7f850658f4dcd8e628e3529d1e05c1e78f24784c | PASS 73 / FAIL 0 | .build/evidence-redesign/ui-smoke.json |
| Light/pixel static fixture | 7f850658f4dcd8e628e3529d1e05c1e78f24784c | PASS 23 / FAIL 0 | .build/evidence-light-working/pixels.json |

Sau code checkpoint chỉ cập nhật tài liệu/handoff; bốn assets và hai runners vẫn cùng Git blob IDs đã kiểm. Independent final report/manifest được giao riêng và không thay báo cáo own.
