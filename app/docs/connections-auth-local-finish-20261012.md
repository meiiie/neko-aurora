# Kết nối auth-v2: code local và phạm vi thử

App đã kiểm: `5987ed1ce0155c36332e1bc8d1b8e02ddd2d8f06`. Core: `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`, binary SHA256 `356247616082e613abd99b97838fd533f5e14ab64b07f480d78ae799d10648bf`. Candidate độc lập: `E:/CodexScratch/core-auth-bridge-20261012/candidate-5987ed1`. Delivery chỉ thêm tài liệu sau code; commit và hashes nằm trong `release-summary.json`. Candidate chưa khởi động; live8765 và Core đã cài giữ nguyên.

## Phần dùng được trong fixture

Kết nối vẫn có ba bước Nguồn, Profile, Phạm vi. Discovery chỉ liệt kê nguồn đã cấu hình. Sau inspection consent, Core đọc metadata và, với nguồn bật broker, chụp các environment key mà profile Core đã chỉ định vào capsule mã hóa. Go/Rust/browser nhận metadata và opaque binding; key không vào chat, model context hay browser storage. Intake/watchdog/receipt Verify không tự chụp hoặc làm mới environment.

File API key và no-key vẫn dùng được với pin v2. Cached OAuth hiện chỉ hỗ trợ Kimi, dùng access token còn hạn, không login/refresh hay viết lại token nguồn. Deadline hiệu dụng trừ 300 giây từ expiry của token; consent và từng request bị chặn thêm bởi ngân sách đã duyệt. HTTP401 giữ trạng thái không thành công `recovery_required`, dừng process, không tự gọi lại provider. Những OAuth khác hiển thị chưa hỗ trợ.

Inspection mới thay snapshot, dừng việc gắn với snapshot cũ và giữ pin/nonce/consent gốc. Hai GET cho cùng một inspection dùng chung một capture. Receipt lịch sử vẫn đọc được, nhưng snapshot đã thay không được báo còn active. Nguồn bật environment broker có generation mới mỗi lần service khởi động: lịch sử và ACK giữ nguyên, quyền dùng profile của nguồn ấy phải được tự đồng ý lại. Thay environment bên ngoài process đang chạy được quan sát ở inspection/restart tiếp theo; chưa có live ambient watcher.

## Cách thử sau khi người dùng quyết định

1. Trong thư mục candidate, chạy `Start-CoreProfile-Candidate.ps1 -CheckOnly` để kiểm hash mà chưa mở service. Khi muốn tự thử, mở `.cmd` cùng tên; dùng state riêng ở cổng8767 và từ chối cổng đang có chủ. Bản Tauri hiện gắn8765 nên không dùng launcher Desktop cho candidate này.
2. Vào Kết nối → Dùng profile có sẵn, chọn nguồn, rồi **Cho phép xem profile · 5 phút**. Đây mới là bước Core đọc metadata/credential source thuộc tài khoản Windows đó. Không gửi key vào chat.
3. Chọn profile có transport được hỗ trợ. Với env hoặc cached Kimi, đọc loại nguồn và hạn hiệu dụng. Không có nút OAuth login/refresh giả.
4. Ở Phạm vi, xem bot/kênh/dự án, provider và dữ liệu có thể gửi; chọn **Cho phép dùng trong phạm vi này**. Consent chưa gọi model. Chỉ khi người dùng gửi request mới thì provider mới được gọi và có thể phát sinh phí.
5. Thu hồi quyền để dừng việc đang dùng nó. Đổi snapshot/profile/token, hết deadline hoặc restart không tự phục hồi authority cũ. Với cached Kimi hết hạn/401, người dùng quản lý credential ở Core; app không tự login hay refresh.

Chưa thực hiện những bước với profile/provider thật trong đợt phát triển này. Để thử thật chỉ cần chọn profile/provider, kênh/bot, dữ liệu được phép gửi và giới hạn; không cần chọn lại stack hoặc quyết định kiến trúc.

## Bằng chứng và giới hạn

| Gate | Commit/phạm vi | Kết quả |
| --- | --- | --- |
| Go full test/vet/build | `2f260d4`; native Git tree không đổi ở `5987ed1` | 494 top-level +499 subtests PASS; DOCX reference và browser seed tùy chọn SKIP |
| Rust offline/fmt/release | `5987ed1` | 33 PASS |
| Core affected regression/type/lint/build | `973a3bd` | 282 PASS /2299 assertions, 25 files; writer Windows5 nằm trong số đó |
| Core compiled broker/cleared-env Verify | `973a3bd` | 9 PASS |
| UI auth tuple/snapshot/focus | exact asset hashes, authored Chrome fixtures | 40 PASS; không dùng thay backend/native |
| Actual Go/Rust/Core · env | `5987ed1`, synthetic loopback | 25 PASS |
| Actual Go/Rust/Core · cached Kimi | `5987ed1`, isolated rerun | 22 PASS |
| Actual Go/Rust/Core · file key/no-key | `5987ed1`, synthetic loopback | 19 +19 PASS |
| Review closure | exact `2f260d4`/`5987ed1` +Core973 | concurrent inspection và absent-expiry P2 đóng; negative receipts giữ nguyên |

Native checks giữ genuine Core HANDLE trước và sau stop khi snapshot thay, revoke, request deadline và HTTP401. Chúng dùng Go/Rust/Core thật, credential/model loopback giả lập. Chưa nghiệm thu token/capsule expiry300s bằng một native drill đủ5 phút, Tauri/Wiii owner dialog, phone/keyboard/safe-area/AT, model quality/provider thật hay device/cloud.

Một lượt Kimi ở bốn service fixture chạy song song bị `host_rejected` tại admission của request thứ hai, trước inference. Cùng binary/driver/assertions chạy riêng PASS22. Lượt fail và projection journal giữ nguyên; nguyên nhân chưa được chứng minh, nên chưa gọi là lỗi harness hay nghiệm thu chịu tải song song. Lượt Kimi cũ `2f260d4` timeout sau401 là expectation sai đã đối chiếu được với contract `recovery_required`; cũng được giữ riêng.

Full OAuth renewal và các provider OAuth khác vẫn thiếu code: refresh-origin/budget authority, token rotation CAS, cancel/redirect và chuyển credential revision. Wiii lock Core1.7/four-field catalog chưa đáp ứng bridge-v2; chưa thay bản đã cài hoặc owner transport consent. Backlog sản phẩm hữu hạn đã ghi ở `ux-connections-profile-steps-20261012.md` vẫn còn, không mở rộng thành framework mới trong lát cắt này.

Source Main chỉ fast-forward sau scan/kiểm; ba edits cũ, 17 ZIP và 1509 protected files được đối chiếu hashes. Patch, local bundle, binaries và public receipts nằm trong packet; private capsule/key/source homes, journal, browser profiles và credential fixtures không được đưa vào packet.
