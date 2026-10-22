# Roadmap

[← Tài liệu](README.md) · [Trạng thái](project-status.md)

Checklist này theo dependency và gate, không phải lịch phát hành hoặc cam kết hoàn thành theo ngày.

| Lát cắt | Hiện trạng | Gate để tiến tiếp |
|---|---|---|
| Bot/kênh/project và lịch sử local | Có source và tests local scoped | Giữ invariant scope/nonce/revision khi sửa |
| Durable control/recovery/effect | Có các contract và native/fixture evidence theo commit | Reproduce đúng failure window và pin; không suy exactly-once toàn hệ thống |
| Saved-account witness/Core bootstrap | WIP; valid Windows Issue test đầu tiên từng FAIL | Issue hợp lệ PASS, denial/rotation/revoke/replay có evidence; wiring CLI/runtime hoàn tất trước kích hoạt |
| Provider/account thật | Chưa nghiệm thu | Owner-authorized provider gate, không lộ secret, có hạn mức và evidence đúng model |
| Multi-device/cloud | Chưa hoàn tất | Pairing identity, ACL/destination, durable realtime sync và revoke/recovery trên nhiều máy |
| Thiết bị/AT | Viewport browser đã có tests, thiết bị thật còn thiếu | Keyboard/IME, safe area, lifecycle và AT speech trên thiết bị thật |
| Phát hành | Development source | Packaging/signing, rollback, user acceptance và support scope công bố rõ |

## Checklist tiếp tục

- [ ] Hoàn thiện witness trước khi gọi nó là runtime saved-account hoạt động.
- [ ] Nghiệm thu provider/model thật theo authorization/budget riêng.
- [ ] Ghép Wiii overlay chỉ trên base đã đối chiếu byte và kiểm lại native integration.
- [ ] Thiết kế/triển khai pairing + sync cloud và kiểm revoke/cross-device recovery.
- [ ] Nghiệm thu điện thoại/keyboard/AT thật.
- [ ] Tạo release được ký/kiểm tra và kế hoạch rollback/support phù hợp.

Theo dõi đề xuất/reproducer qua [Issues](https://github.com/meiiie/neko-aurora/issues). Checklist không tự tạo grant, mua dịch vụ, deploy hoặc cho phép gửi dữ liệu riêng tư. [CONTRIBUTING](../CONTRIBUTING.md) mô tả bằng chứng cần cho thay đổi; chưa có CI tự động ở root để thay các gate trên.
