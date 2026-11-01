# Chat thật: OpenMausBot và Neko Aurora — 2026-11-01

## Phạm vi và môi trường

Theo yêu cầu trực tiếp của người dùng, chạy OpenMausBot với ChatGPT thật và đối chiếu Aurora. Tự thực hiện, không subagent. Các lượt thử dùng dữ liệu mẫu, không thực hiện gửi thư, thanh toán, cài phần mềm hay tác động dịch vụ ngoài.

- Aurora gốc đang mở ở cổng 63501. Baseline thử riêng: cổng 63621, cùng executable SHA-256 `2878f5240d00cc34738cc9f9dd8ea9a38f18241a3f78939e8bcff6e37816055c`, code `b782ad4534d4b108d745a51395245401a2920ed8`.
- OpenMausBot: Apache-2.0, `88d54eb17a3655b968470cbbf3b5097de225a8b5`, v0.1.136; giữ nguyên `reference/OpenMausBot`, bản chạy ở `reference/OpenMausBot-live`, UI 63610, server 63611.
- Windows, Node 25.9.0, pnpm 10.14.0, Codex CLI 0.160.0, Go 1.27.1. Cài OpenMaus theo lockfile bằng `pnpm install --frozen-lockfile --ignore-scripts --config.manage-package-manager-versions=false`; không chạy script tải Electron hay cài git hook.
- Model đối chiếu: `gpt-6-sol`, tài khoản ChatGPT đã được kết nối trong Aurora. Đây là inference thật, dùng hạn mức tài khoản. Không dùng API key trả tiền riêng.
- Adapter `scripts/comparison-codex.mjs` trong bản OpenMaus riêng truyền access token qua giao thức `account/login/start`, loại `chatgptAuthTokens`, của native Codex. Chỉ cấp lease hai giờ; không sao chép refresh token, không ghi token vào source/log/argv. Khi cần refresh phải dừng và cấp lease mới. Không mô phỏng câu trả lời của model.
- Đây là kiểm tra **tái sử dụng tài khoản đã đăng nhập**, không phải kiểm thử một vòng OAuth mới từ trình duyệt. Không thay implementation hội thoại, prompt, tool dispatch hay steering của OpenMaus. Chỉ thêm adapter xác thực thử nghiệm và tắt analytics của bản này.
- Dữ liệu thử ở `%LOCALAPPDATA%/NekoAuroraComparison/2026-11-01`. Lần khởi động đầu chưa đặt `CODEX_HOME` tường minh, nên native Codex đọc cấu hình/instructions của người dùng. Đã sửa launcher đặt home riêng và chạy lại ca nghiên cứu trên bot mới `Meiiie Live`. Các số thời gian không phải benchmark: lịch sử, prompt, số lần gọi và mức tải khác nhau.

## Kết quả baseline thật

| Ca thử | OpenMausBot | Aurora baseline |
|---|---|---|
| Chào, ghi nhận dự án mẫu Lá Mầm và hạn thứ Sáu | Trả lời đúng, một câu | Trả lời đúng, một câu |
| Tin tiếp theo đổi hạn sang thứ Hai | Nhớ tên dự án, đổi đúng hạn | Nhớ tên dự án, đổi đúng hạn |
| Đọc trang tải Node.js và Go; gửi yêu cầu bỏ bảng sau 3 giây | Có native `webSearch`, kết quả nguồn gồm cả `nodejs.org` và `go.dev`; trả hai dòng theo tin bổ sung | Không có lần đọc trang; trả lời không thể mở trang trong lượt này |
| Gửi bổ sung trong lúc đang làm | Receipt `steered: true`; cùng native thread/turn; UI đánh dấu `sent mid-turn` | Receipt ban đầu `queued: true`; sau đó Go hấp thụ vào yêu cầu đang chạy và gọi model lại |
| Đọc tên miền `.invalid` | Thử công cụ, báo không đọc được, không bịa | Nói không đọc được; baseline không có bằng chứng đã thử công cụ |
| Dừng khi đang xử lý, rồi nhắn tiếp | Dừng được; lượt sau trả đúng câu yêu cầu | Dừng được; lượt sau trả đúng câu yêu cầu |

OpenMaus không chỉ hiển thị link trong câu trả lời. Native receipt `item/completed` chứa `webSearch.results` với hai trang nguồn trong ca nghiên cứu. Tuy vậy bảng trên chỉ chứng minh các ca đã chạy, không phải đánh giá tổng quát về năng lực model.

Lần chạy lại trên bot mới có home riêng vẫn truy xuất được hai nguồn, nhưng tin bổ sung gửi lúc native session đang khởi động được **xếp hàng**, không steer ngay. Kết quả là một bảng theo yêu cầu cũ, rồi một câu trả lời hai dòng theo tin bổ sung. Receipt và UI thể hiện đúng hai lượt. Đây là giới hạn quan sát được của OpenMaus, không nên mô tả nó luôn xử lý tin giữa chừng hoàn hảo.

Chứng cứ local, không đưa credential vào Git:

- `.audit/2026-11-01-live-chat/openmaus-research.json`, `aurora-research.json`: prompt, receipt, chuỗi trạng thái và hội thoại thật.
- `openmaus-context.json`, `aurora-context.json`: nhớ ngữ cảnh.
- `openmaus-unavailable.json`, `aurora-unavailable.json`: nguồn không tồn tại.
- `openmaus-research-isolated.json`: lần thử lại với Codex home riêng.
- `openmaus-stop.json`, `aurora-stop.json`, `openmaus-recovery.json`, `aurora-recovery.json`: dừng và nhắn tiếp.
- `openmaus-greeting/transcripts/`: transcript được export qua harness chính thức `control-omb`.
- `launch.json`, `aurora-launch.json`: bản chạy, PID, đường dẫn state, commit/hash.

## Nguyên nhân trong Aurora và bản sửa hẹp

1. **Chỉ dẫn tự mâu thuẫn.** `turn.go` yêu cầu in `TOOL: {...}` để trusted host chạy công cụ, nhưng `corePrompt` lại thêm “không gọi công cụ” ở cuối yêu cầu. Thay câu cấm chung bằng hợp đồng rõ: model phát directive Aurora; host xác thực và thực thi; Core native tools vẫn bị tắt. Không bật quyền shell hay bỏ approval.
2. **Tin hiện tại bị đẩy vào lịch sử khi có system nudge.** Bộ chuyển đổi chỉ nhận user cuối nếu đó là phần tử cuối mảng. Plan/tool/final nudge phía sau khiến phần “yêu cầu hiện tại” trống. Bản sửa tìm user cuối độc lập với các system message và giữ nguồn gốc của peer context.
3. **URL người dùng cung cấp bị chặn vì ngoài danh sách mặc định.** Cho phép host đọc đúng URL HTTPS do chủ hội thoại cung cấp hoặc bổ sung khi đang làm, qua `FetchFound`. Không cấp quyền toàn tên miền; URL khác vẫn bị chặn. HTTPS, cấm userinfo/cổng lạ, kiểm tra địa chỉ public ở thời điểm dial và chính sách redirect vẫn giữ nguyên. Agent peer không tự cấp quyền URL cho chủ hội thoại.

## Khác biệt kiến trúc còn lại

Đây là phần nên học từ cách OpenMaus hoạt động, không chép toàn bộ code hay prompt:

- **Hội thoại có phiên thật:** OpenMaus start/resume native thread và gửi tin hiện tại dưới dạng user input. Aurora tạo tiến trình CLI cho từng lần hỏi và ghép system/history/tool results thành một chuỗi. Cần chuyển boundary sang session và thông điệp có role rõ ràng, để Neko Core thực sự sở hữu agent loop theo định hướng dự án.
- **Tool call có cấu trúc:** OpenMaus nhận sự kiện native cho tool/progress/completion. Aurora vẫn yêu cầu model in JSON directive trong văn bản. Sửa prompt xử lý lỗi trước mắt; chưa thay thế được hợp đồng tool có schema, ID và kết quả nối tiếp.
- **Tin bổ sung vào lượt đang chạy:** OpenMaus dùng native steer; Aurora đọc inbox ở các mốc, bỏ draft rồi gọi lại. Cần phân biệt rõ “đã nhận”, “đã đưa vào lượt đang chạy”, “sẽ xử lý lượt sau”; không chỉ hiện chung là đang làm.
- **Tiến độ phản ánh việc thực tế:** OpenMaus phân biệt commentary với final và gắn activity vào tool item. Aurora cần giữ được các loại sự kiện này xuyên Core → Go → UI, tránh phát lại toàn bộ lời hứa hoặc coi kết thúc một lần gọi model là hoàn thành công việc.
- **Lỗi có khả năng tiếp tục:** giữ request và checkpoint, nối retry đúng thao tác chưa hoàn thành; không phát lại tác động ngoài khi không biết thao tác trước đã thành công hay chưa.

Không nên dùng OpenMaus làm chuẩn tuyệt đối. Với Codex CLI cũ trong phép thử, catalog của nó rơi về danh sách dự phòng với nhãn `availability unverified`; các lời chào/setup của bot còn xuất hiện khi chưa có model riêng. Những phần này không nên đưa nguyên trạng vào Aurora. Giữ nguyên hướng catalog theo tài khoản và model receipt đã làm trong Aurora.

## Kiểm chứng bản sửa

Theo `AGENTS.md`: ghi command, base commit, môi trường, trạng thái và hash source; không đồng nhất fixture với chat thật.

- Kiểm thử tập trung cho prompt, nguồn gốc peer, URL chính xác, không cấp quyền URL khác, tool receipts, steering và chặn private address: PASS.
- Lần chạy bộ test đầu có 612 pass và hai test yêu cầu sandbox Linux bị fail trên Windows. Dùng Go source overlay tại `4ee689c` tái hiện cùng hai lỗi. Hai test đó nay kiểm tra `CanRun` và báo skip nếu host không có sandbox, giống quy ước của các test máy tính khác. Không bật chạy lệnh không sandbox để làm test xanh; không coi skip là kiểm chứng OS sandbox.
- Bản sửa tra cứu: 612 test pass, 10 skip vì năng lực host; `go vet` và build Windows PASS. Manifest `candidate-manifest-final.json`, executable SHA-256 `8cc01c4c65ab469357e80ddd526d064f359023a954331178bc74cae865fe041b`. Ca ChatGPT thật `aurora-research-fixed.json` đã kết thúc thành công: hai `browser_open` đọc hai trang qua HTTP (không phải trình duyệt tương tác), có nguồn và trả đúng hai dòng theo tin bổ sung. Harness đã ghi đủ evidence trước khi lệnh in tóm tắt gặp lỗi encoding console Windows; dữ liệu JSON và kết quả model không bị mất.
- Lệnh đã chạy: `go test -p 1 -json ./internal/aurora ./internal/tools -count=1`, `go vet ./internal/aurora ./internal/tools`, `go build -o <audit>/neko-aurora-candidate.exe ./cmd/neko-aurora`, từ `app/native`, Go 1.27.1 Windows, môi trường allowlist không chứa provider credentials. Base commit `4ee689cfd17aa6c97011a0a3bfdee14576606273`; hash từng source ở manifest nói trên.
- Test live qua HTTP API cục bộ là kiểm tra backend; kiểm tra hiển thị trình duyệt được ghi riêng, không đánh đồng hai loại.
- Chưa kiểm thử live GLM/API providers trong lượt này; không đo khả năng cloud sync, sandbox OS, mobile native hay năng lực model trên một bộ đánh giá lớn.

## Điều chỉnh theo phản hồi mới: giữ đoạn trả lời khi nhắn thêm

Người dùng xác nhận Meiiie đã khá ổn định và yêu cầu tập trung vào đoạn phản hồi xuất hiện rồi biến mất khi chèn tin giữa lượt. Không triển khai thay kiến trúc phiên/agent loop trong lượt này.

Nguyên nhân cụ thể: `ask()` chủ động hủy stream khi inbox có tin mới rồi phát `draft_end`. Client xóa `S.drafts[requestId]`, nhưng đoạn vừa hiện chưa có message trong journal. Nhánh nhận tin muộn ở cuối lượt cũng bỏ draft như vậy. Đây không phải thao tác ẩn/xóa tin đã lưu của người dùng.

Bản sửa lưu phần văn bản công khai đã stream thành `DirectPartial` trước khi phát sự kiện xóa draft. Mỗi đoạn có ID riêng, đứng trước tin bổ sung tương ứng, và nhãn “Đoạn trả lời bị ngắt khi bạn nhắn thêm”. Phản hồi tiếp tục giữ riêng phía dưới. Không gán đoạn dở dang là kết quả hoàn thành; không thực thi tool/card đang viết dở. Chỉ giữ phần trước chỉ thị nội bộ, qua bộ che bí mật, không lưu reasoning vào đoạn này.

Client chuyển draft sang message trong cùng lần cập nhật, không để khoảng trống rồi mới tải lại lịch sử. Khi nguồn của đoạn cũ được cập nhật, không xóa draft của câu trả lời mới. Schema lưu trữ bổ sung trường tùy chọn; giữ dữ liệu cũ. Đoạn đã lưu được trả lại khi reload chat và qua replay journal của lượt đã kết thúc. Chưa mở rộng cam kết sang khôi phục mọi đoạn của lượt bị crash và tự resume.

Các kiểm tra bổ sung: stream thực từ HTTP fixture → nhận tin mới → lưu message trước `draft_end`; nhiều lần chèn tin giữ thứ tự; tool/card JSON đang viết dở không lộ; journal replay và snapshot không dùng chung mảng; cập nhật nguồn cũ không xóa câu mới; không tạo bản sao đoạn cũ. Đây là fixture, không phải bằng chứng chất lượng model.

Kết quả kiểm thử toàn phần bị ảnh hưởng và ca live cho thay đổi này được ghi tiếp trong `validation.jsonl`, manifest `candidate-manifest-interrupted.json`.

Bộ test gồm `./internal/aurora ./internal/tools ./internal/ledger ./internal/web`: 1.449 pass, 11 skip, không fail; `go vet` và build Windows PASS. Node: hai ca giữ tin nhắn bị ngắt và 19 kiểm tra UX PASS; syntax và docs check PASS. Đây là kết quả trên manifest `candidate-manifest-interrupted.json`.

Lần chat thật đầu xác nhận đoạn bị ngắt được lưu riêng, câu trả lời tiếp theo đúng yêu cầu bổ sung. Đồng thời phát hiện ký tự tiếng Việt bị mất khi Core chia stdout giữa các byte UTF-8: vị trí cắt kiểm tra byte trước ranh giới thay vì byte tại ranh giới. Sửa phép kiểm tra đó; phần lưu đoạn bị ngắt lấy văn bản đầy đủ đã nhận thay vì ghép lại các delta. Regression mới phát từng byte tiếng Việt/emoji qua tiến trình Bun thật và xác nhận mọi delta hợp lệ, ghép lại đúng nguyên văn, không lộ phần thống kê cuối CLI.

Sau sửa Unicode, chỉ ba file khác manifest đầy đủ: `core.go`, `turn.go`, `core_unicode_test.go`. Đã chạy lại nhóm test Core CLI thực với các provider fixture, Unicode, interruption, burst và thứ tự hội thoại; PASS. Vet/build chạy lại PASS. Hash source cuối: `candidate-manifest-unicode.json`; các lệnh và bằng chứng nằm trong `validation.jsonl`. Không gọi thêm các provider API trả tiền trong lượt kiểm tra này.

Ca ChatGPT thật cuối PASS trên executable SHA-256 `3098cd0f8db22d70645f99c19df91d0f89df1ed91598a051c14cd9b89aab8dc2`, `gpt-6-sol`, state riêng cổng 63631: 254 ký tự của đoạn truyện đang viết được lưu thành tin bị ngắt; tin bổ sung đứng sau đoạn đó; câu trả lời cuối là “Mình đã nhận lời nhắn bổ sung của bạn.”. Kiểm tra trình duyệt và reload PASS: cả ba đoạn vẫn hiện, tiếng Việt đúng. Evidence: `aurora-interrupted-live.json`, `aurora-interrupted-launch.json`, `aurora-interrupted-after-reload.png`. Lần r1 được giữ riêng để không che lỗi Unicode đã phát hiện.

Giới hạn: chỉ xác nhận ca chat thật và các regression kể trên; không tuyên bố mọi chức năng/provider đã hoàn thiện. Luồng xử lý yêu cầu bổ sung của Aurora được giữ nguyên, thay đổi chính là lưu và hiển thị đoạn phản hồi bị ngắt.
