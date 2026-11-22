# Đóng góp cho Neko Aurora

Cảm ơn bạn đã giúp cải thiện một bản phát triển đa bot/đa kênh. Trước khi sửa, đọc [README](README.md), [setup](docs/getting-started.md), [trạng thái hỗ trợ](docs/project-status.md) và AGENTS.md của component liên quan.

## Chọn phạm vi

Tìm [issue hiện có](https://github.com/meiiie/neko-aurora/issues) trước. Với tính năng mới hoặc thay đổi authority/runtime, mở đề xuất có mục tiêu, scope và acceptance trước khi làm lớn. Sửa tài liệu/lỗi nhỏ có thể gửi PR trực tiếp. Giữ mỗi PR tập trung vào một kết quả; không trộn thay license, nâng dependency và sửa runtime không liên quan.

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
- Go: từ `app/native`, chạy `go test -count=1 ./...`, `go vet ./...` và build command trong [setup](docs/getting-started.md).
- Rust host: `cargo test --locked --manifest-path app/host/Cargo.toml`; desktop build theo setup. Chỉ dùng `--offline` khi cache đã đủ.
- Frontend: `node --check app/native/internal/web/assets/app.js` và `node --check app/native/internal/web/assets/connections.js` chỉ kiểm cú pháp. Cần thêm kiểm tra interaction/focus/viewport phù hợp khi đổi hành vi; cú pháp PASS không chứng minh UI hoạt động.
- Core: dùng Bun/lockfile và gate trong [Core testing](core/docs/process/TESTING.md). Không thay các kiểm thử runtime bằng fixture ở component khác.

Ghi bằng chứng manual trong PR: exact commit, command, tool/OS, PASS/FAIL/BLOCKED và scope. Không gọi fixture là model evaluation, viewport là physical-phone acceptance hoặc local replica là cloud sync. Nếu một gate chưa chạy, nói rõ lý do. Giữ nguyên failure có ý nghĩa; không nới assertion chỉ để có PASS.

## CI cho pull request

CI lọc đường dẫn và chạy song song Go vet/build/test, Rust host test, Linux desktop check, Core typecheck/lint/build/test và docs check; `ci-ok` là check tổng hợp cho branch protection. Main/manual chạy đầy đủ; thay workflow hoặc `tools/ci/` bật mọi lane. Core PR chọn test theo đồ thị import TypeScript (kể cả alias trong tsconfig), fallback toàn bộ khi đổi config/helper/asset, xóa file hoặc không phân giải được dependency. Bốn shard được chia greedy theo thời gian đã đo cộng 0,1 giây/file; file mới mặc định 1 giây. Xem kế hoạch bằng `bun tools/ci/core-tests.ts --base origin/main` hoặc `--full`; thêm `--run --shard 1` để chạy một shard. Để cập nhật timing, chạy `(cd core && bun test) > "$HOME/core-test-timings.log" 2>&1`, rồi `bun tools/ci/core-tests.ts --refresh "$HOME/core-test-timings.log"` và review JSON trước commit. Bun 1.4.0 chưa có `bun check`, nên typecheck vẫn dùng TypeScript; Linux check không chứng minh desktop/DPAPI chạy được trên Windows.

## Quyền, dữ liệu và provenance

Không dùng provider trả phí, credentials, state thật hoặc grant mới làm kiểm thử mặc định. Chỉ dùng fixture không nhạy cảm, state/profile/port riêng. Không đưa raw journal, account store, startup challenge, chat riêng, token, build/cache, `.env` hoặc bundle tham chiếu vào PR. Quét secrets trước checkpoint/push và không in giá trị match.

Giữ license/NOTICE/attribution gốc; không suy rộng Apache từ SDK sang Core/app. Root app chưa có license tổng thể. Đóng góp không tự tạo một license mới hoặc chuyển quyền sở hữu. Material bên ngoài phải có quyền phân phối rõ; xem [Core contribution policy](core/CONTRIBUTOR-LICENSE-POLICY.md) khi sửa Core. Neko Peek/Geist giữ notice tại [asset provenance](docs/assets/README.md).

Dùng lời lẽ tôn trọng, nêu vấn đề cụ thể và bằng chứng có thể kiểm tra. Người gửi chịu trách nhiệm hiểu và xác minh mọi thay đổi, kể cả code có trợ giúp AI. Không có cam kết thời gian review/merge. Vấn đề bảo mật theo [SECURITY](SECURITY.md), không đăng chi tiết nhạy cảm vào PR/issue công khai.
