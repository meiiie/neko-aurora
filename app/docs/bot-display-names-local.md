# Đổi tên hiển thị bot

Lát cắt SCN-PB-002 / direction §2.2, §2.3. Chỉ sửa tên hiển thị. Stable bot ID, `@alias`, runtime `revision`, description, colour, channel membership và mọi Work/context đã nhận giữ nguyên. Intake mới dùng tên hiện tại; task đang chạy giữ tên trong context đã pin. Không thêm persona, memory reset hoặc permission.

Capability `local_bot_display_names_v1` mở nút Đổi tên trong header private conversation. Shared channel dùng cấu hình nhóm riêng. HTML dialog có tên, alias note và close/save keyboard controls; không hiện trên backend cũ thiếu capability.

`POST /v1/bots/{id}/name` nhận `{name, expectedDisplayRevision, clientNonce}`. Counter bắt đầu 0 cho legacy/create, tách với bot runtime revision. Name trim, không rỗng/control, tối đa 80 UTF-8 bytes, không trùng tên bot khác theo policy case-fold hiện có. Nonce hợp lệ 8..128 ký tự, không dùng lại nonce của bot-create. Stale counter/changed original body/collision trả409; unknown bot404; missing counter/unknown fields400. Cookie/Host/Origin/JSON protections của service vẫn áp dụng.

`bot.renamed` ghi durable journal, clone toàn bộ bot và chỉ cho phép name/counter đổi. Replay kiểm tra lại mutation và uniqueness; caller không được sửa receipt qua returned Events. Same original rename nonce trả current bot, không rename ngược về tên cũ. Original create retry so input của create event, không so tên hiện tại đã đổi. Reply `{bot, duplicate}` không tạo task/grant/context.

UI giữ original body/nonce cho ACK chưa rõ trong page lifetime, read-only name và explicit “Thử lại cùng yêu cầu”. Close/reopen cùng bot giữ lệnh đó. 400/404/409 tải lại và yêu cầu mở form mới; không tự rebind stale revision. Rename pending UI chưa durable qua reload; server receipt durable, reload lấy current name và không tự POST. Đây là metadata mutation, không được mượn durable task-command acceptance.

SSE và ACK chỉ thay metadata nếu display revision không giảm; runtime identity fields khác hoặc cùng counter khác tên bị từ chối. Late ACK luôn nhận metadata hợp lệ nhưng không đổi bot/draft/focus hay đóng dialog mới. Sidebar node được cập nhật bằng ID, giữ focus và history.

Review43 phát hiện snapshot đến muộn từng bypass counter guard, và launcher Desktop từng ghép argument khi state path kết thúc bằng backslash. Follow-up cho snapshot đi qua cùng `acceptBotRecord`; registry hiện chỉ append/rename, nên snapshot cũ cũng không xoá bot vừa được ACK-create. Launcher escape terminal backslashes theo Windows native argument grammar. Giữ hai RED repro và thêm VM snapshot schedules, actual browser held-snapshot gate cùng actual argv-only subprocess; không dùng kết quả code43 để tuyên bố follow-up PASS.

Kiểm chứng mới: HTTP endpoint RED404 trước code; targeted API/ledger/pins/original receipts/replay3; compiled Go + Chrome actual API/nonce/ACK loss/crossed SSE/mobile viewport gates. First browser round thiếu wait work SSE được giữ; helper thêm đúng synchronization và quan sát fields, không sửa assertion hay product để che fail. Packet cuối ghi exact clean source/build/hash/command và pass/fail. Không phải real provider, native OS dialog, physical phone hoặc semantic memory acceptance.
