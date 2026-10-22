# Candidate Kết nối: phần local đã chốt

Candidate mới: `E:/CodexScratch/core-profile-bridge-20261012/candidate-92b442d`. App đã kiểm: `92b442de74d67df63ab26585cc26c80887a13ec8`. Core giữ nguyên `0aa0771ba47e6803018297a01d51427c9e61283f`, binary `81e5facc66453fe15cfc98159b395f45bcfffb39980d724ade51d677eaca10ae`. Delivery sau code được ghi trong `release-summary.json`; không dùng số commit của docs để relabel test.

## Cách thử tối thiểu

1. Mở `Start-CoreProfile-Candidate.cmd` trong thư mục candidate, rồi vào `http://127.0.0.1:8767`. Script dùng state riêng và từ chối nếu cổng đã có chủ; không dừng bản 8765. Có thể chạy `.ps1 -CheckOnly` trước để chỉ kiểm hash, chưa mở dịch vụ.
2. Mở **Kết nối → Dùng profile có sẵn**. Bước **Nguồn** chỉ liệt kê nguồn đã được native cấu hình; chưa đọc profile hay gọi model.
3. Chọn **Cho phép xem profile · 5 phút**. Đây là thao tác tự đồng ý để Core đọc metadata ở home Core của tài khoản Windows. Không cần nhập key vào chat hay Neko Aurora. Chọn profile đủ điều kiện ở bước **Profile**.
4. Ở bước **Phạm vi**, xem hội thoại/bot/dự án, provider và dữ liệu được gửi. Ngân sách mặc định nhìn thấy: mỗi tin tối đa 64.000 byte đầu vào, 1.024 token đầu ra, 1 lượt gọi, 1 bước, 120 giây; quyền gốc 300 giây. **Chỉnh giới hạn** mở editor. Bấm **Cho phép dùng trong phạm vi này** mới tạo consent; thao tác đó chưa gọi model.
5. Gửi một tin mới khi bạn muốn thử provider thật. Việc gửi là bước dùng credential và gọi provider, có thể phát sinh chi phí. Lượt phát triển này chưa thực hiện bước đó với profile thật.
6. Trong Kết nối, chọn **Thu hồi quyền dùng profile đã chọn** để dừng việc đang dùng phạm vi đó. Đổi key/profile hoặc hết hạn cũng giữ hàng rào output/effect; retry không tự mở lại quyền.

Banner phạm vi đã cấp giữ ngân sách gốc. Chỉnh các giá trị trong form chỉ chuẩn bị một yêu cầu mới; không âm thầm đổi consent đang dùng. Khóa và config gốc do Core đọc, bộ nhớ/task/artifact nằm ở home task riêng. Core đã cài và state/binary live cũ được bảo toàn.

## Mốc bằng chứng

- Main source đã tích hợp bridge `28293c9`; delivery UX tiếp theo được fast-forward sau khi kiểm, giữ ba edits cũ. Binary/state đang phục vụ 8765 không được thay thế.
- UX `92b442d`: 58/58 Chrome fixture checks sạch, bao gồm 50 scope/retry/session cases cũ và 8 bước/caps/focus/motion cases mới. Targeted Go/build PASS.
- Journey thực `92b442d`: 14/14 với Go chứa đúng bốn assets, Rust/Core thật, **profile/key/model loopback giả lập**. Đây không phải profile/provider thật.
- Native bridge 24/24 ở `bc10baf` được giữ đúng provenance. Logic Go authority/runtime, Rust/desktop và Core không đổi trong UX slice; không chạy lại broad audit hay relabel nó thành native `92b442d`.
- Full Go 484 top/479 sub, Rust30, Core257/2003 assertions và Tauri build ở các mốc đã ghi trong packet bridge cũ vẫn là bằng chứng lịch sử có phạm vi, không thành nghiệm thu phone/native window mới.

## Ranh giới kết thúc

Phần local của lát cắt này đủ khi nguồn Main, candidate độc lập và receipts/hashes khớp: reuse profile API key trong file/no-key có inspection, explicit execution, ngân sách, revoke, durable history và UI dùng được. Không tự mở candidate với nguồn thật, không tạo credential grant thật, không login/refresh, deploy hay push.

**OAuth và key chỉ ở environment chưa hỗ trợ.** Đây còn thiếu CODE/broker/policy, không phải chỉ thiếu một lần test. Map exact Core0aa chỉ ra refresh-origin/budget, token lease/rotation CAS, output protection và Core-owned env source còn thiếu; bật transport hoặc truyền ambient environment sẽ hạ boundary. Cached Kimi no-refresh là đề xuất contract khác, chưa triển khai hoặc quảng bá.

Backlog ngoài lát cắt được ghi hữu hạn trong `ux-connections-profile-steps-20261012.md`: avatar/memory controls, richer retrieval/planner, active/project controls, PDF/parser isolation, portable restore/retention/installer, remote identity/connector/cloud/phone code. Không coi những phần ấy là xong vì fixture xanh. Profile/provider thật, chất lượng model, native Tauri/Wiii interaction, phone/keyboard/safe-area/AT cần bằng chứng và hành động user riêng.
