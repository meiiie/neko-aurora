# Giao tiếp agent trong Aurora

Aurora có danh bạ phiên và hộp thư bền vững trong cùng workspace. Mở **Giao tiếp agent** ở đầu danh sách bot, chọn đúng phiên riêng hoặc nhóm, nhập nội dung rồi chọn cách gửi:

- **FYI** lưu thông tin, không tạo tác vụ hoặc gọi model. Thông tin được ghép vào lượt hợp lệ tiếp theo của bot nhận.
- **Đánh thức agent** tạo một tác vụ qua scheduler hiện có; có thể phát sinh lượt model. Đánh thức từ một FYI của bạn tạo thư mới, giữ nguyên thư gốc.
- **Đánh dấu đã xem** chỉ ghi nhận người dùng đã đọc. Nó không xác nhận model đã nhận hoặc công việc đã hoàn tất.

Hộp thư hiển thị riêng trạng thái giao tin và trạng thái tác vụ. `completed` là trạng thái Work của Aurora, không phải biên nhận thành công của một dịch vụ bên ngoài. Nếu có đề xuất cần duyệt, mở thẻ duyệt hiện có trong cuộc trò chuyện. Văn bản của agent không tạo quyền thực thi hoặc quyết định của con người.

## Phạm vi và nguồn

Danh bạ dùng ID phiên ổn định theo journal, bot, cuộc trò chuyện và thiết bị; đổi bí danh không đổi đích của thư đã gửi. Adapter hiện tại là `server` qua HTTP đăng nhập/same-origin. Nó dùng runner sẵn có, không thêm một vòng suy luận khác.

Agent đang chạy có bốn công cụ: `network_list_sessions`, `network_send_message`, `network_check_inbox`, `network_message_status`. Chúng chỉ hoạt động trong nhóm chung và revision đang được công việc giữ. Thư từ agent giữ nguồn `agent`, hiển thị tên người gửi và không được đưa vào luồng điều khiển của chủ tài khoản. Tác vụ nhận thư agent không được đọc bộ nhớ/file riêng, thay đổi bot, dùng bí mật hoặc tạo hành động chỉ người dùng mới được yêu cầu.

Một gốc công việc được gửi tối đa bốn thư agent; mỗi cặp gửi/nhận tối đa 12 thư trong 10 phút. Ngân sách lấy từ journal nên khởi động lại không đặt lại giới hạn. Nonce gửi lại cùng nội dung trả về thư cũ; dùng lại nonce với nội dung khác bị từ chối. Liên kết thư đánh thức với Work nằm trong cùng record nhận việc.

## File và lưu trữ

File đính kèm là văn bản UTF-8: tối đa tám file, 32 KiB mỗi file và 64 KiB tổng cộng. Nội dung thư tối đa 16 KiB. Tên ẩn, đường dẫn, tên trùng không phân biệt hoa thường và ký tự điều khiển bị từ chối. File tải về theo scope của thư, dưới dạng văn bản đính kèm. Không gửi khóa hoặc bí mật qua hộp thư; tính năng này không bổ sung mã hóa riêng cho journal.

Thư chưa giao hết hạn sau bảy ngày. Hết hạn không xóa lịch sử. Khôi phục journal giữ nguyên ID, liên kết Work, FYI đã ghép ngữ cảnh và dấu đã xem. Với hosted quota, một wake được giữ chỗ trước khi nhận Work; giữ chỗ có thể bảo thủ nếu tiến trình dừng giữa hai bước. FYI và yêu cầu gửi lại không bị tính như một wake mới.

## Kiểm tra từ source

Trong `app/native`, chạy:

```powershell
go test -count=1 -run 'Test.*Network|TestCancelledNetwork|TestHoldAll' ./internal/ledger ./internal/aurora ./internal/hub ./internal/identity
go vet ./...
go build -o ../.build/neko-aurora-network.exe ./cmd/neko-aurora
```

Tại root, chạy `node app/scripts/verify_network.mjs`. Script dùng tiến trình Go thật, state riêng và model fixture trên loopback; không dùng tài khoản provider. Kết quả nằm trong `app/.build/network/acceptance.json`. Thêm `--serve` để giữ ứng dụng fixture cho kiểm tra giao diện, rồi dừng tiến trình khi xong.

## Nguồn tham khảo và giới hạn

Thiết kế học từ [Reason Network](https://github.com/reason-machines/reason-network) tại commit `5e995a580520d95e7fa130b3a2d1915364f2d746`: định danh phiên, hộp thư bền vững, retry, capability, nguồn thông tin và phân biệt giao tin với hoàn tất tác vụ. Phần triển khai Aurora dùng ledger và runtime của Aurora; không chép mã nguồn Reason.

Bản này chưa liên kết các workspace độc lập qua P2P, chưa có iroh relay, adapter Codex/Claude/MCP hay đồng bộ nhiều máy. Không chạy `reason-network setup` hoặc thay đổi cấu hình agent toàn máy. Fixture loopback không chứng minh chất lượng provider, Railway hay thiết bị vật lý. Xem [kế hoạch và acceptance](../../docs/superpowers/plans/2026-10-25-agent-network.md) để biết phạm vi kiểm chứng.
