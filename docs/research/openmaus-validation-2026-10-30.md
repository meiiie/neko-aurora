# Kiểm chứng workspace tham khảo OpenMausBot — 2026-10-30

Code commit: `6f93ea26dbe7d99e60b0e63fc6df2fee1c5fc471`, gồm API ở `9740419f12e624db65bb1cc77a24764ccca6fccc`, trên base đã merge `35dfbe46b79d25bb6df780befbd345938f643aef`. Nhánh local: `codex/openmaus-interface`. Source manifest SHA256: `80977f7bff121a0eb00d28d7934c402048a359a907135ee82a14cbb718dcf8f9`; 1.873 file đã đối chiếu với Git, có xét chuẩn hóa LF sẵn có. Tài liệu/ảnh đi sau không thay source ứng dụng.

Một agent tự thực hiện theo pstack, redesign-existing-projects và webapp-testing. Không có subagent hoặc independent reviewer. Reference chỉ đọc, code/assets không được chép vào Aurora. Không dùng account/provider thật, tạo grant, push hoặc deploy trong lượt UI này.

## Môi trường và lệnh

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. Runner dùng danh sách biến môi trường cho phép, loại thông tin provider; Go/Node có USERPROFILE/XDG/npmrc riêng, `GOTOOLCHAIN=local`, `AURORA_BRIEF_LOOP=off`, `AURORA_COMPUTER=off`, `PSModuleAnalysisCachePath=NUL`. Browser dùng profile Playwright tạm, dữ liệu loopback tổng hợp. Các gate nặng chạy tuần tự, không sửa source trong lúc kiểm tra.

Lệnh dưới đây rút gọn đường dẫn executable thành `go`, `node`, `python`; đối số giữ nguyên. Nhật ký local `.audit/2026-10-30-openmaus/validation.jsonl` giữ đường dẫn thực, thời gian, SHA và kết quả từng lần. [Manifest trình duyệt](openmaus-browser-validation-2026-10-30.json) đi kèm source để tra từng script, command, thời gian và phiên bản source.

| Kiểm tra | Lệnh | Kết quả |
|---|---|---|
| API scope lịch | `go test -v -count=1 -run TestScheduleOverviewWorkspaceBoundary ./internal/aurora` từ `app/native` | PASS: đúng workspace, client cũ, workspace khác, rỗng và lặp query |
| Browser | `python -u app/tests/aurora/<tên>-browser.py` từ root; ngoại lệ OAuth bên dưới | PASS, 39/39 scripts; mọi manifest đều khớp source commit cuối, không có source thay đổi giữa gate |
| OAuth qua Go API thật | `go test -v -count=1 -timeout=180s -run ^TestMCPOAuthBrowserLocalCookieFlow$ ./internal/aurora`, với `AURORA_TEST_MCP_BROWSER=<repo>/app/tests/aurora/mcp-oauth-browser.py` | PASS, 7 nhóm; login/token endpoint vẫn là fixture |
| Composer/outbox unit | `node --test app/tests/aurora/composer-drafts-test.mjs app/tests/aurora/outbox-test.mjs` | PASS, 8 tests |
| JS syntax | `node --check` cho `assets/app.js`, `assets/connections.js` và từng `assets/app/*.js` | PASS; chỉ chứng minh cú pháp |
| Full Go | `go test -json -p=1 -count=1 -timeout=300s ./...` từ `app/native` | PASS, 2.106 test/subtest, 38 SKIP, 25 package; 372,64 giây; `go-serial-213205.log` |
| Static/build | `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` từ `app/native` | PASS; `vet-213818.log`, `build-213828.log` |
| Docs | `python tools/check_docs.py`; kiểm tra bổ sung link trong tài liệu mới, schema cơ bản của issue forms và hash ảnh | PASS: 166 link từ checker chính, 11 link bổ sung, 3 issue forms, 39 bản ghi browser và 3 ảnh fixture |

Không chạy lại Rust hoặc toàn bộ Neko Core: source, dependencies và toolchain của hai component này không thay đổi. Build ở đây là control-plane binary có nhúng UI, không phải bộ cài Tauri/release đã ký.

## Tình huống đã kiểm tra

Fixture workspace mới có 15 nhóm: layout và số liệu thật; rail mở các chức năng sẵn có; lọc bot; chuyển bot giữ draft, không gửi; palette với dấu tiếng Việt/IME/keyboard/focus; cột hồ sơ đúng bot; thu gọn qua reload; lịch có filter/error/wrong-scope/retry/manage; đổi chat lúc lịch còn tải; đổi workspace hủy dữ liệu/focus cũ; mobile/back/draft; dark theme và dialog; ngôn ngữ không đổi tên người dùng; các mốc chiều rộng 320/768/900/1200/1440; không có mutation hoặc lỗi JavaScript từ thao tác điều hướng. Nút pin còn được kiểm tra hit target khi nhận focus.

Bộ hồi quy bao phủ startup/reload composer, chat loading/identity/reading position, danh sách và unread/working state, IME, form/question drafts, selection/copy, table scrolling/viewer, ảnh upload/file download, dictation, Pages, máy tính recovery, MCP config/discovery/OAuth, memory, secret và Vault lifecycle, usage recovery, lịch edit/test/loading/timezone. Các thao tác có side effect trong regression dùng fixture riêng, không phải tài khoản thật.

Ảnh light/dark, chat/context, mobile và routines đã được xem trực tiếp. Ảnh được phân phối dưới `docs/assets/screenshots/` có provenance tại [assets](../assets/README.md); dữ liệu là tổng hợp, không phải nội dung tài khoản người dùng.

## Failure trail và sửa nguyên nhân

- Test scope lịch RED trước khi handler nhận workspaceId; GREEN sau khi từ chối scope rỗng/khác/lặp và echo đúng scope. Client hồ sơ cũ vẫn được kiểm tra tương thích.
- Palette ban đầu để Tab ra ngoài; thêm vòng focus rõ ràng, không đổi assertion.
- Focus composer bị chặn bởi inert cập nhật trễ; cập nhật accessibility layout đồng bộ trước thao tác focus của handler cũ.
- Resize từ desktop sang mobile làm mất message đang đọc do reflow; bổ sung anchor theo message và offset. Kiểm tra tiếp phát hiện sai số pixel tích lũy; giữ offset ổn định dưới một pixel. Test giữ nguyên ngưỡng dưới hai pixel và thêm collapse/expand sidebar.
- Chuyển trực tiếp từ lịch đang tải sang bot có thể quay lại trạng thái reload bị khóa. Test bổ sung RED rồi GREEN sau khi hủy overview khi vào chat.
- Một lần `table-scroll-browser-211857.log` sai khác scrollTop khi focus control; assertion được giữ, bổ sung giá trị chẩn đoán. Không tái hiện ở hai lượt tiếp theo và năm lượt liên tiếp trên source cuối (`213005` đến `213032`). Chưa xác định được nguyên nhân của lần đơn lẻ; không gán nó thành một lỗi sản phẩm đã sửa.
- OAuth browser cần BASE/BOT do Go fixture cấp; lần gọi Python trực tiếp bị lỗi harness. Chạy qua `TestMCPOAuthBrowserLocalCookieFlow` đạt, bao gồm owner cookie và service worker thật. Không nới kiểm tra token/callback.

## Phạm vi và checkpoint

Quét recognizable secrets trước checkpoint API: 2 candidate files, 0 newly reachable blobs, 0 match; trước checkpoint UI: 42 candidate files, 2 newly reachable blobs, 0 match. Quét tài liệu/ảnh: 9 candidate files và 44 newly reachable blobs, 0 match. Không in matched values. Đây không phải chứng nhận phát hiện mọi secret. Manifest và kết quả riêng từng checkpoint nằm trong audit local.

Kiểm thử viewport không thay thế điện thoại vật lý hoặc native WebView. Fixtures không chứng minh chất lượng model, cloud sync, provider OAuth certification hay hoạt động trong môi trường production. Service worker thực đã được kiểm tra ở OAuth fixture; chưa có bài acceptance mất mạng đầy đủ cho cả workspace. Không tuyên bố tương đương tất cả OpenMausBot; [bản đồ nghiên cứu](openmaus-interface-2026-10-30.md) tách rõ các khả năng chưa triển khai.
