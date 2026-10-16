# Bounded final gap audit

`gap-audit.mjs` chạy 8 ca mới trên immutable Git object assets bằng Chrome headless profile riêng và Node loopback fixture. `gap-confirm-audit.mjs` chỉ xác nhận hai nhóm mất focus đã phát hiện. Các runner độc lập với config/existing tests; không dùng package mới hoặc credentials.

```powershell
& 'C:\Program Files\nodejs\node.exe' .\tests\mobile-audit-20261011\gap-audit.mjs --source '<authorized UI checkout>' --output '<new owned evidence directory>' --commit '<full immutable commit>' --committed true
& 'C:\Program Files\nodejs\node.exe' .\tests\mobile-audit-20261011\gap-confirm-audit.mjs --source '<authorized UI checkout>' --output '<second new owned evidence directory>' --commit '<full immutable commit>' --committed true
```

Chọn output mới mỗi lượt để giữ lịch sử evidence. Node runner mặc định Chrome tại `C:\Program Files\Google\Chrome\Application\chrome.exe`; có thể override `--chrome` bằng executable đã được cho phép. Sandbox trên máy audit cần escalation cho Chrome riêng và source Git object read; chưa chạy trên OS khác.

Exit `0`: assertions pass; `1`: assertion failure; `2`: execution blocked. Full JSON ghi commit, source hashes, harness/cases hashes, environment, command và từng measurement. Technical gates và behavior cases được đếm riêng trong report để tránh gọi 13 gates thành 13 hành vi.

Commit đã audit: `12ef2f5078d40b483c9d3e67489f51845a3a9f9c`: primary `11/2/0`, confirmation `5/2/0` (pass/fail/blocked), hai defect groups. Xem `docs/reviews/mobile-accessibility-final-gap-20261011.md`.

Viewport resize, coarse pointer và CDP key events là mô phỏng. Chưa nghiệm thu bàn phím vật lý/OS điện thoại, phone IME/AT/safe-area/lifecycle, native Tauri, actual Go font routes/CSP, Core/provider hoặc authenticated multi-host sync. Fixture font CSP chỉ dùng cho audit browser này.
