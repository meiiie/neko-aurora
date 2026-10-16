# Dự án và tệp local — UI 2026-10-11

Nhánh UI riêng `ui-project-files-98c457d`, base `98c457d4de4a866606c11a40080a5392e10427a8`. Runtime được đọc và kiểm qua checkout riêng từ Git object `dd86f3f6cf5cdbb428f8c95c5da89d2517c80fcb`. Không sửa Go/Rust/Core, E/main, không dùng provider hay cửa sổ native. Giữ light direction, NekoPeek và font Geist có license.

## Graph và phạm vi

- Nhóm local là origin của chat/reply/tệp kết quả. Dự án cho phép một tập nhóm cụ thể; cùng dự án không nhập lịch sử hay nguồn của nhóm khác. Chọn dự án làm một view riêng trong nhóm, giữ draft/scroll/following riêng. Không gắn dự án hiện các việc không gắn dự án; chọn dự án hiện đúng `work.resources.project.id`. Phiên bản cũ vẫn có thể xem nhưng không dùng để gửi yêu cầu mới.
- Thanh routing giữ account, bot nhận và dự án trên cùng một hàng. Dự án đổi phiên bản hoặc bị loại khỏi nhóm yêu cầu chọn lại rõ ràng; không tự nâng revision. Tệp nguồn và kết quả có phần riêng trong panel Tệp.
- Tạo/sửa dự án qua native dialog: tên, mục tiêu, 1–16 nhóm và expected revision. POST giữ nonce/body khi thử lại; 409 tải metadata và yêu cầu xem lại. Late ACK cập nhật danh sách nhưng không đóng form mới, đổi nhóm hoặc xóa draft mới.
- Upload ghim `projectId/projectRevision/channelId/filename/contentBase64/clientNonce` từ phạm vi đã mở. Chỉ đọc tệp người dùng chọn. Giới hạn 1 MiB/file; queue thử lại chỉ trong bộ nhớ, không ghi bytes/base64/excerpts vào browser storage. Đóng form không xóa tệp server đã nhận. Thử lại tệp gốc giữ bytes, scope và nonce; 400/404/409 không tự đổi phạm vi rồi gửi lại.
- TXT/MD/CSV/JSON UTF-8 là `ready_text`; runtime tự xếp hạng nguồn liên quan trong đúng dự án/nhóm khi intake. UI không giả parser hay ép `fileIds` vào mọi request. PDF/định dạng khác là `quarantined_unsupported`, chỉ tải lại; không được đọc hay dùng làm ngữ cảnh. Caption thể hiện trích đoạn giới hạn nếu runtime báo truncated.
- Download dùng `/v1/projects/{projectId}/files/{fileId}?channelId=...`; kết quả giữ path origin channel của work. UI không dùng owner-wide link để chuyển nguồn sang nhóm khác.
- File access là quyền local, CAS theo `accessRevision` và nonce. ACK chỉ `{updated:true}`, nên chờ SSE/snapshot; không đoán revision. Revoke có thể fence các việc phụ thuộc. Regrant không tự refresh/recover/resume. Pin cũ vẫn giữ access revision/hash/project revision cũ; khi pin khác catalog hiện tại, UI hướng dẫn gửi yêu cầu mới và ẩn context refresh không thể tái cấp quyền nguồn.

## Freshness và dữ liệu

Capabilities `local_projects_v1`/`local_uploads_v1` gate controls. Snapshot có projects/files; SSE project/upload là metadata và được phân nhánh trước Work. Không có GET collection riêng. Unknown zero-Work events chỉ tiến cursor, không tạo ghost task. Event chứa `taskControl.related` (hoặc legacy `relatedWork`) yêu cầu resnapshot để không áp một nửa graph.

Catalog project/file không giảm counter đã quan sát. Upload ACK không thay metadata access mới hơn đã nhận. Public file catalog/SSE không có excerpt; existing `work.resources` và `contextPin.resources` có pinned excerpts trong contract, nhưng UI không hiển thị hay lưu chúng vào browser storage. Nguồn hiển thị bằng tên/hash/trạng thái/phạm vi. Nội dung tài liệu không cấp quyền cho hành động khác.

Mọi intake/outbox giữ thêm project revision bên cạnh account/channel/CAS/nonce. Draft ACK dùng view và generation; đổi dự án, bot, account hoặc đóng/mở form không bị response cũ kéo lại. File actions bị disable/hide giữ focus trong mobile panel, chỉ restore khi cùng panel/view/generation và người dùng chưa tự chuyển focus.

“Tin mới” nằm trong vùng hội thoại, tránh đè lên routing controls khi composer cao. Dialog và panel có scrollport riêng; touch targets 44px và short viewport được kiểm bằng geometry/hit-test, không coi đó là phone acceptance.

## Validation

Runner owned `scripts/ui_project_files_local.mjs`: actual Go journal, byte-preserving upload/download, resource selection và permission fences; headless Chrome. Independent matrix 28 PFUI gate ở `.build/independent-project-files-ui`, riêng với actual Go evidence. Exact code commit/command/environment/assets/binary/harness SHA và PASS/FAIL/BLOCKED sẽ nằm trong JSON receipt sau checkpoint; bản đang triển khai chưa được coi là final PASS.

Giữ scope: ledger relevance không phải model evaluation; fixture artifacts không phải Core output; headless không phải Tauri/physical phone/AT speech/provider/cloud acceptance. Runtime owner giữ native slot và task-controls/finite-collaboration contract cho lát cắt sau.

### Checkpoint và kết quả cuối của lát cắt

Code UI `f3cc6078c100281b3992ebcd7dd561d12d08450f`; integration actual Go riêng `2213385fcf554693e9ef8a6383c9d43d96ef2c81`, runtime base `dd86f3f6cf5cdbb428f8c95c5da89d2517c80fcb`. Bốn embedded assets được đối chiếu byte với source. Go/Rust/Core không đổi trong integration UI.

- Actual Go: project/file 32 PASS, account/channel 27 PASS. Giữ riêng lượt Chrome startup timeout tại integration294; fresh rerun đã PASS, không gộp lỗi setup vào product acceptance.
- UI regression 73 PASS, light pixels 23 PASS trên đúng code f3. Protocol SSE 4 PASS; baseline fb tái hiện thiếu `taskControl.related`, delta resnapshot trước khi render nửa graph.
- Independent endpoint fixtures 28 PASS / 0 FAIL / 0 BLOCKED trên Git objects f3, runtime contract dd86 đã hash-verify. Hai lượt 27/1 trước đó giữ nguyên: harness đọc sai vùng caption, sau đó dùng summary ẩn thay vì nút mở panel. Không sửa product để làm các assertion đó xanh.

Ảnh actual Go đã xem ở desktop, form mobile, transcript mobile và viewport 390×260. Headless geometry xác nhận account/recipient/project và Send 44px, hit-test đúng, font control 16px, panel có scrollport. Direction light đã được duyệt trước đó; controls mới chưa được người dùng duyệt visual.

Residual riêng: khi viewport 390×260, composer nhiều dòng và ba việc suspended, transcript còn 17px. “Tin mới” logical shown nhưng rect 85.11×44 tại y76..120 bị clip bởi transcript y64..81; center hit false. Routing/Send vẫn dùng được và không bị che. Đây không phải bằng chứng visible-overlay acceptance; keyboard reach của control này chưa được kiểm. Affordance cho short view sẽ nối trong lát cắt composer/task-controls tiếp theo, không thay kết quả PFUI26 về các control gửi tin.

Commands/environment và SHA binary/assets/harness/report nằm trong JSON từng suite; frozen manifest/patch chỉ chứa năm path UI/docs/tests so với base98. Native Tauri cho controls mới, điện thoại/keyboard/safe-area thật, AT speech, provider/model và cloud nhiều owner còn chưa nghiệm thu.
