# Khôi phục và lưu checkpoint

[← Tài liệu](README.md)

GitHub lưu các commit đã được đẩy thành công. Máy phát triển có thể được thay thế
bằng một checkout mới, nhưng file chưa commit hoặc commit chưa push vẫn có thể mất.
Git không phải bản sao lưu tài khoản, chat, state hoặc build artifact.

## Khôi phục trên máy mới

```sh
git clone https://github.com/meiiie/neko-bot.git
cd neko-bot
git fetch origin --tags
git status --short
git log -1 --oneline origin/main
git branch -r
```

Đọc AGENTS.md và hướng dẫn component trước khi build. Đối chiếu SHA với checkpoint
đã bàn giao, không chỉ dựa vào tên nhánh. Tag `v0.1.0-alpha.1` là mốc source,
không phải installer hoặc bằng chứng sản phẩm hoàn chỉnh.

Các nhánh `wip/*` giữ công việc chưa hoàn tất; `archive/*` giữ snapshot cũ.
Không merge toàn bộ nhánh cũ vào main chỉ để lấy vài file: tài liệu và source ở
nhánh đó có thể cũ hơn. Chọn thay đổi có phạm vi rõ, kiểm thử rồi review.

## Lưu một mốc phát triển

1. Kiểm tra `git status --short` và diff; giữ nguyên thay đổi của người khác.
2. Chọn file source cụ thể. Không dùng `git add .` khi chưa kiểm tra toàn bộ cây.
3. Quét secrets, xem xét mọi finding mà không in giá trị nhạy cảm. Không đưa
   credentials, state/journal, dữ liệu chat, cache, binary hay reference bundle
   không có quyền phân phối vào commit.
4. Chạy các gate phù hợp và ghi rõ PASS, FAIL hoặc chưa chạy. Fixture không thay
   kiểm thử provider thật, Linux không thay DPAPI/desktop acceptance trên Windows.
5. Commit lên nhánh phát triển có tên rõ; chỉ push khi được chủ repo cho phép.
   Công việc chưa đạt acceptance vẫn có thể lưu ở nhánh WIP với trạng thái trung thực.
6. Sau push, đọc lại SHA trên remote và đối chiếu với commit vừa lưu. Chỉ khi khớp
   mới gọi mốc đó là đã sao lưu trên GitHub. Không force-push hoặc di chuyển tag
   đã công bố để thay thế lịch sử.

Mỗi lần bàn giao nên có nhánh/SHA, mục tiêu đã đạt, lệnh kiểm thử và môi trường,
phần còn thiếu, và bước tiếp theo. Không lưu key hoặc đường dẫn kho bí mật trong
bản bàn giao công khai.

## Phạm vi dữ liệu

- Source đã push: khôi phục bằng Git trên máy mới.
- Dependency/build cache: tái tạo từ lockfile và toolchain được chỉ định.
- Wiii: thư mục trong repo là overlay, cần base đúng; không ghi đè checkout đang dùng.
- Tài khoản và chat: không nằm trong repo; cần quy trình backup riêng đã được người
  dùng chấp thuận. DPAPI gắn với môi trường Windows, không giả định copy file sang
  Linux là giải mã được.
- Mất máy không được xử lý bằng cách bỏ qua kiểm tra hash, identity hoặc quyền.

Đọc [setup](getting-started.md), [phạm vi kiểm chứng](project-status.md) và
[bảo mật](../SECURITY.md) trước khi chạy trên môi trường mới.
