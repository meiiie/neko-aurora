# Desktop candidate có endpoint/state được xác minh

Code đã kiểm: `c91e4de75ef57fd34cafe11dfe491fa77385e7bd`. Core giữ `973a3bd93466baa4689e01c4050dfc59f5a7f7c4`, host giữ hash `a1693c4f3309c86e29b348ffc913c888cae3f8fd2bdf04bcac418e107a6654d2`. Bản được đề xuất hiện tại là `E:/CodexScratch/desktop-instance-20261012/candidate-c91e4de`; delivery chỉ thêm tài liệu. Manifest của candidate ghi exact commit và hashes. Candidate mới chưa mở cửa sổ hay service; live8765 giữ nguyên. Các packet cũ và blocker reports giữ nguyên theo lịch sử.

## Cách mở đơn giản nhất

Mở **Start-Neko-Desktop.cmd** trong candidate mới. Launcher xác minh hashes, dùng port8767 và state riêng, khởi động Go do nó sở hữu, rồi mở Tauri đúng service. Khi người dùng quay lại, chỉ xem giao diện/Kết nối và thử Tab/Escape trước; **chưa cần đồng ý xem/dùng profile**. Đóng cửa sổ sẽ kết thúc service candidate do launcher sở hữu. Nếu port8767 có chủ hoặc binding không khớp, launcher từ chối; không gắn vào8765.

Có thể chạy `Start-Neko-Desktop.ps1 -CheckOnly` để kiểm file/manifest mà không khởi động. Không mở trực tiếp Desktop `.exe`: shell hiện yêu cầu binding do launcher cấp, không còn fallback implicit8765. Candidate có broker registry cho profile của chính người dùng Windows, nhưng discovery chưa đọc credentials hoặc gọi model. Xem README auth được giữ trong `docs/auth-profile-steps.md` trước khi tự chọn profile/dữ liệu/ngân sách và consent riêng; cached Kimi vẫn không refresh, không có OAuth login tự động.

## Contract đã triển khai

Go chỉ nhận receipt opt-in, file mới nằm trực tiếp trong state, ghi bằng O_EXCL/Sync. Receipt private chứa phiên bản1, endpoint, PID, hash executable thật, hash đường dẫn state canonical, challenge ngẫu nhiên và cửa sổ startup30 giây. GET `/v1/local-instance` phải có challenge trong header riêng, đúng Host/Origin; trả binding không chứa challenge hoặc raw state path. Không có model, profile capture hay grant qua endpoint này. CSP HTTP hiện có và CORS/IPC policy không mở rộng.

Tauri chỉ nhận argv đầy đủ; endpoint phải đúng dạng `http://127.0.0.1:<port>`, không DNS/remote, userinfo, path/query/fragment/secret URL hoặc implicit port/default. Strict JSON từ chối field lạ/duplicate. Shell kiểm state/hash/TTL, Windows TCP listener owner, process image hash và creation time, rồi đối chiếu challenge/binding HTTP. Receipt cũ, PID/port/state/hash/challenge sai bị từ chối trước WebView. Thời hạn30 giây chỉ cho startup; khi đã mở, liveness tiếp tục ràng buộc process/port/hash/challenge gốc, không tự rebind. Navigation chỉ cùng origin đã chọn, new window bị từ chối, IPC capabilities vẫn rỗng. WebView dùng profile directory theo state và incognito; reparse path bị chặn. Monitor phát hiện mismatch sẽ đóng cửa sổ; polling không phải bảo đảm instruction-level cho mọi subrequest.

Launcher yêu cầu desktop contract1 và hashes role. Nó kiểm port thuộc đúng Go child, chờ Desktop `--verify-only` bằng Process.WaitForExit/ExitCode trước khi mở cửa sổ, rồi chỉ cleanup child do nó sở hữu. Candidate wrapper pin manifest, control và Desktop hashes để chặn trộn/downgrade artifact. `-VerifyDesktopOnly` dùng cho kiểm contract/launcher không tạo cửa sổ; đây không phải công cụ thay thế CUA.

## Bằng chứng đúng scope

| Gate | Kết quả ở clean code c91e4de |
| --- | --- |
| Full Go test/vet/build | 502 top-level +510 subtests PASS; DOCX reference/browser seed tùy chọn SKIP |
| Tauri offline locked fmt/test/release | 2 test PASS, không warning ở lượt final; release binary mới |
| Actual Go + Desktop CLI + PowerShell launcher | 29 PASS: hai process/port/state/challenge, history/cookie isolation, wrong PID/port/state/hash/challenge/clock/version, stale receipt, no default/downgrade, launcher wait/cleanup |
| Bounded independent source review | 11 paths; không P1/P2 mới được xác nhận; không mượn kết quả runtime/UI |

Go RED trước implementation được giữ. Hai lượt working29 có launcher FAIL được giữ: gọi GUI executable bằng `&`/LASTEXITCODE khiến cleanup xảy ra trước verifier hoàn tất; chuyển sang Process.WaitForExit/ExitCode, cùng binaries qua29. Lượt clean final dùng binary đúng snapshot, không dùng working artifact làm release. Source/runtime auth concurrency, host/Core và bốn UI asset không đổi; 154 native auth/concurrency chốt trước là evidence kế thừa, không gọi là lượt chạy mới.

## Gate chưa hoàn tất

Thiếu endpoint hardcoded đã được sửa bằng code và kiểm contract. **Native visualQA vẫn BLOCKED**: CUA inventory lỗi kernel hai lần, native API bị disabled và không có node_repl/@oai/sky callable. Không mở/điều khiển cửa sổ bằng phương thức thay thế. CLI tests không chứng minh WebView open/close, incognito cookie isolation trong hai cửa sổ thật, navigation/monitor close, Kết nối consent UI, foreground/background/group/report, keyboard/focus hoặc reduced-motion native.

Source hoàn tất trong scope endpoint/launcher binding local và auth-v2/admission concurrency đã kiểm. Toàn bộ Neko Bot chưa production-ready: còn native UI/user/device/phone/IME/safe-area/AT, model/provider/account thật, Wiii bridge-v2, OAuth renewal và device/cloud. Người dùng có thể duyệt UI trước rồi quyết định thử profile và scope thật; không cần lặp lại lựa chọn stack. Không đọc/sửa credential thật, deploy, push, grant thật hoặc đụng live8765 trong đợt này.
