# Lịch hẹn theo khoảng thời gian

Mở hồ sơ bot, đến phần lịch hẹn rồi chọn **Sửa lịch**. Trong **Lặp lại**, chọn **Theo khoảng thời gian** và nhập số phút nguyên từ 15 đến 10080. Ví dụ: 60 phút cho mỗi giờ, 1440 phút cho mỗi 24 giờ. Bấm **Lưu lịch** để áp dụng.

Lượt đầu đến hạn sau một khoảng tính từ lúc tạo lịch hoặc đổi số phút. Chỉ sửa nội dung, tạm dừng hay tiếp tục sẽ giữ mốc ban đầu. Sau thời gian máy dừng hoạt động, Aurora chỉ nhận một lượt đến hạn gần nhất; không chạy dồn tất cả lượt đã bỏ lỡ.

Đây là khoảng thời gian thực trôi qua. Nếu cần chạy vào 8 giờ sáng theo địa phương, dùng **Mỗi ngày** với giờ và múi giờ. Khoảng 1440 phút có thể lệch một giờ theo đồng hồ địa phương khi chuyển giờ mùa hè.

Lịch được kiểm tra định kỳ mỗi 30 giây. Thời điểm bắt đầu tác vụ còn phụ thuộc máy đang hoạt động, hạn mức và hàng đợi. **Tạm dừng** ngăn lượt mới và huỷ lượt còn chờ host; tác vụ đã chạy cần được dừng trong hội thoại như trước.

Nếu lưu lỗi, nội dung chỉnh sửa vẫn còn. Dùng **Tải lại lịch** để kiểm tra phiên bản máy chủ trước khi thử lại. Khi thiết bị khác đã sửa lịch, Aurora từ chối ghi đè phiên bản cũ.

Bản phát triển cũng hướng dẫn model tạo lịch khoảng khi người dùng yêu cầu rõ, nhưng kiểm thử hiện tại dùng dữ liệu fixture; chưa đánh giá việc model thật hiểu chính xác mọi câu yêu cầu. Sau khi tạo, hãy kiểm tra lịch trong hồ sơ bot. Tính năng chưa có webhook hay lịch đồng bộ giữa nhiều máy chủ.

[Bằng chứng kiểm thử và giới hạn](research/grokbot-schedule-interval-2026-10-29.md)
