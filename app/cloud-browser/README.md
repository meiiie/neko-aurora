# Triển khai trình duyệt cloud cho Aurora

Worker này chạy Chromium trên máy riêng của Aurora. Docker dùng named volume `/data`; pilot Railway dùng đĩa của một Sandbox VM riêng. Máy của người dùng chỉ hiển thị ảnh và gửi lệnh qua Aurora.

Bản Railway đã triển khai tại [Aurora cloud](https://neko-aurora-production.up.railway.app/app/). Kiểm thử giao thức Go–Node, Vault và Chromium đã chạy cả trong Docker lẫn VM Railway. Đăng nhập Zalo thật trên VM còn chờ owner xác nhận. Xem [phạm vi và hợp đồng](../docs/cloud-browser-20261025.md).

## Kiểm thử tạm bằng Docker

Docker Desktop trên máy phát triển có thể chạy hồ sơ riêng của Aurora khi owner cho phép. Các bài kiểm chứng bên dưới dùng volume fixture tách biệt. Để chạy liên tục khi máy người dùng tắt, đặt Docker trên VPS hoặc máy chủ riêng.

Từ thư mục gốc repository, build image:

```sh
docker compose -f app/cloud-browser/compose.yaml build
```

Chạy bài kiểm chứng Chromium trong container không có mạng ngoài:

```sh
docker compose -f app/cloud-browser/compose.yaml --profile acceptance run --rm acceptance
```

Để kiểm tra qua hai container, dùng một volume fixture mới, chạy lần lượt:

```sh
docker compose -f app/cloud-browser/compose.yaml --profile acceptance run --rm -e AURORA_FIXTURE_PHASE=prepare acceptance
docker compose -f app/cloud-browser/compose.yaml --profile acceptance run --rm -e AURORA_FIXTURE_PHASE=check acceptance
```

Lần đầu lưu cookie có thời hạn, localStorage, IndexedDB và biên nhận thao tác. Lần sau tạo Chromium trong container mới, đọc lại hồ sơ và kiểm tra rằng lệnh cũ không tạo thêm tác động. Hai lần dùng cùng volume `fixture-data`; volume này tách khỏi `browser-data`.

Compose dùng `seccomp.playwright.json` từ Playwright để cho Chromium tạo user namespace mà vẫn bật sandbox. Nguồn và giấy phép nằm trong [THIRD-PARTY.md](THIRD-PARTY.md). Không dùng `--privileged`, `seccomp=unconfined` hoặc tự chuyển sang `--no-sandbox`. Xem [hướng dẫn Playwright](https://playwright.dev/docs/docker) nếu kernel của máy chủ vẫn từ chối sandbox.

## Chạy worker trên Docker của máy chủ riêng

1. Tạo khóa dịch vụ ngẫu nhiên ít nhất 32 byte và cấp biến `AURORA_CLOUD_BROWSER_KEY` cho Compose. Giữ khóa qua các lần tạo lại container.
2. Chạy `docker compose -f app/cloud-browser/compose.yaml up -d browser`.
3. Dùng `docker compose -f app/cloud-browser/compose.yaml ps` để kiểm tra health check.
4. Kết nối container Aurora vào network `aurora-browser_cloud`. Đặt URL worker của Aurora thành `http://browser.aurora.internal:8080` và cấp cùng khóa dịch vụ.
5. Phục vụ giao diện Aurora qua HTTPS có xác thực owner như cấu hình hiện có. Worker không publish cổng ra host.

Compose dùng named volume `browser-data`, một worker, giới hạn RAM 1 GiB và CPU 1. Không gắn thư mục trình duyệt hoặc Docker socket của host. `docker compose down` giữ volume; thao tác xóa volume sẽ mất hồ sơ và cần được xử lý như xóa phiên đăng nhập.

Compose này chạy worker trình duyệt và bài kiểm chứng. Nó không tự thay cấu hình của Aurora đang chạy ngoài Docker. Aurora cần ở cùng mạng riêng hoặc truy cập một endpoint HTTPS được bảo vệ. Nếu Aurora có cấu hình HTTP proxy, đặt `NO_PROXY` cho hostname worker riêng.

`docker compose -f app/cloud-browser/compose.yaml stop` dừng worker. Các container fixture không chứa tài khoản Zalo thật.

## Railway: VM trình duyệt và service keeper

Service Railway thông thường đã thất bại khi chạy Chromium bật sandbox (`credentials.cc:131`, Permission denied). Pilot hiện dùng Sandbox VM riêng 1 CPU / 1 GiB trên mạng riêng, worker UID `10001`, không bỏ sandbox. Service tên `aurora-browser-worker` chỉ còn là keeper; nó không chứa Chromium hoặc hồ sơ tài khoản.

Khi chuẩn bị máy mới:

1. Dùng base checkpoint đã kiểm chứng, không chứa secret hoặc hồ sơ. Cài source worker công khai, Node 24 và Playwright theo lockfile; chạy `node --test tests/cloud-browser-live.mjs` bằng user thường trước khi cho phép đăng nhập thật. Một health check Node thành công chưa chứng minh Chromium chạy được.
2. Tạo VM từ base sạch với `AURORA_CLOUD_BROWSER_KEY` ngẫu nhiên ít nhất 32 byte, `DATA_DIR=/data`, `PORT=8080`, đường dẫn browser và môi trường tối thiểu. Không cấp Railway API token, model key hoặc Vault master key cho VM. Giữ `/data` mode `0700`, một worker ghi, tối đa một browser hoạt động.
3. Chạy worker trên VM bằng user `aurora`. Bản hiện tại dùng `/usr/sbin/runuser -u aurora -- /usr/local/bin/node /opt/aurora-browser/server.mjs` trong một Railway sandbox exec session detached, không đặt deadline làm dừng server. Chỉ công bố hostname `*.railway.internal` cho Go.
4. Triển khai keeper với [Dockerfile.keeper](Dockerfile.keeper), [railway.keeper.json](railway.keeper.json) và `vm-keeper.mjs`. Keeper cần `AURORA_BROWSER_SANDBOX_ID`, `RAILWAY_ENVIRONMENT_ID` và reference tới `RAILWAY_TOKEN` đã được owner cho phép trên server. Token chỉ ở keeper. Không gắn hồ sơ browser vào service này.
5. Chờ keeper `/healthz` báo `ready: true` từ mạng riêng, rồi dừng heartbeat bootstrap trên máy phát triển. Keeper gửi heartbeat mỗi 60 giây; phản hồi trễ quá 150 giây làm health check thất bại. Keeper không tạo, hủy hay khôi phục VM.
6. Kiểm tra Go gọi được worker qua chữ ký thật, rồi mở giao diện owner để đăng nhập. Không expose cổng worker, CDP hoặc proxy ra Internet.

Khi tải source bằng CLI, chuẩn bị archive riêng đã kiểm tra, dùng `--path-as-root`, giữ `.railwayignore` và không dùng `--no-gitignore`. Keeper chỉ cần các file tối thiểu ở bước 4. Không tải thư mục trạng thái, khóa, reference clone hoặc toàn bộ working tree.

Worker nhận cả IPv4 và IPv6. Bản Docker chạy bằng `tini`, chuyển sang UID `10001`. Bản VM hiện dùng exec detached với cùng UID. Cả hai đều bật Chromium sandbox; không tự thêm `--no-sandbox` khi môi trường từ chối.

**Giới hạn pilot:** đĩa VM giữ hồ sơ khi đóng/mở Chromium và restart worker, nhưng chưa có volume bền vững qua hủy VM. Keeper duy trì máy, không thay thế backup. Nếu máy bị hủy/thu hồi, cần tạo máy sạch và đăng nhập lại. Không dùng checkpoint chứa tài khoản thật; việc khôi phục checkpoint cũ có thể làm sống lại dữ liệu đã xóa.

## Kết nối Aurora

1. Triển khai bản Aurora có `internal/cloudbrowser` và giao diện `cloud.js`.
2. Đặt `AURORA_CLOUD_BROWSER_URL=http://<hostname-VM>.railway.internal:8080` trên service Aurora. Dùng hostname VM thực tế, không dùng hostname keeper.
3. Đặt `AURORA_CLOUD_BROWSER_KEY` bằng đúng khóa của worker.
4. Khởi động lại Aurora với state directory bền vững hiện có. Giữ file `cloud-browser-namespace` trong state directory đó.
5. Với hosted mode, đặt `AURORA_CLOUD_BROWSER_EMAILS` cho đúng các owner trong pilot. Không có danh sách này thì hosted tenant không được dùng máy cloud.
6. Mở Aurora bằng tài khoản chủ sở hữu. Bấm biểu tượng **Máy tính cloud**, chọn bot và bấm **Mở trình duyệt**.

Nếu worker chưa được cấu hình, giao diện hiện **Chưa cấu hình cloud**. Nếu worker gặp lỗi, Aurora trả lỗi cloud và không mở Zalo bằng trình duyệt của người dùng.

## Kiểm chứng Chromium trước khi dùng tài khoản

Trong một container cloud thử nghiệm dùng cùng image, chạy:

```sh
node --test tests/cloud-browser-live.mjs
```

Lệnh này mở một trang fixture trong container bằng hồ sơ tạm mới. Lệnh kiểm tra ảnh thật, điền Vault mã hóa, cookie, localStorage, IndexedDB, đóng rồi mở lại worker, tách namespace và chống gửi lại thao tác cùng nonce. Lệnh không dùng tài khoản Zalo và không chứng minh volume còn nguyên sau khi Railway thay container.

Để chạy các kiểm thử không mở Chromium, dùng:

```sh
npm ci
npm test
```

## Đăng nhập và kiểm tra hồ sơ thật

1. Trong cửa sổ máy tính cloud của bot, bấm **Nhận điều khiển**.
2. Đăng nhập Zalo Web qua QR hoặc thao tác trực tiếp trên ảnh. Nếu dùng Vault, chọn mục đăng nhập có đúng origin, xác nhận rồi bấm **Duyệt & điền lần này**.
3. Hoàn tất xác nhận trên điện thoại khi Zalo yêu cầu. Vault chỉ điền trường, không tự bấm đăng nhập.
4. Kiểm tra trang tài khoản hiển thị trong ảnh. Nhãn **Đăng nhập chưa xác minh** có thể vẫn xuất hiện vì adapter chưa xác minh danh tính Zalo tự động. Để giao quyền dùng hồ sơ cho agent, lấy **Quan sát mới** rồi bấm **Cho phép agent dùng hồ sơ này**. Nút chỉ hoạt động khi owner đang giữ điều khiển và trang có nội dung, không còn QR hoặc bước bảo vệ. Quyền dùng hồ sơ không xác minh danh tính tài khoản; thao tác agent vẫn cần approval.
5. Đóng rồi mở lại trình duyệt cloud. Kiểm tra tài khoản trên ảnh.
6. Khi owner đã sẵn sàng, restart tiến trình worker trên cùng máy rồi mở trình duyệt và kiểm tra tài khoản lần nữa. Với Docker, có thể thay container trên cùng volume. Không hủy VM Railway để thử persistence: pilot chưa có volume bền vững qua hủy máy.
7. Chuyển sang bot khác và xác nhận bot đó có hồ sơ riêng.

Đóng trình duyệt giữ hồ sơ. Đóng cửa sổ chỉ ẩn giao diện. Để thu hồi phiên Zalo, dùng thao tác đăng xuất hoặc quản lý thiết bị của Zalo rồi kiểm tra lại trên ảnh.

## Khôi phục thao tác chưa rõ kết quả

Nếu Aurora báo kết quả chưa rõ, giữ nguyên mã thao tác để đối chiếu. Bấm **Quan sát mới**, đọc trạng thái hiện tại và xác nhận đã xem trước khi mở lại trình duyệt nếu cần.

Không tự gửi lại thao tác cũ. Trạng thái `unknown` có nghĩa lệnh có thể đã tác động tới trang trước khi mất phản hồi. Chỉ tạo thao tác mới sau khi kiểm tra kết quả thực tế trên trang.

Nếu worker báo không chắc Chromium cũ đã đóng, khởi động lại service worker trước khi nhận lại quyền điều khiển. Không chạy thêm replica để vượt qua khóa hồ sơ.

## Sao lưu và khôi phục với Docker volume

1. Đóng trình duyệt và dừng worker trước khi chụp bản sao volume. Sao lưu toàn bộ `/data`, gồm hồ sơ và SQLite.
2. Lưu riêng bản sao state của Aurora, gồm `cloud-browser-namespace` và dữ liệu Vault theo cơ chế sao lưu hiện có.
3. Khôi phục hai phần vào service riêng, giữ đúng khóa worker và namespace. Chỉ chạy một worker ghi vào volume.
4. Mở lại bằng quyền chủ sở hữu và kiểm tra phiên trên ảnh trước khi giao việc.

Đổi khóa worker làm thay đổi namespace được Go dẫn xuất và khóa giải mã bản che giá trị nhạy cảm. Quy trình hiện tại chưa có công cụ chuyển đổi hồ sơ khi đổi khóa. Giữ bản sao cũ cho đến khi xác nhận quá trình chuyển đổi.

Quy trình volume này không áp dụng cho VM Railway hiện tại. Pilot chỉ giữ base checkpoint sạch; không sao lưu/khôi phục hồ sơ thật. Khôi phục backup có dữ liệu tenant đã bị xóa cần một thiết kế erasure riêng trước khi được sử dụng.

## Xóa tài khoản Aurora

Xóa tài khoản có xác nhận sẽ yêu cầu worker purge namespace đã dùng trước khi xóa locator và state của tenant. Tombstone chặn lệnh mới và không cho tái tạo namespace đó. Worker phải chứng minh không còn tiến trình ghi hồ sơ trước khi xóa; lỗi cleanup trả trạng thái chờ để owner thử lại. Không chạy purge thủ công bằng namespace do người dùng nhập và không xóa tài khoản thật để kiểm thử.
