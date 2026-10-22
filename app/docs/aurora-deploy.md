# Neko Aurora — triển khai (deploy readiness, chưa tốn tiền)

## Biến môi trường
> Tên cũ `DUET_*` vẫn được đọc nếu chưa đặt `AURORA_*`.

| Biến | Bắt buộc | Ý nghĩa |
|---|---|---|
| `ZAI_API_KEY` | có (để có AI) | Khoá z.ai. Thiếu thì Aurora chạy ở chế độ không AI. Không bao giờ được log. |
| `AURORA_AI_MODEL` | không | Mặc định `glm-5.3`. |
| `AURORA_AI_BASE_URL` | không | Mặc định `https://api.z.ai/api/coding/paas/v4`. |
| `AURORA_PUBLIC_HOST` | có khi deploy | Tên miền công khai, ví dụ `neko-aurora.up.railway.app`. Chỉ `/app/` được phục vụ trên host này. |
| `AURORA_OWNER_CODE` | có khi deploy | Mã đăng nhập của chủ, ≥12 ký tự ngẫu nhiên (`openssl rand -base64 18`). |
| `AURORA_SMTP_HOST` / `AURORA_SMTP_PORT` | không | Ví dụ `smtp.gmail.com` / `587` (STARTTLS) hoặc `465` (TLS). |
| `AURORA_SMTP_USER` / `AURORA_SMTP_PASS` / `AURORA_SMTP_FROM` | không | Tài khoản gửi. Thiếu thì email đã duyệt nằm ở trạng thái "queued", không gửi. |
| `AURORA_BRIEF_LOOP` | không | `off` để tắt bản tin buổi sáng tự động. |
| `PORT` | do nền tảng cấp | Khi có `AURORA_PUBLIC_HOST` + `AURORA_OWNER_CODE`, server bind `0.0.0.0:$PORT`. |

An toàn: khi bind 0.0.0.0, mọi kết nối không phải loopback chỉ tới được host công khai `/app/` (sau đăng nhập mã chủ) và `/healthz`; toàn bộ control plane khác trả 403. Email chỉ gửi khi chủ bấm **Duyệt** trên thẻ, và chỉ tới địa chỉ chính chủ đã gõ trong luồng đó.

## Railway (gói miễn phí / trial, không cần thẻ để thử)
1. railway.com → New Project → Deploy from GitHub repo `meiiie/neko-aurora` (nhánh `feat/neko-aurora-mobile` hoặc sau khi merge). `railway.json` + `Dockerfile` ở gốc repo tự được dùng.
2. Settings → Networking → **Generate Domain** → chép tên miền vào `AURORA_PUBLIC_HOST`.
3. Variables: thêm `ZAI_API_KEY`, `AURORA_OWNER_CODE`, (tuỳ chọn) `AURORA_SMTP_*`.
4. Add Volume, mount path `/data` (giữ agent, lịch sử, khoá VAPID).
5. Deploy → mở `https://<domain>/app/` trên iPhone → nhập mã chủ → Chia sẻ → "Thêm vào MH chính" để có push.

Chạy local bằng Docker:
```
docker build -t neko-aurora .
docker run -p 8080:8080 -v neko-data:/data -e AURORA_PUBLIC_HOST=localhost:8080 -e AURORA_OWNER_CODE=... -e ZAI_API_KEY=... neko-aurora
```

## Gmail SMTP bằng App Password
1. Bật Xác minh 2 bước: myaccount.google.com → Bảo mật → Xác minh 2 bước.
2. Mở myaccount.google.com/apppasswords → đặt tên "Neko Aurora" → Tạo → chép mã 16 ký tự (bỏ dấu cách).
3. Đặt biến: `AURORA_SMTP_HOST=smtp.gmail.com`, `AURORA_SMTP_PORT=587`, `AURORA_SMTP_USER=<địa chỉ Gmail>`, `AURORA_SMTP_PASS=<app password>`, `AURORA_SMTP_FROM=<cùng địa chỉ Gmail>`.
4. Thu hồi bất cứ lúc nào tại cùng trang App passwords. Gmail thường giới hạn khoảng 500 thư/ngày.
Lưu ý: một số nền tảng chặn cổng SMTP ra ngoài trên gói miễn phí (Railway chặn SMTP trên gói Free/Trial; cần gói Hobby) — khi đó thư ở trạng thái "queued"/"failed" trên thẻ, không mất dữ liệu.
