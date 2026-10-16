# Đóng góp cho Neko Bot

Cảm ơn bạn đã giúp cải thiện một bản phát triển đa bot/đa kênh. Trước khi sửa, đọc [README](README.md), [setup](docs/getting-started.md), [trạng thái hỗ trợ](docs/project-status.md) và AGENTS.md của component liên quan.

## Chọn phạm vi

Tìm [issue hiện có](https://github.com/meiiie/neko-bot/issues) trước. Với tính năng mới hoặc thay đổi authority/runtime, mở đề xuất có mục tiêu, scope và acceptance trước khi làm lớn. Sửa tài liệu/lỗi nhỏ có thể gửi PR trực tiếp. Giữ mỗi PR tập trung vào một kết quả; không trộn thay license, nâng dependency và sửa runtime không liên quan.

| Phần việc | Entry point |
|---|---|
| UI web | `app/native/internal/web/assets/` |
| Journal/API/điều phối | `app/native/internal/ledger/`, `app/native/internal/api/`, `app/native/internal/runtime/` |
| Trusted host/Tauri | `app/host/`, `app/desktop/` |
| Agent loop | `core/`; đọc [working rules](core/AGENTS.md) và [testing](core/docs/process/TESTING.md) |
| Wiii | `wiii/` chỉ là 61 file overlay; phải đối chiếu base trước khi sửa/build |
| Onboarding và hình ảnh repo | `README.md`, `docs/`, `.github/` |

Fork repo và tạo nhánh từ `main`. Các nhánh `wip/*` và `archive/*` dùng để bảo toàn trạng thái cũ, không phải nền mặc định cho đóng góp mới. Không force-push vào nhánh chung hoặc sửa source repository gốc qua bản snapshot này.

## Kiểm tra theo thay đổi

- Docs/assets: chạy `python tools/check_docs.py`, kiểm tra link nội bộ, chính tả, alt text, render desktop/mobile và provenance. SVG phải thụ động, không script/remote dependency. `docs/assets/generate_presentation.py` cần fontTools/Brotli để tái tạo các SVG hiện có.
- Go: mặc định từ `app/native`, chạy regression/package bị ảnh hưởng, ví dụ `go test -count=1 ./internal/providerexec`, cùng `go vet` cho package liên quan; thay package theo diff thực tế. Khi sửa boundary dùng chung về authority, persistence, protocol hoặc platform, hoặc rủi ro integration cuối/release yêu cầu, chạy `go test -count=1 ./...`, `go vet ./...` và build command trong [setup](docs/getting-started.md). Diff nhỏ không tự chứng minh rủi ro thấp.
- Rust host: `cargo test --locked --manifest-path app/host/Cargo.toml`; desktop build theo setup. Chỉ dùng `--offline` khi cache đã đủ.
- Frontend: `node --check app/native/internal/web/assets/app.js` và `node --check app/native/internal/web/assets/connections.js` chỉ kiểm cú pháp. Cần thêm kiểm tra interaction/focus/viewport phù hợp khi đổi hành vi; cú pháp PASS không chứng minh UI hoạt động.
- Core: dùng Bun/lockfile và gate trong [Core testing](core/docs/process/TESTING.md). Không thay các kiểm thử runtime bằng fixture ở component khác.

Chọn kiểm tra theo hành vi có thể hỏng, không theo số lượng test. Ưu tiên regression rẻ trên đường chạy có sẵn; nếu harness mới quá tốn hoặc chỉ kiểm mock, dùng reproduction gần nhất và ghi giới hạn. Sau baseline sạch, không chạy lại toàn suite khi source được kiểm, dependency và môi trường vẫn không đổi. Thay đổi liên quan, conflict integration, failure hoặc rủi ro chưa giải quyết yêu cầu chạy lại scope bị ảnh hưởng; không gắn bằng chứng cũ thành lần chạy mới. Xem [workflow phát triển](docs/development-workflow.md).

Repo này chưa có CI tự động ở root. Ghi bằng chứng manual trong PR: exact commit, command, tool/OS, PASS/FAIL/BLOCKED/NOT RUN và scope. Không gọi fixture là model evaluation, viewport là physical-phone acceptance hoặc local replica là cloud sync. Nếu một gate chưa chạy, nói rõ lý do, nhất là Windows/ConPTY/SRT, thiết bị thật và provider thật; local fixture không thay các gate này. Giữ nguyên failure có ý nghĩa; không nới assertion chỉ để có PASS.

## Quyền, dữ liệu và provenance

Không dùng provider trả phí, credentials, state thật hoặc grant mới làm kiểm thử mặc định. Chỉ dùng fixture không nhạy cảm, state/profile/port riêng. Không đưa raw journal, account store, startup challenge, chat riêng, token, build/cache, `.env` hoặc bundle tham chiếu vào PR. Quét secrets trước checkpoint/push và không in giá trị match.

Giữ license/NOTICE/attribution gốc; không suy rộng Apache từ SDK sang Core/app. Root app chưa có license tổng thể. Đóng góp không tự tạo một license mới hoặc chuyển quyền sở hữu. Material bên ngoài phải có quyền phân phối rõ; xem [Core contribution policy](core/CONTRIBUTOR-LICENSE-POLICY.md) khi sửa Core. Neko Peek/Geist giữ notice tại [asset provenance](docs/assets/README.md).

Dùng lời lẽ tôn trọng, nêu vấn đề cụ thể và bằng chứng có thể kiểm tra. Người gửi chịu trách nhiệm hiểu và xác minh mọi thay đổi, kể cả code có trợ giúp AI. Không có cam kết thời gian review/merge. Vấn đề bảo mật theo [SECURITY](SECURITY.md), không đăng chi tiết nhạy cảm vào PR/issue công khai.
