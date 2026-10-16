## Thay đổi và lý do

Nêu vấn đề cụ thể, behavior trước/sau và component bị ảnh hưởng. Liên kết issue nếu có.

## Kiểm chứng

| Exact commit | Command / môi trường | PASS / FAIL / BLOCKED | Scope / giới hạn |
|---|---|---|---|
| | | | |

Với thay đổi nhìn thấy được, thêm ảnh có nhãn phiên bản và dữ liệu fixture, hoặc giải thích vì sao không áp dụng. Không coi syntax check là UI test hay viewport là physical-phone acceptance.

## Checklist

- [ ] Phạm vi tập trung; không sửa code/lock/license không liên quan.
- [ ] Không có secret, account store, state/journal, chat thật hoặc bundle không được phép phân phối.
- [ ] Bằng chứng đúng commit; failure/blocker không bị trình bày thành PASS.
- [ ] Giữ attribution/license/provenance và ghi rõ material bên ngoài nếu có.
- [ ] Đã đọc CONTRIBUTING.md; vấn đề bảo mật theo SECURITY.md thay vì đăng chi tiết công khai.

## Rủi ro hoặc phần chưa kiểm chứng

Nêu gate còn thiếu và cách rollback nếu thay đổi ảnh hưởng runtime/authority/state. Không tự hứa SLA hoặc nghiệm thu provider/cloud/native.
