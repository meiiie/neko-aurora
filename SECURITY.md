# Bảo mật

## Phạm vi hỗ trợ

Neko Bot hiện là development snapshot, chưa có stable release hoặc bảng phiên bản nhận security updates được cam kết. `main` là nhánh phát triển; WIP/archive không được coi là bản an toàn để triển khai. Không có SLA phản hồi, patch hoặc disclosure được hứa.

## Báo cáo lỗ hổng

Tại lần rà repo ngày 2026-10-16, **GitHub private vulnerability reporting chưa được bật** và chưa có email/kênh security riêng được xác minh cho repo này. Vì vậy tài liệu này không bịa một địa chỉ hoặc hứa một private inbox đang hoạt động.

- Không đăng token, credential, raw state/journal, chat riêng, exploit có dữ liệu thật hoặc thông tin người dùng vào issue, PR hay Discussions công khai.
- Nếu cần báo cáo riêng tư, có thể mở một issue **chỉ yêu cầu maintainer cung cấp kênh báo cáo bảo mật**, không nêu chi tiết lỗ hổng hoặc dữ liệu ảnh hưởng. Chờ một kênh riêng được maintainer xác minh trước khi chia sẻ bằng chứng nhạy cảm.
- Chỉ dùng [GitHub Security](https://github.com/meiiie/neko-bot/security) để báo riêng khi tính năng Report a vulnerability thực sự xuất hiện/được bật. Link tới tab Security không đồng nghĩa private reporting đã hoạt động.
- Nếu không có kênh riêng, giữ bằng chứng cục bộ thay vì đăng công khai hoặc gửi tới một địa chỉ suy đoán.

Khi đã có kênh phù hợp, báo exact commit, môi trường, component, hành vi mong đợi/thực tế, impact và reproducer tối thiểu dùng fixture. Không cần gửi secret thật để chứng minh một boundary lỗi.

## Quy tắc cho người phát triển

Giữ ranh giới loopback, owner/scope, revision/nonce, context/resource pin, runtime admission, process-stop và effect receipt. Không bypass fail-closed behavior để chạy demo. Build mới hoặc receipt được chấp nhận không đồng nghĩa có quyền thực thi/provider/file mới. Không thử tấn công dịch vụ, tài khoản hoặc dữ liệu của người khác.

Nếu vô tình public secret thật, ngừng chia sẻ nó, thu hồi/rotate tại nhà cung cấp và thông báo qua kênh an toàn; xóa khỏi file hiện tại không xóa khỏi Git history/cache. Không force-push hoặc xóa bằng chứng của người khác khi chưa có chỉ dẫn phù hợp. [License/NOTICE](README.md#attribution-và-giấy-phép) không phải bảo đảm an toàn hoặc chứng nhận production.
