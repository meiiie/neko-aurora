# Neko Duet — triển khai (deploy readiness, chưa tốn tiền)

## Biến môi trường
| Biến | Bắt buộc | Ý nghĩa |
|---|---|---|
| `ZAI_API_KEY` | có (để có AI) | Khoá z.ai. Thiếu thì Duet chạy ở chế độ không AI. Không bao giờ được log. |
| `DUET_AI_MODEL` | không | Mặc định `glm-5.3`. |
| `DUET_AI_BASE_URL` | không | Mặc định `https://api.z.ai/api/coding/paas/v4`. |
| `DUET_PUBLIC_HOST` | có khi deploy | Tên miền công khai, ví dụ `neko-duet.up.railway.app`. Chỉ `/duet/` được phục vụ trên host này. |
| `DUET_OWNER_CODE` | có khi deploy | Mã đăng nhập của chủ, ≥12 ký tự ngẫu nhiên (`openssl rand -base64 18`). |
| `DUET_SMTP_HOST` / `DUET_SMTP_PORT` | không | Ví dụ `smtp.gmail.com` / `587` (STARTTLS) hoặc `465` (TLS). |
| `DUET_SMTP_USER` / `DUET_SMTP_PASS` / `DUET_SMTP_FROM` | không | Tài khoản gửi. Thiếu thì email đã duyệt nằm ở trạng thái "queued", không gửi. |
| `DUET_BRIEF_LOOP` | không | `off` để tắt bản tin buổi sáng tự động. |
| `PORT` | do nền tảng cấp | Khi có `DUET_PUBLIC_HOST` + `DUET_OWNER_CODE`, server bind `0.0.0.0:$PORT`. |

An toàn: khi bind 0.0.0.0, mọi kết nối không phải loopback chỉ tới được host công khai `/duet/` (sau đăng nhập mã chủ) và `/healthz`; toàn bộ control plane khác trả 403. Email chỉ gửi khi chủ bấm **Duyệt** trên thẻ, và chỉ tới địa chỉ chính chủ đã gõ trong luồng đó.

## Railway (gói miễn phí / trial, không cần thẻ để thử)
1. railway.com → New Project → Deploy from GitHub repo `meiiie/neko-bot` (nhánh `feat/neko-duet-mobile` hoặc sau khi merge). `railway.json` + `Dockerfile` ở gốc repo tự được dùng.
2. Settings → Networking → **Generate Domain** → chép tên miền vào `DUET_PUBLIC_HOST`.
3. Variables: thêm `ZAI_API_KEY`, `DUET_OWNER_CODE`, (tuỳ chọn) `DUET_SMTP_*`.
4. Add Volume, mount path `/data` (giữ agent, lịch sử, khoá VAPID).
5. Deploy → mở `https://<domain>/duet/` trên iPhone → nhập mã chủ → Chia sẻ → "Thêm vào MH chính" để có push.

Chạy local bằng Docker:
```
docker build -t neko-duet .
docker run -p 8080:8080 -v neko-data:/data -e DUET_PUBLIC_HOST=localhost:8080 -e DUET_OWNER_CODE=... -e ZAI_API_KEY=... neko-duet
```

## Gmail SMTP bằng App Password
1. Bật Xác minh 2 bước: myaccount.google.com → Bảo mật → Xác minh 2 bước.
2. Mở myaccount.google.com/apppasswords → đặt tên "Neko Duet" → Tạo → chép mã 16 ký tự (bỏ dấu cách).
3. Đặt biến: `DUET_SMTP_HOST=smtp.gmail.com`, `DUET_SMTP_PORT=587`, `DUET_SMTP_USER=<địa chỉ Gmail>`, `DUET_SMTP_PASS=<app password>`, `DUET_SMTP_FROM=<cùng địa chỉ Gmail>`.
4. Thu hồi bất cứ lúc nào tại cùng trang App passwords. Gmail thường giới hạn khoảng 500 thư/ngày.
Lưu ý: một số nền tảng chặn cổng SMTP ra ngoài trên gói miễn phí (Railway chặn SMTP trên gói Free/Trial; cần gói Hobby) — khi đó thư ở trạng thái "queued"/"failed" trên thẻ, không mất dữ liệu.
