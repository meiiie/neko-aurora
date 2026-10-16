# Candidate auth-v2 sau khi sửa admission song song

Bản nên thử trong đợt này là `E:/CodexScratch/core-auth-concurrency-20261012/candidate-15565e8`, cổng **8767**, state riêng. Code app đã kiểm: `15565e89dd22d8cda959b2e58c7463464c333d80`; delivery chỉ thêm tài liệu này. Core: `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`. Commit delivery, binary hashes và bằng chứng nằm trong `release-summary.json` và `artifact-manifest.json`. Các candidate cũ được giữ làm lịch sử; chúng không phải lựa chọn được đề xuất hiện tại. Candidate mới chưa chạy. Live8765 và Core đã cài không thay đổi.

## Lỗi đã sửa và bằng chứng

Lượt Kimi cũ `5987ed1` có request thứ hai nhận `host_rejected` trước inference. Giữ nguyên journal, executor state và lượt FAIL; saved lease không bị retire và các binding/replay predicates vẫn hợp lệ. Capture chẩn đoán riêng trên `aa59fed` tái hiện private Verify hết deadline trong khi Work vẫn current, store không closed/poisoned: catalog command bị hủy sau 88 ms, catalog tổng 488 ms. Log này không đo trực tiếp thời gian chờ mutex. Đối chiếu code cho thấy caller bắt đầu deadline ba giây trước khi chờ mutex nguồn, rồi các watchdog/Bind/Admit/receipt lần lượt đọc và hash Core. Kiểm thử giữ mutex tái hiện caller hết hạn vẫn bị chặn ở bản cũ (RED).

Bản sửa chia sẻ **lần đọc metadata đang chạy** của cùng nguồn/generation. Caller tự quan sát hủy/deadline; hủy caller đầu không hủy lần đọc của caller còn sống. Producer có context riêng ba giây. Kết quả được copy và bỏ khỏi flight khi hoàn tất: lần Verify tiếp theo phải đọc Core mới. Không cache catalog đã hoàn tất, không capture môi trường, refresh token, rebind pin hay tạo quyền. Inspection vẫn serialize; kiểm tra context sau khi chờ trước capture. Generation/snapshot/expiry và tuple profile/auth/origin gốc vẫn phải khớp, cùng các chốt current/closed/poisoned của store. Context không cưỡng chế được mutex hay I/O hệ điều hành; producer kiểm tra hết hạn sau khi lấy mutex trước khi đọc catalog.

| Gate | Code và phạm vi | Kết quả |
| --- | --- | --- |
| Go test/vet/build mới | Clean `15565e8`, default build | 500 top-level + 502 subtests PASS; DOCX reference và browser seed tùy chọn SKIP |
| Review độc lập | Git objects `15565e8`, synthetic callbacks | 6 top-level + 3 subcases PASS; không có P1/P2 mới được xác nhận trong phạm vi |
| Bốn service native chạy song song | Default Go `15565e8`, Rust host/Core đã pin; mỗi service 16 admission lặp thêm | Env 41, cached Kimi 38, file key 35, no-key 35: **149 PASS**, gồm 401/revoke/deadline/restart và genuine Core HANDLE |
| Một production manager | Hai bot × foreground/background, bốn Core cùng live và request thứ năm | **5 PASS**; ACK thứ năm 371.674 ms trong fixture này; cả năm hoàn tất một lần, config/token cache không đổi |

Đây là closure hữu hạn của lỗi private Verify/admission ở cửa sổ đã tái hiện, không phải cam kết mọi mức tải hay SLO. Toàn bộ lượt FAIL/chẩn đoán/RED giữ nguyên, không đổi thành PASS của lượt khác. Các service fixture đã dừng. Chỉ dùng credentials tổng hợp và loopback; không đọc profile, key, token hoặc provider thật. Host/Core/UI không đổi trong bản sửa concurrency: Rust33, Core282/2299 assertions + 9 compiled probes, UI40 là bằng chứng kế thừa theo hash/tree, không gọi là lượt chạy mới.

## Cách tự thử

1. Trong thư mục candidate mới, chạy `Start-CoreProfile-Candidate.ps1 -CheckOnly` để kiểm hashes mà chưa mở service. Khi quyết định thử, mở `Start-CoreProfile-Candidate.cmd`, rồi vào `http://127.0.0.1:8767`. Launcher từ chối cổng có chủ; không gắn vào service8765. Dừng CLI của candidate khi xong. Desktop hiện gắn8765 nên candidate này dùng browser.
2. Mở **Kết nối → Dùng profile có sẵn**, chọn nguồn rồi **Cho phép xem profile · 5 phút**. Đây mới là consent để Core xem metadata và credential source của tài khoản Windows. Không gửi key/token vào chat.
3. Chọn profile được hỗ trợ. File key/no-key, profile `key_env` do Core chỉ định và cached Kimi còn hạn được nối; Kimi dùng token cache, không login/refresh. Kiểm tra loại nguồn và deadline hiệu dụng.
4. Ở **Phạm vi**, kiểm bot/kênh/dự án, dữ liệu có thể gửi và ngân sách; chọn **Cho phép dùng trong phạm vi này**. Consent chưa gọi model. Gửi request mới mới có thể gọi provider và phát sinh phí.
5. Thu hồi quyền để dừng việc dùng nó. Thay snapshot/profile/token, hết hạn hoặc restart không tự khôi phục quyền cũ. Với token Kimi hết hạn/401, tự quản lý credential ở Core; app không đăng nhập hay làm mới hộ.

## Những gate còn lại

Native Tauri/Wiii owner dialog chưa được nghiệm thu: công cụ điều khiển native hiện không khả dụng và shell hiện gắn8765. Phone thật, bàn phím/IME/safe-area/AT speech, provider/model thật và device/cloud vẫn chưa nghiệm thu. Chưa drill native đủ 300 giây riêng cho capsule expiry; các deadline request ngắn đã thử thật. OAuth renewal/provider OAuth khác và Wiii lock/catalog cũ chưa có bridge-v2 đầy đủ; không mở rộng trong bản concurrency này.

Packet chứa source patch, local Git bundle, binary đã pin và public receipts. Không chứa raw journal, source homes, capsule/key/token, private state hoặc browser profiles. Source Main chỉ fast-forward sau kiểm/scan; ba raw-byte edits cũ, 17 ZIP, 1509 protected files và installed Core hash phải được giữ theo receipt tích hợp.
