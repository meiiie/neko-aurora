# Neko Aurora: tiếp nhận nghiên cứu Grok Bot — 28/10/2026

Đã đối chiếu `origin/main` của `meiiie/neko-aurora` sau fetch ngày 28/10/2026: `66376d68691d8bce16b6565c6268c4331802ef57`. Nhánh làm việc: `codex/grokbot-adoption`. Gói thay đổi dưới đây phát triển từ commit này, chưa phải release đã triển khai.

Nguồn tham khảo là [changelog chính thức của Grok Bot](https://x.ai/changelog/bot), đến 0.77.0 ngày 26/10/2026. [Danh mục 55 mục phát hành](research/grokbot-release-inventory-2026-10-28.md) và [index 820 dòng thay đổi](research/grokbot-adoption-index-2026-10-28.json) giữ dấu vết nguồn. Đây không phải 820 tính năng đã hoàn thành. Không đưa mã, tài sản hay toàn văn changelog của Grok vào repository.

Quy trình dùng pstack [Feature](https://github.com/cursor/plugins/blob/main/pstack/skills/poteto-mode/playbooks/feature.md), [Prove It Works](https://github.com/cursor/plugins/blob/main/pstack/skills/principle-prove-it-works/SKILL.md) và Show Me Your Work; tự thực hiện và tự review theo yêu cầu không dùng subagent. [Webapp-testing](https://github.com/anthropics/skills/tree/main/skills/webapp-testing) hướng dẫn kiểm tra tương tác bằng trình duyệt headless. Không gọi dịch vụ AI thật, cấp grant hay triển khai trong các kiểm thử này.

## Phần đã triển khai

| Hành vi | Thay đổi trong Aurora | Bằng chứng |
|---|---|---|
| Nhập bộ nhớ có duyệt | Mỗi dòng một điều, xem trước, chọn từng mục; lọc trùng và mẫu bí mật/Vault; kiểm tra revision trước khi lưu. Giới hạn 100 dòng, 300 ký tự/mục, 30 mục/bot; không đẩy mục cũ ra để lấy chỗ. | `memory_import_test.go`, kiểm tra HTTP và trình duyệt |
| Tạm dừng lịch | Nút tạm dừng/tiếp tục/xóa với revision. Thay đổi cũ bị từ chối; hủy occurrence còn chờ, không tự dừng lượt đã chạy. Admission thất bại không làm mất lịch một lần. | `schedule_control_test.go`, kiểm tra HTTP và trình duyệt |
| Khôi phục tin chưa gửi | Lưu riêng từng tin trên thiết bị theo workspace; giữ nonce khi tải lại, timeout POST 30 giây, tối đa 128 tin. Hết 7 ngày ngừng tự gửi nhưng vẫn giữ nội dung. Tin bị từ chối giữ lại để xem/copy/xóa. | `outbox-test.mjs`, trình duyệt offline/reload/lost receipt/account switch/storage failure |
| Hạn mức khi retry | Hosted lưu reservation cho nonce và payload, chống tính lượt lặp lại; payload đổi nhận 409. Bộ nhớ hỏi/tóm tắt đi qua quota; áp dụng chỉnh sửa đã duyệt không gọi model. | `message_quota_test.go`, `utility_quota_test.go` |
| Câu hỏi không bị trả lời nhầm | Tin nhắn tiếp nối thường không đóng lựa chọn. Có câu trả lời tự nhập; `answerFor` và nonce cố định gắn với đúng câu hỏi, giữ qua lần tải lịch sử. | `adoption_api_test.go`, trình duyệt |
| Biểu mẫu theo biên nhận | `formFor` và nonce cố định gắn câu trả lời với đúng form/chat. Giữ giá trị offline qua tải lại; chỉ hiện đã gửi sau biên nhận ledger. Không có receipt thì endpoint hoàn tất trả 409; đổi nội dung sau khi nhận bị từ chối. | `form_receipt_test.go`, HTTP và trình duyệt mất mạng/mất phản hồi |
| Vòng đời MCP | Cache riêng từng service/workspace, đổi khóa làm mới phiên; một handshake khi truy cập đồng thời. 401/403 ngừng tự thử, 429 chờ `Retry-After`; chủ động kết nối lại, không tự phát lại tool có tác động. Chặn chuyển credential qua redirect/cross-origin SSE. | `mcp_lifecycle_test.go`, trình duyệt reconnect/cooldown; server giả lập cục bộ |
| Bảng trong chat | Sao chép TSV/Markdown, hộp xem lớn, zoom 100–200%, Escape và trả focus. Nội dung HTML vẫn là văn bản. | Trình duyệt 1280×900 và 390×844 |
| Độ tin cậy công cụ | Ledger và harness thống nhất tối đa 10 công cụ. Không chạy công cụ khi chưa ghi được trạng thái bắt đầu. Gửi lại tin không supersede bản duyệt hiện có. | Regression đỏ trên base, xanh sau sửa; replay/overflow và kiểm tra HTTP |
| Giao diện và đóng gói | Giữ bot đích khi ảnh đang tải; đăng ký hai script mới trong Go asset registry và cache PWA v48. | JS syntax, test từng script qua Go asset server |
| Lỗi nền và tài liệu | Đóng ledger trong test public-host; tách deadline lỗi giả lập với deadline production khi kiểm tra worker tái sử dụng; giữ ngữ cảnh file khi computer không chạy được; sửa hai link tài sản lịch sử, phục hồi nguyên bản notice Geist cho banner. | Kiểm tra đích và docs checker |
| Sáu lỗi Windows và Rust | Fixture Core chạy child process native; Railway dùng Git Bash/Python, stdin cho lệnh dài và tar local. Vault/backup tạo file với DACL chỉ SID hiện tại trước khi ghi, kiểm tra và sửa ACL file cũ. Pin Rust 1.97.1 và helper chọn MSVC/SDK đã cài. | Sáu test đều PASS; toàn bộ Go 25 package PASS; 43 test Rust host PASS; Tauri check PASS |

Các implementation chính nằm trong [Aurora service](../app/native/internal/aurora/service.go), [memory import](../app/native/internal/aurora/memory_import.go), [schedules](../app/native/internal/aurora/schedule.go), [hosted admission](../app/native/internal/hub/message_quota.go) và [PWA](../app/native/internal/web/assets/app/app.js).

## Giới hạn của phần mới

- Nhập bộ nhớ là thao tác của owner trên một bot, chưa có đọc/export trực tiếp từ tài khoản trợ lý khác. Bộ lọc bí mật dựa trên mẫu và các giá trị Vault đã biết, không phải bảo đảm phát hiện mọi bí mật.
- Câu hỏi hỗ trợ một nhóm lựa chọn với câu trả lời riêng. Chưa triển khai thẻ nhiều câu hỏi từng bước. Form mới hoàn tất theo receipt; cờ hoàn tất đã lưu bằng phiên bản cũ được giữ để tương thích, không tự suy đoán lại nội dung lịch sử.
- MCP được kiểm tra bằng remote transport giả lập; chưa có OAuth discovery/refresh hay acceptance Gmail/Outlook/GitHub thật. Trạng thái tạm thời nằm trong service đang chạy; restart không phải bằng chứng phục hồi phiên OAuth.
- Bảng sao chép TSV và Markdown; chưa xuất định dạng Slack rich text, căn dấu thập phân, footnote hay mọi chế độ bảng của nguồn tham khảo.
- Outbox là dữ liệu trong trình duyệt của thiết bị, không phải cloud sync. Tin đã bị từ chối không tự gửi lại khi đăng nhập hoặc hết quota; người dùng xem lịch sử rồi quyết định. Retry có tham chiếu chat đã đổi hoặc nhóm đã đổi revision có thể nhận 409 và được giữ lại để xem xét. Chưa có transaction fan-out toàn bộ nhóm.
- Quota reservation giữ tối thiểu đến lần dọn sau 8 ngày và giới hạn 16.384 mục/workspace; lỗi admission sau reservation giữ lượt đã dành. Không tuyên bố exactly-once cho email, thanh toán hay công cụ bên ngoài.
- Lịch dùng múi giờ Việt Nam và các kiểu lặp sẵn có. Kiểm tra revision là trên cùng server; chưa kiểm chứng bằng hai điện thoại thật. Occurrence đã chạy cần dừng bằng thao tác trong chat. Dữ liệu lịch vẫn dùng file hiện hữu, không phải bộ điều phối phân tán.
- PWA là bề mặt đã kiểm tra. Chưa có acceptance Tauri, iOS/Android vật lý, APNs production hay đánh giá chất lượng model.

## Công việc còn lại từ toàn bộ nguồn

| Nhóm | Điểm tích hợp hiện có | Điều kiện để được ghi nhận hoàn thành |
|---|---|---|
| Connector, email và reconnect | `aurora/mcp.go`, `connectors.go`, `outbound/`, `connections/` | Đã có foundation MCP cục bộ; còn OAuth/provider-specific lifecycle, schema refresh, fake-provider contract từng dịch vụ rồi acceptance tài khoản được cấp phép. |
| Chat, nhiều câu hỏi và form | `aurora/view.go`, `groups.go`, PWA | Đã có form theo receipt; còn nhiều câu hỏi có danh tính riêng, quoted reply, giữ nháp/tệp khi đổi chat, fan-out nguyên tử và retry tham chiếu đã pin. |
| Routine, webhook và handoff | `schedule.go`, `network.go`, ledger | Lịch phong phú, webhook có identity, reconnect refresh, nguồn/đích kết quả, hạn mức trigger và restart admission. |
| Pages, Library và tài liệu | `documents/`, `documentworker/`, `artifacts.go`, UI file | Biên tập/lưu/xuất có version; phục hồi crash; kiểm tra cấu trúc và render nội dung thật, accessibility. |
| Computer và browser | Rust host, Go computer, cloud-browser | Quyền theo hành động, cancellation, reconnect đúng thiết bị, artifact outcome; kiểm thử crash/unknown outcome và host thật. |
| Voice và thông báo | PWA voice, `push.go`, native shell | Cuộc gọi realtime, reconnect microphone, tuyến desktop/phone, APNs sender và kiểm thử thiết bị thật. |
| Team/Enterprise | Hosted identity, ledger requester/publication | Role, tenant, connector riêng, memory scope, audit/revoke; không dựng giao diện quyền nếu backend chưa cưỡng chế. |
| Billing và payment | Hosted plan/quota, Vault/outbound | Adapter theo nhà cung cấp được chọn, giao dịch có biên nhận, sandbox, phê duyệt owner; thương hiệu/quyền lợi/X Money của Grok không thể chép thành dịch vụ Aurora. |
| Desktop và nền tảng | `app/host`, `app/desktop`, `core` | Đã sửa fixture Windows/toolchain cho checkout; còn installer/update, offline và acceptance từng nền tảng. |

Mục đổi model để vượt một lần dừng của bộ lọc an toàn không được tiếp nhận. Fallback vì lỗi vận hành không phải cơ chế bỏ qua quyết định an toàn.

Ưu tiên tiếp: retry nhóm nguyên tử và tham chiếu đã pin; câu hỏi nhiều bước; lifecycle OAuth/provider thật; lịch/webhook, tài liệu, voice và quyền nhóm. Mỗi mục cần bằng chứng riêng trong index; không tự chuyển trạng thái theo keyword hay theo việc có file tương tự. Tám dòng nguồn hiện có bằng chứng cục bộ một phần; 811 dòng vẫn chưa được ghi nhận có kiểm chứng tương ứng.

## Bằng chứng kiểm tra

Log cục bộ ban đầu tại `.audit/2026-10-28-grok/validation.jsonl`; lượt tiếp tục tại `.audit/2026-10-28-windows-backlog/validation.jsonl`. Mỗi dòng ghi base SHA, hash diff, manifest SHA256 của tracked/untracked source, lệnh, thư mục, môi trường, exit code và PASS/FAIL. Runner cuối chụp source trước lệnh và so lại sau lệnh, đánh dấu STALE nếu source đổi trong lúc kiểm tra. Log, state và executable kiểm thử không được đưa vào commit. [Test API](../app/native/internal/aurora/adoption_api_test.go), [test browser](../app/tests/aurora/adoption-browser.py), [test outbox](../app/tests/aurora/outbox-test.mjs) được giữ trong source để chạy lại.

Môi trường: Windows amd64, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome headless. Runner loại bỏ biến môi trường provider/account, chỉ dùng fixture và loopback. Kết quả cuối cùng được ghi trong [validation report](research/grokbot-validation-2026-10-28.md).
