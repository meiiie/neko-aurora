# Chọn AI cho Aurora và từng bot

[← Workspace](workspace.md)

Nhấn tên AI cạnh ô soạn tin, hoặc vào **AI của bot** trong hồ sơ. Cấu hình ở đây áp dụng cho bot đang mở. Muốn đặt AI mặc định cho các bot chưa chọn riêng, vào **Thiết lập → Mô hình & nhà cung cấp**.

## Kết nối nhanh

1. Chọn **GLM · Coding Plan**, **GLM · API**, **ChatGPT**, **OpenAI · API**, **Claude · API**, **Gemini · API** hoặc **DeepSeek · API**.
2. Với API, dán khóa rồi nhấn **Tải danh sách model**. Với ChatGPT, nhấn **Đăng nhập ChatGPT**, hoàn tất trên trang đăng nhập rồi quay lại Aurora.
3. Chọn một model trong danh sách và nhấn **Lưu và dùng** hoặc **Dùng kết nối này**.
4. Nhấn **Quay lại trò chuyện**. Bản nháp được giữ nguyên; gửi tin nhắn khi bạn sẵn sàng.

Khi bot rảnh, tên provider và model hiện dưới tên bot và cạnh composer. Khi đang chạy, header hiện model của lượt đó; nút cạnh composer vẫn hiện lựa chọn cho lượt tiếp theo. Khi tên hiển thị của model khác ID, menu hiện cả hai để dễ phân biệt. **Theo mặc định Aurora** nghĩa là bot chưa đặt model riêng. Model đang chọn là cho lượt tiếp theo; lượt đang chạy giữ nguyên AI cho đến khi hoàn tất. Đổi model không tự gửi lại tin đã lỗi.

GLM Coding Plan và GLM API là hai kết nối riêng. Chọn đúng loại khóa/gói; Aurora không tự chuyển từ gói Coding Plan sang API tính tiền theo mức sử dụng. Khóa API lưu trong kho bí mật trên máy chạy Aurora và dùng chung cho các bot chọn provider đó; trường khóa không đọc ngược khóa đã lưu. Để trống trường này để dùng khóa đã lưu.

## Khi không tải hoặc không dùng được model

- **Chưa tải danh sách:** nhập khóa hoặc đăng nhập, rồi tải model. Aurora không chọn thay bằng một model mặc định đoán sẵn.
- **Khóa đã thay đổi:** tải lại danh sách bằng khóa mới trước khi lưu. Tải danh sách chưa lưu khóa và chưa đổi AI.
- **Model không còn trong danh sách:** lựa chọn cũ vẫn hiện để bạn nhận ra; tải lại và chọn model khác. Không tự fallback.
- **Khóa chưa được chấp nhận:** kiểm tra khóa, provider và loại gói. Với ChatGPT, có thể cần đăng nhập lại.
- **Lỗi mạng/quota/quyền:** thông báo chỉ rõ nhóm lỗi và đưa đường về thiết lập AI. Tải được danh sách model chưa đảm bảo còn quota cho mọi yêu cầu tạo nội dung.
- **Chưa có Neko Core:** thiết lập nhanh cần Core có sẵn trên máy chạy Aurora; xem [hướng dẫn build](getting-started.md). Cấu hình nâng cao vẫn dành cho endpoint và model tự khai báo.

Danh sách nhanh chỉ chứa model văn bản phù hợp với transport hiện được Aurora hỗ trợ. Nó có thể nhỏ hơn danh sách tất cả model của nhà cung cấp (ví dụ embedding, ảnh hoặc audio). Các kết nối custom trong cấu hình nâng cao vẫn dùng danh sách bạn khai báo, chưa có cơ chế tự khám phá như bảy route trên.

Xem [nghiên cứu và hợp đồng provider](research/provider-flow-openmaus-2026-11-01.md) và [phạm vi kiểm thử](research/provider-flow-validation-2026-11-01.md).
