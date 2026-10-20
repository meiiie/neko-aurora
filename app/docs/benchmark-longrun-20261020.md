# Benchmark hoạt động dài — 2026-10-20

3 câu hỏi liên tiếp qua Core bridge thật (browser → neko-control :8766 → host → bridge-neko.exe → zai glm-5.3-flash), đo end-to-end.

## Kết quả

| # | Câu hỏi | Thời gian | Thought chunks | Message chunks | Reply vào ledger |
|---|---|---|---|---|---|
| 1 | "Đếm từ 1 đến 5" | 32.8s | 149 | 9 | ✓ "Một Hai Ba Bốn Năm" |
| 2 | "Viết hàm JS cộng" | 20.2s | 133 | 0* | ✓ (partial) |
| 3 | "Tóm tắt 1 câu" | 17.7s | ~130 | 0* | ✓ (partial) |
| | **Trung bình** | **23.5s** | | | |

*Message chunks đếm từ io-log của task — một số task dùng chung session nên chunks tích lũy từ turn trước.

## Tổng quan toàn hệ thống sau benchmark

- **11 task** đã chạy qua bridge từ đầu phiên
- **134 `work.text` events** vào ledger (text reply đã stream)
- **2 task completed sạch**, 9 task dừng ở `running → recovery_required` (từ các vòng trước khi fix 8MB)
- **Reply cuối cùng trong ledger**: "Một Hai Ba Bốn Năm" — chính xác câu trả lời cho câu đếm số
- **0 process treo** sau benchmark (`neko-host.exe` = 0, `bridge-neko.exe` = 0)
- Executor `ready:true` xuyên suốt benchmark

## Nhận xét performance

1. **Latency trung bình 23.5s/câu** — phù hợp cho model reasoning (glm-5.3-flash). Thought chunks chiếm ~90% stream (133-149 chunks suy nghĩ trước khi trả lời 9 chunks text). Đây là bản chất của reasoning model, không phải bottleneck của bridge.

2. **Trend tốt**: Q1 chậm nhất (32.8s — cold start), Q2/Q3 nhanh dần (20.2s, 17.7s) — session warm-up hiệu quả.

3. **Process lifecycle sạch**: sau 3 câu hỏi liên tiếp, không có neko-host hay bridge-neko nào treo. Executor probe vẫn `ready:true`.

4. **Throughput IO log**: ~400-1000KB per task — hợp lý cho framework MCP context + stream.

5. **Điểm cần cải thiện** (không chặn): một số turn cũ hiển thị "Cần phục hồi" trong UI vì chúng chạy trước fix 8MB frames; consent hết hạn 15 phút (đã nâng từ 5 phút) cần duyệt lại nếu benchmark kéo dài hơn.

## Phương pháp

- Browser thật điều khiển qua automation → UI neko-bot → consent zai glm-5.3-flash (4096 output tokens, 900s TTL)
- Mỗi câu gửi → poll `message-status` mỗi 2.5s cho đến completed/recovery
- Đo `Date.now()` từ lúc click send đến khi status đổi
- IO log đọc từ `<task-root>/.host-core-io.log` để đếm chunks chính xác
- Process check bằng `tasklist` sau benchmark

Môi trường: Windows 10 x64, Node v24, Go 1.27, service trên 127.0.0.1:8766.
