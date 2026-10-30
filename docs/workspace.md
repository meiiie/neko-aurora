# Không gian làm việc Aurora

Trên desktop, Aurora có thanh điều hướng bên trái, danh sách bot ở cạnh hội thoại và trang tổng quan. Chọn bot để tiếp tục chat; bản nháp của từng bot vẫn được giữ khi chuyển qua lại.

## Đi đến công việc

- Dùng thanh bên để mở tổng quan, lịch chạy, Pages, kết nối ứng dụng, Vault, giao tiếp agent hoặc cài đặt. Di chuột hay đưa focus lên biểu tượng để xem tên.
- Nhấn **Ctrl+K** trên Windows/Linux hoặc **Cmd+K** trên macOS để tìm bot và hành động. Có thể gõ không dấu tiếng Việt, dùng mũi tên để chọn, Enter để mở và Escape để đóng.
- Dùng **Chưa đọc**, **Đang làm**, **Cần bạn** để lọc danh sách. Các số đếm lấy từ trạng thái bot trong workspace hiện tại.
- Nút ở cuối thanh điều hướng thu gọn hoặc mở lại danh sách. Lựa chọn được giữ trên trình duyệt này sau reload.

## Hội thoại và ngữ cảnh

Thanh trên hội thoại có lối vào **AI & công cụ**, **Bộ nhớ** và **Máy tính**. Bấm tên bot để mở hồ sơ. Từ 1280px, hồ sơ nằm cạnh hội thoại; trên desktop hẹp hơn, hồ sơ dùng vùng nội dung chính. Chuyển bot sẽ đóng hồ sơ bot trước đó.

Vị trí đang đọc được giữ khi đổi chiều rộng hoặc thu gọn danh sách. Các thao tác gửi, phê duyệt, lưu file và thử lại vẫn dùng luồng xác nhận hiện có.

## Kết nối AI nhanh

Trong hội thoại, chọn **Thiết lập AI**, hoặc mở **AI & công cụ → Thiết lập nhanh**. Chọn **GLM · Coding Plan** hoặc **GLM · API** đúng với loại khóa đang dùng, dán API key rồi bấm **Lưu và dùng**. Với **ChatGPT**, chọn **Đăng nhập ChatGPT**, hoàn tất trong trình duyệt rồi quay lại bấm **Dùng kết nối này**. Có thể hủy hoặc đăng nhập lại.

Mở thiết lập từ bot sẽ chọn AI cho bot đó; mở từ cài đặt chung sẽ đặt mặc định cho bot chưa có model riêng. Khóa provider được lưu trong workspace. Các trường endpoint, provider tùy chỉnh và model đọc ảnh nằm ở **Cấu hình nâng cao**. Model, preset và đăng nhập dùng Neko Core đã có trên máy; không cần sửa biến môi trường để dùng luồng này. Tin nhắn cũ không tự gửi lại. [Kiểm thử và giới hạn](research/quick-ai-setup-validation-2026-10-30.md).

## Xem lịch của các bot

Mở **Lịch chạy** từ thanh điều hướng hoặc đầu danh sách bot. Trang này tập hợp lịch hiện có, cho phép lọc bot và hiển thị giờ/múi giờ hoặc khoảng lặp, cùng trạng thái bật, tạm dừng hay cần kiểm tra.

Chọn **Quản lý lịch** để mở hồ sơ đúng bot. Tạo, sửa, tạm dừng, tiếp tục và kiểm thử lịch vẫn nằm ở luồng quản lý lịch hiện có; tổng quan không tự chạy lịch. Sau khi sửa, mở lại tổng quan hoặc bấm **Tải lại** để lấy trạng thái mới. [Cách dùng khoảng thời gian và giới hạn](routines.md).

Nếu tải lỗi, dùng **Tải lại**. Khi đổi workspace, dữ liệu lịch cũ được xóa khỏi màn hình. Trang tổng quan hiện đọc theo yêu cầu; chưa có calendar, webhook hoặc đồng bộ lịch giữa nhiều máy chủ.

## Màn hình nhỏ và giao diện

Dưới 900px, Aurora giữ cách dùng một màn hình tại một thời điểm: danh sách → hội thoại → hồ sơ. Lịch chạy có nút quay lại riêng. Theme và ngôn ngữ tiếp tục đổi trong cài đặt. Tên bot và nội dung người dùng không bị dịch tự động.

[Nghiên cứu OpenMausBot và quyết định thiết kế](research/openmaus-interface-2026-10-30.md) · [Kiểm thử và giới hạn](research/openmaus-validation-2026-10-30.md)
