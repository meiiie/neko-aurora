# Kết nối model và trải nghiệm hội thoại — 2026-11-01

Aurora mở rộng danh sách model theo kết nối thật cho GLM Coding Plan, GLM API, OpenAI API, Claude API, Gemini API và DeepSeek API; giữ cơ chế catalog tài khoản ChatGPT đã có. Nút AI cạnh composer mở đúng cấu hình của bot. Lựa chọn mới áp dụng từ lượt tiếp theo; lượt đang chạy giữ route, model và khóa API đã chụp trong bộ nhớ.

## Nguồn nghiên cứu

Reference `reference/OpenMausBot` đã được cập nhật từ `10af83e7` tới [88d54eb17a3655b968470cbbf3b5097de225a8b5](https://github.com/milind-soni/OpenMausBot/tree/88d54eb17a3655b968470cbbf3b5097de225a8b5), commit ngày 2026-10-31 UTC. Checkout sạch, đọc source/tests và ảnh minh họa; không chạy chương trình hoặc cài dependency. License của snapshot là Apache-2.0. Những thay đổi dưới đây là triển khai riêng trong stack hiện tại; không sao chép source hoặc tài sản hình ảnh. Đây là nghiên cứu sâu các luồng liên quan yêu cầu, không tuyên bố kiểm toán toàn bộ repository.

## Đối chiếu hành vi với OpenMausBot

| Luồng | Bằng chứng source tại snapshot | Áp dụng cho Aurora |
|---|---|---|
| Chọn AI | `src/components/ModelPicker.tsx`, `src/lib/model-picker-pane.ts`, `src/lib/model-choices.ts`, `src/lib/engine-rail.ts`: danh sách theo engine, readiness và lựa chọn rõ ràng | Catalog riêng từng kết nối, trạng thái đang tải/lỗi/tài khoản chưa có model. Không chèn preset. Giữ native select để dùng bàn phím và mobile; nhãn có cả ID nếu tên hiển thị khác ID. |
| Kết nối trước khi chọn | `server/model-catalog/catalog.ts` có known-model catalog/cache; không phải bằng chứng quyền của từng tài khoản | Aurora chỉ dùng phản hồi catalog bằng khóa/tài khoản đang chọn. Phân biệt GLM Coding Plan với API tính theo mức dùng. Không suy diễn quyền truy cập từ models.dev hoặc danh sách marketing. |
| Đổi AI khi đang làm | `src/lib/ai-switch.ts`: theo dõi từng thread/turn, đợi interrupt hoàn tất khi thay AI ở lượt đang chạy | Aurora giữ nguyên AI cho toàn bộ lượt đang chạy, áp dụng cấu hình mới ở lượt kế tiếp. UI nói rõ. Đây là lựa chọn thích ứng, không phải bản sao semantics interrupt của reference. |
| AI nhìn thấy được | `ModelPicker.tsx`, `ChatHeadPill.tsx` và ảnh `docs/screenshots/model-picker.png` | Provider/model ở header, hồ sơ và cạnh composer. Khi có lượt đang chạy, header lấy model từ receipt của lượt; composer hiện AI cho lượt tiếp theo. Mặc định Aurora và cấu hình riêng của bot phân biệt rõ; tiêu đề nói lựa chọn cho tin nhắn mới. Receipt của lượt lấy cùng snapshot với đường gọi model. |
| Câu trả lời tự nhiên | `server/system-prompt.ts`, `shared/reply-style.ts`, `src/components/ChatView.conversational.test.ts`: độ dài theo câu hỏi, đoạn văn rõ, giữ code/bảng | Bổ sung hướng dẫn riêng: chào/câu ngắn trả lời vừa đủ, không dựng kế hoạch; yêu cầu dài nêu kết quả trước; hỏi khi thiếu điều quyết định. Giữ renderer Markdown hiện có, không tách paragraph làm vỡ code hoặc bảng. Bỏ hiệu ứng gõ giả cho phản hồi đã lưu: lịch sử hiện đầy đủ khi mở lại; nội dung đang stream vẫn đến từ delta thật. Chưa đo chất lượng model thật. |
| Tiến độ | `src/state/turn-status.ts`, `ChatView.live-status.test.ts`, `ChatView.progress-lines.test.ts`: trạng thái từ sự kiện, lời bot không bị nuốt vào tool summary | Giữ trạng thái/tool timeline từ runtime thật, thu gọn hoạt động đã có. Không thêm bước tiến độ giả hoặc hiệu ứng làm như đang gọi công cụ. Aurora ẩn dòng chờ chung khi đang stream chữ để giảm trùng lặp. |
| Hàng đợi và gửi lại | `src/state/queue-receipts.ts`, `ComposerQueuedMessages.tsx`: receipt/tombstone chống nhận lại phản hồi queue cũ | Giữ outbox, nonce, draft theo bot, stop và queue hiện có. Mở setup/tải model/lưu AI không tự gửi lại tin đã lỗi. Browser test kiểm tra không có POST messages trong luồng setup. |
| Lỗi | `server/turn-outcome.ts`, `src/state/chat-error.ts`: kết quả gắn với lượt; một số banner tạm thời tự biến mất | Aurora giữ lỗi cần xử lý cho đến khi người dùng giải quyết, phân biệt khóa/quyền/quota/catalog/model. Không hiển thị raw stderr có thể chứa dữ liệu nhạy cảm; không đổi lỗi thành câu trả lời thành công. |
| Fast routing | `server/turn-route.ts` có đường chọn model nhanh trong một số điều kiện | Không đưa vào Aurora: model do người dùng chọn phải quyết định route chat. Không âm thầm thay bằng model rẻ/nhanh hơn. |

Các đường dẫn trên đều nằm trong [snapshot đã pin](https://github.com/milind-soni/OpenMausBot/tree/88d54eb17a3655b968470cbbf3b5097de225a8b5). Nghiên cứu shell, rail, lịch, files, approvals, MCP và computer trước đó ở [báo cáo 2026-10-30](openmaus-interface-2026-10-30.md) vẫn có hiệu lực trong phạm vi đã ghi; lượt này tập trung vào AI và vòng đời hội thoại.

## Hợp đồng kết nối

| Provider | Nguồn danh sách | Đường chat qua Neko Core |
|---|---|---|
| GLM Coding Plan | `/api/coding/paas/v4/models` | Adapter Anthropic `/api/anthropic/v1/messages` |
| GLM API | `/api/paas/v4/models` | OpenAI-compatible chat completions trên cùng base API |
| ChatGPT | Catalog tài khoản đăng nhập của Core | Responses hoặc Codex theo metadata/khả năng hỗ trợ của model |
| OpenAI API | `/v1/models` | Chat Completions; lọc các loại model hiện không chạy được trên route này |
| Claude API | `/v1/models`, cursor `after_id` | Anthropic Messages |
| Gemini API | `/v1beta/models`, `pageToken`, chỉ model có `generateContent` | OpenAI-compatible chat completions của Gemini |
| DeepSeek API | `/models`, loại model không xuất văn bản nếu có metadata | OpenAI-compatible chat completions |

Z.AI phân biệt endpoint Coding Plan với API thông thường; không fallback qua route tính phí khác. Xem [ZCode Connect Models](https://zcode.z.ai/en/docs/configuration). Native Gemini catalog có tên và khả năng tạo nội dung cùng pagination; xem [Google Models API](https://ai.google.dev/api/models). Claude dùng danh sách có cursor; xem [Claude List Models](https://platform.claude.com/docs/en/api/models/list). Metadata DeepSeek theo [List Models](https://api-docs.deepseek.com/api/list-models/). OpenAI liệt kê nhiều loại model; danh sách chat của Aurora là tập con phù hợp transport hiện có, không phải toàn bộ `/models`: [OpenAI List Models](https://developers.openai.com/api/reference/resources/models/methods/list).

1. Chọn provider, dán khóa hoặc đăng nhập ChatGPT.
2. Tải danh sách từ chính kết nối đó. Khóa đang nhập chỉ phục vụ request catalog, chưa lưu và chưa đổi AI.
3. Chọn model rõ ràng rồi lưu. API routes tải lại catalog với đúng khóa để phát hiện model đã mất quyền giữa lúc xem và lưu.
4. Quay lại draft và gửi khi sẵn sàng. Core kiểm tra catalog trước khi gọi model; lỗi có hướng xử lý, không fallback sang provider khác.

Catalog được giới hạn kích thước, số trang, số model và thời gian; chặn redirect. Cache API chỉ ở bộ nhớ, gắn fingerprint của khóa hiện lưu. Đổi khóa vô hiệu hóa danh sách cũ. Request lỗi không tự thay key/selection. Khóa truyền qua environment riêng của child process, không argv hoặc URL; không ghi raw lỗi của provider vào phản hồi công khai.

Snapshot AI của lượt nằm trong bộ nhớ và không journal khóa. Sau restart/resume, lượt có thể dùng cấu hình hiện tại theo cơ chế recovery hiện có; đây không phải bảo đảm pin credential xuyên restart. Login ChatGPT vẫn do Core quản lý, không đóng băng token hay cản refresh token hợp lệ.

Catalog thành công chứng minh lấy được metadata bằng kết nối đó; chưa chứng minh quota đủ hay mọi request tạo nội dung sẽ thành công. Route custom trong cấu hình nâng cao vẫn do người dùng khai báo endpoint/model; nó không được gắn nhãn là catalog tài khoản đã tự khám phá. Provider khác chưa có route thiết lập nhanh không nằm trong bảy route của đợt này.

## Kiểm chứng và giới hạn

Xem [nhật ký kiểm chứng](provider-flow-validation-2026-11-01.md). Giữ Tauri/Rust/Go/Core, kho bí mật, permission và receipt contracts. Chưa thêm engine của OpenMausBot, auto-routing, computer backend, trigger backend hoặc đồng bộ cloud mới. Không gọi completion thật bằng tài khoản người dùng, không đổi provider/credential của họ, không push/deploy. Kiểm thử CLI thật dùng mạng giả lập; kiểm thử viewport không thay thế thiết bị vật lý.
