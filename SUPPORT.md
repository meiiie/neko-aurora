# Hỗ trợ

Repo hiện có [GitHub Issues](https://github.com/meiiie/neko-bot/issues), chưa bật Discussions và chưa có kênh hỗ trợ riêng/SLA được công bố. Đây là source development, không phải dịch vụ được vận hành hoặc một installer stable đã nghiệm thu.

1. Đọc [setup](docs/getting-started.md) và [trạng thái hỗ trợ](docs/project-status.md).
2. Tìm issue hiện có; dùng mẫu bug hoặc feature request khi cần mở issue mới.
3. Ghi exact commit/branch, OS/toolchain, command và exit code. Dùng fixture tối thiểu, nêu expected/actual và gate nào PASS/FAIL/BLOCKED.
4. Với ảnh/log, chỉ chia sẻ dữ liệu synthetic đã kiểm tra; che thông tin không đủ nếu secret vẫn nằm trong file hoặc Git history. Không gửi account store, journal thật, chat, `.env`, token hay challenge.

Lỗ hổng và yêu cầu kênh riêng theo [SECURITY](SECURITY.md). Không dùng mẫu bug công khai để gửi exploit/dữ liệu nhạy cảm. Không có cam kết thời gian trả lời, hỗ trợ mọi nền tảng hoặc tương thích mọi provider.

Các trạng thái thường gặp: `waiting_host` là chưa có runtime được cấu hình; `stored_unverified` không phải account đã xác minh; viewport mobile không có nghĩa ứng dụng điện thoại native đã được phát hành. Wiii overlay thiếu full base nên không thể build standalone bằng source ở thư mục `wiii/`.
