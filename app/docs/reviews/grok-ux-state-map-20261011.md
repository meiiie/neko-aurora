# Grok 0.68.1: bản đồ UX, state và motion cho Neko Aurora

Ngày phân tích: 2026-10-11. Phạm vi: đọc tĩnh reference do người dùng chỉ định, đối chiếu UI Neko, viết checklist để implementer thực hiện. Không sửa frontend/runtime/index Git, chạy/import bundle, mở browser/native app, dùng mouse, deploy, gọi provider hay tạo quyền.

Baseline Neko đọc được: **`3cb9b6ff12195b432e1c5c9d69b0db9b56699229`**. UI implementer đang làm bản sửa riêng ở task `01a12a58-38b6-77de-a3d4-7f44118a7022`; runtime/integrator ở `01a1299d-c1f3-72db-9e36-eafba745401c`. Các nhận xét về Neko dưới đây áp dụng cho baseline đã đọc, không phải lời xác nhận patch mới đã sửa xong.

## 1. Cách đọc bằng chứng và giới hạn quyền sử dụng

- **[ĐỌC ĐƯỢC]**: giá trị, nhánh state hoặc binding component tồn tại trong file đã đọc. Đây là bằng chứng code tĩnh, chưa phải kết quả đo pixels/hành vi runtime.
- **[SUY LUẬN]**: ý nghĩa UX hoặc rủi ro suy ra từ code. Cần tái hiện trước khi gọi là lỗi đã kiểm chứng.
- **[ĐỀ XUẤT]**: giải pháp Neko có thể tự viết. Không phải thông số của reference.
- **[CHƯA XÁC MINH]**: screenshot, computed styles, timing thực tế, fidelity, keyboard trên điện thoại, provider thật và cloud sync.

Root reference: `E:\Sach\Sua\NekoBot\reference\extracted\grok-0.68.1\client\dist`. Ký hiệu **R/** bên dưới là `renderer/assets/` dưới root này. **N/** là `native/internal/web/assets/` dưới root repo Neko. `L` là dòng vật lý 1-based; `C` là offset 0-based theo Unicode code point sau khi đọc UTF-8 bằng Python, không phải byte offset, UTF-16 JS index hoặc dòng source-map. Nhiều chunk minify có cả component trên cùng một dòng. Dùng tên symbol/selector và offset cùng nhau để tránh nhầm module.

Đã đọc `AGENTS.md` của Neko và `reference/handoff/neko-development-direction.md`; `.agents/skills` không hiện hữu trong checkout đã đọc. Dùng phần audit interaction/state của skill `redesign-existing-projects`; không áp dụng các gợi ý đổi framework/font hay sửa code ngoài phạm vi task. Skill `using-superpowers` đã đọc có chỉ dẫn bỏ qua khi được dispatch một task cụ thể.

**License boundary:** scan tên file trong `reference/extracted/grok-0.68.1/client` không tìm thấy `LICENSE`, `license`, `NOTICE` hoặc `.map`. Điều này chỉ mô tả kết quả scan này, không phải kết luận pháp lý rằng mọi dependency thiếu license. Không thấy giấy phép cho code/assets ứng dụng trong phần reference đã khảo sát; không suy quyền tái phân phối từ quyền đọc archive. Các license của Core/App khác trong repo không cấp quyền cho bundle Grok này.

Bundle có cả tên Grok, Sand, Cursor và AnySphere; tên folder/version không chứng minh mọi thành phần là UX được triển khai của một bản Grok chính thức. Phân tích ưu tiên component `sand-*` được JS ứng dụng binding, tách token/library/commerce/Mermaid khỏi UI chính.

- Không chép JS/CSS/minified implementation, font `X-VF`/`XMono-VF`, icon font `cursor-icons`, logo/provider imagery, avatar, wallpaper hoặc audio vào Neko. Chỉ ghi thông số/hành vi và tự viết component Neko.
- `renderer/index.html` L16–20 preload icon font, L23–29 load entry/eager modules/CSS; L1–42 được đọc như text. Không mở HTML để chạy scripts.
- Không xem ảnh reference hoặc screenshot mới trong task này. **Không khẳng định visual fidelity, màu thực tế đã render hoặc native/phone acceptance.**
- Không thực hiện backend security audit; chỉ xét cách UI thể hiện quyền, danh tính, bí mật và kết quả thao tác.

## 2. Các kết luận cần dùng ngay

1. P0 là **state không cướp điều hướng/focus/draft khi ACK về muộn**, không phải thêm animation. Neko baseline đã có nhiều cơ chế đúng: keyed message reconciliation, nonce outbox, draft/scroll per bot và secret generation fence. Phải giữ chúng qua redesign.
2. Reference ứng dụng có layout sidebar 280 px mặc định, 240–400 px resize, 88 px collapsed, chat tối thiểu 424 px; composer dock tối đa 960 px. Đây là số liệu binding app, khác các design target Neko 248/740 px đang ghi ở tài liệu cũ.
3. State gửi/transport (`pending/queued/failed/accepted/undelivered`) khác phase thực thi và interruption. Một spinner toàn trang không truyền đạt được các trạng thái này.
4. Composer chat thật ở `chunk-prompt-editor-DXTe_qPf.js`; `chunk-view-DH4vRLk4.js` là email draft (`from/to/cc/subject/body`). Không dùng shortcut/email state làm bằng chứng cho chat.
5. Safe-form 1/5/15 giây là **wake reconciliation**, không phải animation duration hoặc timeout mất credential. `cancelReply()` trong editor xóa reply target; không phải hủy runtime task.

## 3. Layout, typography, spacing và navigation

### 3.1 Thông số layout app đọc được

| [ĐỌC ĐƯỢC] | Bằng chứng | Mapping/việc cần làm trong Neko |
| --- | --- | --- |
| Sidebar mở default 280 px; min 240; max 400; collapsed 88. Info pane 320; chat min 424. | R/index.eager-app-aiFqpW8R.js L14 C617385+ (constants/sidebar domain) | N/style.css L2 hiện grid 248 px; chọn token Neko sau khi đo actual screen, tránh gọi 248 là số reference. Có thể giữ 248 nếu chat đủ rộng. |
| Auto-collapse khi window width < expanded sidebar width + 424; shell grid có sidebar width, `minmax(0,1fr)`, pane auto. | R/index.eager-app-aiFqpW8R.js L14 C634625+, C641926+ | [ĐỀ XUẤT] xét chiều rộng content còn lại thay vì một breakpoint duy nhất khi work pane mở. Không tăng native min-size rồi coi là phone support. |
| Composer dock width 960 px; horizontal inset `max(16px,(100% - 960px)/2)`; bottom 16 px. | R/index-ByKx0M82.js L31 C1366113+; R/index-DLJ0gwBH.css L1 C530511, C533987, C625474 | N/style.css L2 hiện composer/messages 740 px. Dùng shared width token, đo độ dài dòng và khoảng trống trên 1220×820 trước khi nới. Composer rộng không bắt buộc mọi đoạn chat rộng 960 px. |
| Bot row mở height54/padding8/full-width; collapsed54×54/padding9. | R/index-ByKx0M82.js L11 C237335–239460, C242180–244300; R/index-DLJ0gwBH.css L1 C601224, C637279 | N/bots giữ tên+alias và target thao tác lớn; row compact vẫn có label/tooltip/focus. Không biến icon-only thành mất danh tính. |
| Selected fill khác hover fill; selected fill vẫn giữ khi hover. Focus ring 2 px inset -2 px. | R/index-DLJ0gwBH.css L1 C477720 (`selected`), C561837 (`hover`), C571256/C571556 (`focus`) | N/app.js L38 đã có aria-pressed. Style selected/hover/focus phải phân biệt bằng hơn màu accent rất nhẹ, kiểm tra keyboard trên cả drawer. |
| App radius overrides 8/10/14/16/18 px; raw CSS defaults 6/8/12/14/16 không phải theme cuối. | R/index.eager-app-aiFqpW8R.js L104–135 C1171450–1173200 | Tự tạo radius scale Neko; nested input chặt hơn modal/container. Không chép nguyên CSS token inventory rồi gọi là computed appearance. |

Thông số sizing liên quan: sidebar drag có hysteresis đóng210/mở220 px (R/index.eager-app-aiFqpW8R.js L14 C634625–635200). Đó là desktop resize, không phải phone drawer. Bot avatar thường36/pinned60 px; body/title/trailing ẩn khi sidebar container≤130 px (R/index-ByKx0M82.js L11 C236580+; R/index-DLJ0gwBH.css L1 C644359–649546).

### 3.2 Typography và composer geometry

| [ĐỌC ĐƯỢC] | Evidence | Checklist Neko |
| --- | --- | --- |
| Sand size scale11/12/13/14/15/17/22/26; line-height16/16/18/20/22/24/28/32; weights400/500/600. | R/index-DLJ0gwBH.css L1 C427157–428010 | [ ] P1 tạo text roles title/body/label/caption; không dùng một size cho tất cả label/state. |
| Composer thực dùng mt.body1: cursor-lg14 px/sand-base20 px, tracking0em; body2=13/18, label12/16, caption11/16. | R/index.eager-app-aiFqpW8R.js L14 C490170–491600; R/index-ByKx0M82.js L33 C1433964; CSS L1 C505853/C518486; R/index.eager-platform-AatQIEPx.js L3 C143029, L13 C519923–521000 | [ ] P1 phân biệt token scale và component usage. Neko15px body/composer là quyết định authored, không gọi body1 reference là15px. |
| Text line20 px/max6 lines120 px; compact shell44 px; expanded top10/sides12/bottom46 px (=10+28+8), action28 px; expand khi newline hoặc scrollHeight≥39 px. | R/index.eager-app-aiFqpW8R.js L60 C1004211–1004875; aliases R/index-ByKx0M82.js L3 C16161–16259; actual usage L33 C1438300–1439000 | [ ] P1 so sánh compact/expanded composer có đủ room cho actions, thay vì chỉ đổi textarea min-height. Neko hiện autoheight tối đa160px (app.js L53), min66px trong CSS. |
| Dock content gap6 px/dock gap8 px; width/insets như bảng3.1. | R/index-ByKx0M82.js L31 C1366113–1367400; CSS L1 C444555 | [ ] P1 thống nhất composer dock/conversation margins; giữ đường viền/space khi empty→multiline→empty. |
| sand_xvf_typeface flag chọn system hoặc XVF/XVF Mono và tracking.01em, sm.015em; avatar gap8→10/title gap0→1/radius10→12. Khi thêm sand_chat_prose_15px, menu row30→32/gap4→6/radius12→16, row radius6→10, palette12→18. Enabled flags chưa biết. | R/index.eager-app-aiFqpW8R.js L104–135 C1171450–1173200; L14 C487804–488250; R/index-ByKx0M82.js L38 C1998400–1998790; CSS fonts C661180–661421 | [ ] P2 giữ system font Neko; chỉ đo type metrics thực tế sau render. Không nhập font bundle thiếu quyền hoặc khẳng định flag đã bật. |

### 3.3 Navigation state

R/index-ByKx0M82.js L15 C454834/C454932 binding row: `aria-busy`, `aria-current`, `aria-description`, `aria-pressed`, `data-active`, `data-agent-id`, `data-awaiting`, `data-composing`, `data-cycle-cursor`, `data-selected`, `isTyping`, `isWorking`, `workingDot`. R/index-ByKx0M82.js L11 C244100–245400 ánh xạ unread sang info và blocked sang needs-attention, có `role=status` và label; indicator thường 8×8, pinned 10×10.

**[SUY LUẬN]** active/selected, unread, đang gõ, đang làm và cần chú ý là những trục độc lập. Đang chạy ở bot A không có nghĩa A phải trở thành bot được chọn. `roster` có create/update/delete/setUnread/setHiddenFromSidebar ở R/index-ByKx0M82.js L15 C732079/C732566; đó không chứng minh Neko đã hỗ trợ nhóm/ẩn/delete hoặc cần implement tất cả ngay.

Checklist navigation:

- [ ] **P0** giữ selected bot do người dùng chọn; activity cập nhật badge bot khác mà không điều hướng. Tiêu chí: gửi A → chuyển B → nhận ACK/reply A; vẫn ở B.
- [ ] **P1** tách selected, running và blocked label. N/app.js L38 hiện chỉ activity cho starting/running; waiting/recovery_required có thể cần attention label riêng, không giả unread khi chưa có receipt đọc.
- [ ] **P1** collapse giữ avatar/accessible name, title/tooltip và focus ring; mouse hover không mở composer khác.
- [ ] **P1** mobile nav giữ Escape/backdrop close, inert main, focus trap/return; N/app.js L57–60 đã có, cần giữ sau redesign. Xác định Escape theo overlay trên cùng.
- [ ] **P1** work pane khác nav drawer; nếu nonmodal thì không dùng aria-modal/focus trap giả, nếu thành modal mobile thì thêm inert/focus-return thật. N/app.js L61–62 mới focus nút đóng/mở.
- [ ] **P2** resize/collapse dùng chiều rộng chat còn lại, không chỉ CSS window max-width. Không suy container breakpoint 130/284/380/560 của reference thành viewport mobile breakpoint.

## 4. Chọn/tạo bot và draft

### 4.1 Reference

R/chunk-new-agent-pane-C7RPhz6v.js L2 C12378: form nhận `isPending,error,onCancel,onCreate`, trim tên, chỉ cho create khi tên có nội dung, không pending, không crop modal. L2 C13369 gần keyboard handler: Ctrl/Cmd+Enter gọi create; L2 C14723/C14984 nút cancel disabled lúc pending, nút create đổi label và disabled theo can-submit. L2 C11454+ có avatar pick/crop busy riêng; avatar editor64 px. Không thấy `maxLength` trong form component này; không suy rằng không có giới hạn backend.

R/chunk-view-C45JaL8U.js L3 C58990 là launcher sửa/thiết lập plugin có thể launchNewAgent qua beginCreation/createAgent/selectAgent/sendPrompt; có isLaunching và failure callback. Không gọi đây là toàn bộ wizard tạo bot chính.

### 4.2 Neko baseline

N/index.html L21: create dialog name≤80, alias2–32, description≤240, form ghi rõ bot không cấp thêm quyền. N/app.js L14/L44: views lưu draft, scroll, following theo bot trong page session; selected bot ID persist trong localStorage. L63–66: create nonce giữ lại khi input không đổi; success thêm roster/reset/close/select.

**[SUY LUẬN: cần tái hiện]** baseline L66 chưa fence generation: đóng dialog/mở lại/đổi nội dung khi POST đang bay rồi late success có thể reset form mới và đổi bot. Đây là risk UI; không kết luận bot đã bị tạo hai lần. Runtime trả receipt mới quyết định kết quả bền vững.

Checklist bot:

- [ ] **P0** generation/view identity cho create dialog. Late ACK cập nhật roster đúng identity nhưng không reset/đóng form mới hoặc select bot khi người dùng đã rời.
- [ ] **P0** quyết định đóng dialog khi pending: hoặc khóa đóng có label rõ, hoặc cho đóng và fence late result. Đóng UI không đồng nghĩa command tạo bot đã bị hủy.
- [ ] **P0** retry cùng input giữ clientNonce; unknown outcome → refresh roster trước khi tạo mới. Lỗi alias/name trùng phải inline tại form, giữ nội dung người dùng.
- [ ] **P1** tạo xong select đúng bot khi còn cùng form generation, focus composer nếu người dùng chưa chọn nơi khác.
- [ ] **P1** mỗi bot giữ draft/scroll/following. Không claim draft được cloud sync: baseline views Map chỉ sống trong page session.
- [ ] **P2** custom avatar chỉ khi có nguồn có quyền; optional asset editor không được chặn bước tạo bot chỉ cần tên/alias.

## 5. Composer, messages, transport và scroll

### 5.1 Reference chat editor thật

| [ĐỌC ĐƯỢC] | Evidence |
| --- | --- |
| Textbox có aria-label, aria-multiline, spellcheck; editor placeholder có aria-placeholder. | R/chunk-prompt-editor-DXTe_qPf.js L8 C84300+; L6 C73298/C73362 |
| Menu mention/skill xử lý Enter/Tab/Escape/Arrow trước editor. Enter không Shift, không IME composition submit; chặn key.repeat; Shift+Enter có list branch riêng. | R/chunk-prompt-editor-DXTe_qPf.js L8 C84977/C85096/C85524/C85608 |
| Escape gọi cancelReply hoặc blur; voice session dùng cancelVoiceAndRefocus. `cancelReply` được parent ánh xạ thành onClearReplyTarget, không phải interruptAgentRun. | R/chunk-prompt-editor-DXTe_qPf.js L8 C85608+; R/index-ByKx0M82.js L33 C1429854/C1431437 |
| Paste file/binary gọi onStageFiles; multiline text có transform riêng, tránh nhầm HTML structured paste. | R/chunk-prompt-editor-DXTe_qPf.js L8 C85933/C86056; L9 C86495 |
| Composer nhận isSendingPaused,isDragOver,attachmentPaths,attachmentNotice,errorNotice,hasContent,isSendDisabled,isChatCovered,replyTarget,slotKey,initialPrompt/richText. Payload có text hoặc attachments; attachment limit6; canSend còn chặn activating/processing/recording/paused. | R/index-ByKx0M82.js L33 C1426881 (`uF=6`), C1427977/C1428387/C1428833/C1429854/C1444166 |
| Mention menu listbox có option ID, aria-selected; ArrowUp/Down vòng danh sách; Enter/Tab chọn. | R/chunk-prompt-editor-DXTe_qPf.js L2 C3752/C5594; L2 C35927 |

**Không nhầm** R/chunk-view-DH4vRLk4.js L2 C13520/C17891: composing-aware Ctrl/Cmd+Enter của email form từ/to/cc/subject/body. Chỉ có thể coi là pattern reusable, không phải shortcut chat.

### 5.2 Transport state riêng

R/index-ByKx0M82.js L31 C1328118/C1328635/C1328695/C1328905: entry/group send state có pending, queued, failed, accepted; accepted vẫn có thể `isUndelivered`. Giữ `ownedFailedNonce`, `ownedQueuedNonce`, `hasInFlightOwner`, `sentWhileOfflineAtMs`, `sendingSinceMs`. Retry/control gắn action owner, không phải mọi viewer đều retry. L31 C1328905+ thêm sending indicator khi timestamp có và transport chưa down.

**[SUY LUẬN]** receipt nhận command, giao delivery và agent hoàn thành là ba sự kiện khác nhau. Không được đổi một POST ACK thành “bot đã trả lời” hoặc một local EventSource open thành “cloud đồng bộ”.

### 5.3 Scroll/messages

R/index-ByKx0M82.js L31 C1168235/C1168657: transcript viewport có pinned-bottom, mounted range, unread divider, holdEntry/scrollToEntry và interruptForUserGesture; L33 C1486098 transcript `role=log`, aria-live off, keyboard focusable, khi phủ bởi chat cover thì inert/aria-hidden. Điều này cho thấy scroll và gesture là state chủ động; không chứng minh phải dùng cùng thuật toán virtualizer.

N/app.js L13/L27–34/L40–46 dùng keyed reconcile; reply text cập nhật cùng node; files theo artifact ID; L55 following dùng ngưỡng64 px và jump-latest. L52 outbox trước send, giữ nonce cho cùng input; maxlength32768 và tối đa16 tin chưa ACK. Đây là số Neko, không phải reference.

**[SUY LUẬN: cần tái hiện]** N/app.js L52 C11742 late message ACK nếu result.work.botId khác bot đang xem sẽ selectBot, đặt following=true, finally focus prompt. Người dùng chuyển bot/đọc tin cũ/mở drawer có thể bị đưa về ngữ cảnh cũ; không phải vấn đề backend dedupe.

Checklist composer/messages:

- [ ] **P0** tách cập nhật store/outbox từ hiệu ứng view. ACK luôn cập nhật đúng request; clear draft/scroll-to-tail/select/focus chỉ khi vẫn đúng view generation và nội dung draft gửi chưa đổi.
- [ ] **P0** gửi A → đổi B → gõ draft B → ACK A: B/draft/focus/scroll giữ nguyên. Stream A không hiển thị ở B. Trả lỗi A không thay status của request B.
- [ ] **P0** IME Enter không send; key.repeat không tạo nhiều nonce; Shift+Enter newline; disable-submit không khiến text mất. Mention menu nếu thêm phải nhận phím trước send.
- [ ] **P0** retry unknown outcome dùng nonce cũ; phân biệt “đang lưu”, “chờ xác nhận”, “đã lưu/chờ runtime”, “đang trả lời”, “không giao được”. Không sửa durable contract chỉ để animation dễ hơn.
- [ ] **P1** theo tail khi người dùng đang following; khi đọc ở trên thì giữ anchor/selection và hiện tin mới. Không bật following vì ACK ở bot khác.
- [ ] **P1** placeholder/label theo bot đã chọn; suggestion chỉ điền draft, không tự gửi khi chưa có lệnh.
- [ ] **P1** long text/code/files không ép overflow ngang toàn page; copy state cần copied/error label và không đổi node đang chọn text. N hiện render reply bằng textContent, chưa phải markdown/rich message renderer.
- [ ] **P2** mention picker chỉ gắn với alias routing hiện có; multi-bot shared context/project invitation là contract riêng, không suy từ @prefix gửi current request.

## 6. Attachments, artifacts và task progress

### 6.1 Reference evidence

- R/index-ByKx0M82.js L31 C1410028/C1410238: staged attachment theo filePath, onRemove và role=listitem. L31 C1411892 attachment collection. L33 C1440957 notice role=status/aria-live polite.
- R/index-ByKx0M82.js L15 C955382: thumbnail skeleton khi source chưa ready; missing có data-state riêng. L15 C948828 image error. Không suy skeleton này là upload network progress.
- R/chunk-file-stage-DArnOiG9.js L2 C2433/C2583: spreadsheet preview loading/error/no rows; đây là parse/view stage, không phải upload controller.
- R/chunk-fullscreen-file-viewer-body-Bh3wEvoz.js L2 C1586/C1903/C10852: loading→ready/error; resource quá lớn có download fallback; failed có retry; missing riêng; parse error có action riêng. Không gom file missing vào generic spinner vô hạn.
- R/index-ByKx0M82.js L15 C1036018: computer progress bar height6, clamped value0..1, text% rounded. L15 C1037477: progress banner title/subtitle, ring22 px; null value spinner, có value mới đặt aria-valuenow 0..100. Đây là computer progress component nhận dữ liệu, không chứng minh mọi task có %.
- R/chunk-view-CGZ4P0Uu.js L2 C23900/C24005: interruption có agentId, khi có session list thì gửi scoped sessionId qua Promise.allSettled. Không suy “interrupt tất cả bot”.
- R/chunk-view-DoQ1QBN9.js L2 C894/C1842/C6994: connector-grant pending/submitting/failed/allowed/skipped và interruptedBy. Đây là permission card, không phải task completed/error timeline. Neko chưa có grant flow này và task này không tạo grant.

### 6.2 Mapping Neko

N/app.js L5 có waiting_host/starting/running/completed/recovery_required/cancelling/cancelled/suspended. L29–30 hiện phase và recover affordance; L33 link `/v1/artifacts/{id}`, download filename/bytes và SHA title. L42–43 work drawer chỉ liệt kê background, cancel ẩn ở completed/cancelled/cancelling; L50–51 cancel/recover có message unknown outcome. Không thấy input attachment upload trong N/index.html baseline; JSON plugin import không phải chat attachment.

Checklist:

- [ ] **P0** cancelling luôn khác cancelled; mất ACK hủy ghi “chưa xác nhận”, giữ partial reply/artifact/receipt đã có. Không animate biến mất nội dung đã lưu.
- [ ] **P0** recovery_required/suspended có label và action đúng phạm vi, không dùng “thử lại” khiến người dùng tưởng hiệu ứng cũ chắc chắn chưa xảy ra.
- [ ] **P1** task card giữ requestId/bot identity khi đổi view; running bot A không khóa quick message cho B. Foreground/background independence là yêu cầu Neko, không kết luận từ nút interrupt generic reference.
- [ ] **P1** không fabricate %. Nếu contract chỉ có work.status thì dùng phase/indeterminate label; progressbar determinate chỉ khi runtime cấp số có ý nghĩa.
- [ ] **P1** artifacts show filename/bytes/saved/download action; preview unavailable không thay nghĩa file đã lưu. Failed download hiển thị lỗi tại file action, cho retry, không đánh dấu task lại failed.
- [ ] **P2, CẦN CONTRACT** input attachments: define staging/ready/failed/removing/cancelled, allowed types/limits/receipt trước khi thêm paperclip. Limit6 là reference, không tự thành giới hạn Neko. Không fake drag/drop đang chưa lưu file.
- [ ] **P2** viewers có loading/missing/too-large/parse-failed/retry/download fallback; chỉ implementation có quyền và có parser hỗ trợ mới hiện preview.

## 7. Credential form, account switching và plugin/connect

### 7.1 Form an toàn của reference

R/chunk-view-DAE5cvtv.js L2 C1448/C1909/C2169: form biết password/OTP; secret numeric fields dùng password+inputMode numeric; email trong login context dùng username autocomplete; password current-password, OTP one-time-code. Đây là browser user-form, không phải Neko API-key/provider form. Không bê policy autocomplete login vào key input.

L2 C15271/C15378/C15439/C15497: sending muted+spinner12 px border1.5, submitted success/check, fill_failed danger, escalated/dismissed muted. Elevated receipt giữ title/site, fields disabled, không echo values; L2 C16943 busy khi sending, radius16.

L2 C20176/C20395: required field trim không trống mới submit; success và close clear values state; error đưa người dùng trở lại action surface. Không thấy bằng chứng mọi closure/payload đã được zeroize; không tuyên bố từ UI code rằng không còn memory copy.

L2 C15664/C19077/C19154: `At=1e3,Mt=5e3,Nt=15e3`, wakeDelivery pending và settled submission effect; schedule delay1s, retry initial5s/max15s/backoff3, AbortController cleanup. Đây là reconciliation của wake sau submit, **không phải thời gian toast/spinner/animation/safe input hết hạn**.

### 7.2 Account lifecycle của reference

R/chunk-provider-credentials-B5alENCV.js L2 C790/C1111/C1516: credential status/directory snapshots có empty/loading/failed và refresh; accounts brokerState connected. L2 C1972/C2017 phân biệt permission/cancelled/denied; không gom user cancel thành lỗi mạng.

R/index.eager-app-aiFqpW8R.js L136 C1179646/C1180178: getStatus/getLoginFlight gắn sequence; active operation identity thay thế thao tác trước, disposed/current checks, result superseded, freshness chống stale snapshot. L136 C1181264/C1181364: login/addAccount/switchCursorAccount(accountId)/cancelLoginFlight/logout riêng. Đây là account đăng nhập ứng dụng Cursor trong bundle; không tự suy Neko đã có provider active-account contract.

R/chunk-plugin-detail-view-Bg5zQ-OF.js L2 C11120: account-keyed detail và role=alert retry. Nhánh này có institution/account/balance mask của finance connector; **không phải bằng chứng provider API-key account picker**.

### 7.3 Neko hiện có và cần giữ

N/index.html L26–29 có tab account/plugin, progressive add form, key password input autocomplete off; mô tả local encrypted/unverified. N/connections.js L4/L15–24: generation + AbortController; đóng/cancel/tab/provider/collapse invalidate và clear secret; save clear input trước fetch, input.secret cleared trong success/catch/finally; refresh checks token/dialog.open. L11 accounts chỉ cấu hình/saved-secret/unverified, remove dùng expectedRevision; đây không phải active provider selection.

**[SUY LUẬN: cần tái hiện]** L18 mở dialog gọi tab(accounts) (đã refresh) rồi refresh nữa; initial error callback ghi account-list thiếu check generation/open. L26 đọc file async trước POST plugin; nếu user đổi tab/đóng trong lúc file.text thì cần xem lại trước POST và feedback. Abort fetch phía UI không chứng minh host write đã bị rollback.

Checklist accounts/plugins:

- [ ] **P0** giữ secret ngoài chat, local outbox và URL; close/tab/provider change xóa field và fence result cũ. Không thêm debug/toast chứa values.
- [ ] **P0** success chỉ ghi “đã lưu local / chưa xác minh”; abort/unknown response refresh metadata trước quyết định retry. User close form không được hiện success lên form mới.
- [ ] **P0** mọi loading/error/success refresh callback kiểm tra cùng generation+open/tab identity; remove duplicate load. Lỗi refresh phải có inline retry trong tab hiện tại.
- [ ] **P1** save pending khóa đúng form/action, giữ field không-secret cần sửa; status của tài khoản A không đè trạng thái tài khoản B.
- [ ] **P1** nếu thêm active account: cần selectedAccountId/revision/receipt ở contract; UI selected/pending/failed/unverified rõ, thay provider clears secret. Không tô tài khoản thành đang được runtime dùng khi chưa được admit.
- [ ] **P1** remove local khác revoke provider grant. Giữ nhãn đúng phạm vi; account delete không tự cancel mọi bot hoặc xóa conversation.
- [ ] **P1** plugin card tách built-in available, imported manifest_only_disabled, auth-needed, failed. Không dùng một connected badge cho cả catalog/install/auth/execution capability.
- [ ] **P1** plugin import loading/error inline, file input reset cho phép chọn lại cùng file; check generation trước POST sau file.text. Cancel/close late result chỉ cập nhật store phù hợp, không tái mở dialog.
- [ ] **P2, CẦN CONTRACT** auth/connect/reconnect action phải có handler/handoff/status thật; unavailable thì giải thích ngắn và không render nút chết. Không làm OAuth/grants/remote MCP chỉ để giống reference.

R/chunk-view-C45JaL8U.js L3 C59656/C59723/C59855 phân biệt effectivePlugins, servers, catalog, skills, nativeInstalled, authBlocked, isBusy, isInstalledExpanded, onAuthenticate/onFixHandedOff/onFixFailed. Mô hình nhiều chiều này là bài học UX; N/connections.js L13 hiện chỉ hai trạng thái honest availability/disabled manifest. Không thay bằng marketplace giả hoặc chức năng connect chưa có.

## 8. Motion và reduced motion

**Đã xác định application motion:** R/index.eager-app-aiFqpW8R.js L14 C641926+ đặt data-sidebar-animating. R/index-DLJ0gwBH.css L1 C573858–574186 transition grid-template-columns **200 ms cubic-bezier(.22,1,.36,1)**. Điều kiện collapsed/animating quan trọng như duration; không chạy layout transition lúc boot/hydrate/reconcile messages mỗi event.

| Application component | [ĐỌC ĐƯỢC] duration/easing/lifecycle | Evidence |
| --- | --- | --- |
| Sidebar/grid, bot width/padding/transform/body opacity/flex-grow | 200ms cubic-bezier(.22,1,.36,1); row reduced motion transition0 | CSS L1 C573858–574186; C541823/C546396/C548366/C549139/C588292–588408; R/index-ByKx0M82.js L11 C237335+ |
| Info pane enter/exit | Translate inline100%→none; 300ms cubic-bezier(.2,0,0,1); close linger300ms; closing pointer-events none, border0.5px; settled transition none | R/index-ByKx0M82.js L15 C706480–707900; CSS L1 C540474/C543943/C548451/C550112/C534628/C487131/C547411 |
| Typing dots | 1.4s ease-in-out infinite; opacity.35→1/translateY0→−2px; delays−.32/−.16/0s | R/index.eager-app-aiFqpW8R.js L14 C644650–645950; CSS L1 C417114–417227/C459275/C454971/C455034 |
| Working dots | 2.4s ease-in-out infinite; opacity.3→.9; delays−.6/−.3/0s | Same APP region; CSS L1 C414141–414200/C458829/C455097/C455159 |
| Working badge | 3 dots1.4s ease-in-out/opacity.35→1; reduced motion static | R/index.eager-app-aiFqpW8R.js L14 C493700–495600; CSS L1 C416087–416149/C456573/C585405–585518 |
| Shimmer text | 2.2s steps(60), gradient position200%→−200%, pause supported | R/index.eager-app-aiFqpW8R.js L14 C507560–508330; CSS L1 C413584–413672/C457094/C465401/C464297 |
| Reduced motion | Shimmer no gradient/no animation+secondary text; working dots static opacity.5; typing opacity-only fallback1.6s; pane duration0/property none | CSS L1 C584375–590219, specifically C585556/C586623/C586736/C587311/C588292/C589386 |

CSS ở bảng này là R/index-DLJ0gwBH.css. Neko baseline dialog/work-pane160ms ease-out, mobile nav180ms ease, status breathe1.8s là authored values (N/style.css L2–5), không phải reference. Neko blanket reduced-motion rule mạnh hơn fallback per-component của reference. Không lấy default spring/stiffness/duration trong motion library làm thông số sản phẩm. Linger pane300ms nghĩa DOM closing còn tồn tại; [ĐỀ XUẤT] khóa interaction ngay khi close rồi restore focus sau lifecycle tương ứng nếu Neko dùng hiệu ứng đó.

Checklist motion để implement ngay:

- [ ] **P1** hover/pressed/focus state luôn nhìn thấy dù reduced-motion; ưu tiên opacity/color và transform ngắn. Không dựa duy nhất vào pulse để báo đang làm.
- [ ] **P1** nav collapse có enter/exit lifecycle và duration bounded; close overlay xong focus return đúng trigger. [ĐỀ XUẤT] Neko200ms/cubic-bezier(.22,1,.36,1) nếu measured pixels phù hợp, hoặc giữ authored180ms có lý do.
- [ ] **P1** dialog/drawer không animate lại khi refresh list/streaming; keyed node và field focus phải ổn định. Loading/terminal state thay content, không reset identity.
- [ ] **P1** prefers-reduced-motion tắt loop/translation/layout tween, giữ static status label. N/style.css L7 hiện đã animation:none/transition:none/scroll-behavior:auto toàn cục.
- [ ] **P2** busy icon loop không giả determinate progress; spinner duration không thành timeout request. Dismissed/superseded không dùng error shake/red toast.
- [ ] **P2** verify computed transition duration/easing trên bản Neko mới và ghi actual command/env/commit; có CSS declaration không chứng minh animation thật chạy đúng lifecycle.

## 9. Thứ tự triển khai và acceptance thực tế

| Ưu tiên | Work package có thể nhận ngay | Source/contract | Cổng cần chứng minh |
| --- | --- | --- | --- |
| P0.1 | ACK/view generation cho message + create bot; secret/refresh callback fence | N/app.js L52/L66; N/connections.js L15–26 | Deterministic delayed ACK trên A→B/new form/close; store vẫn cập nhật đúng, UI không bị kéo đi |
| P0.2 | Status transport vs durable work; unknown cancel/recover; same nonce retry | N/app.js L5/L8–11/L49–52 | Disconnect/reconnect/retry/cancel late; không duplicate request và không mất partial output |
| P1.1 | Shared chat/composer spacing, sidebar selected/activity/attention, keyboard focus | R layout+row bindings; N/style.css | Actual screenshots + measurements tại1220×820,900px,390px; user review visual còn riêng |
| P1.2 | Accounts/plugins progressive form, real loading/error/retry, honest readiness | N/connections.js; existing local contract | Keyboard tabs, provider switch while pending, remove revision conflict, closed tab late load |
| P1.3 | Composer IME/repeat/menu priority, scroll anchor, file action feedback | R actual prompt editor+transport; N/app.js | IME composition, selection while stream, read above tail, artifacts download/retry |
| P1.4 | Drawer/dialog lifecycle + reduced motion | R app selector motion; N media rules | Repeated open/close/Escape/focus restore; reduced-motion screenshot/static labels |
| P2 | Attach input/viewer, active provider account, unread/read receipt, groups/projects | API/host/UI contract extension needed | Không claim support trước receipt/admission và acceptance riêng |

Mỗi test outcome phải ghi commit code đầy đủ, command, environment, pass/fail/blocked, artifact/report. Fixture là fixture; journal/replay local không là cloud sync; headless390px không là điện thoại thật; browser pixels không là native WebView2 acceptance.

Suggested cases cho UI worker (đây là checklist, **chưa chạy trong task review**):

- [ ] Dùng delayed POST response; sendA→switchB→typeB→ACKA; giữ B/draftB/scroll/focus, A có receipt.
- [ ] Create form1→close→open form2→edit→ACKform1; roster có form1 nhưng form2 giữ nguyên.
- [ ] Connection form submit→provider switch/tab switch/close→late success/error; secret trống và view mới không nhận stale feedback.
- [ ] Open accounts→switch plugins→late load failure; chỉ tab hiện hành được status. Không có duplicate initial refresh.
- [ ] Send với IME, hold Enter, Shift+Enter; một gesture gửi tối đa một nonce; không mất draft khi storage fail/outboxfull.
- [ ] Scroll lên/chọn text khi reply streaming và resize/openwork; không bật following; jump latest rõ. Switch back bot khôi phục draft/scroll.
- [ ] Cancel đang chạy→networkdrop→late reply/receipt; cancelling/unknown khác cancelled, artifacts đã lưu còn downloadable.
- [ ] Accounts/plugins empty/loading/error/saved-unverified/disabled-manifest; mọi button có action thật hoặc disabled reason.
- [ ] Nav/work/dialog repeated keyboard close; overlay trên cùng xử lý Escape; focus về trigger nếu view vẫn tương ứng.
- [ ] Reduced motion giữ mọi status; long alias/file/endpoint/paragraph không làm horizontal overflow; phone keyboard/safe area cần thiết bị thật sau.

## 10. Validation record của task read-only này

Environment: Windows executor, PowerShell; Python **3.13.7**. Root đọc: `E:\Sach\Sua\NekoBot`; workspace ghi phân tích: `C:\Users\Admin\Documents\Codex\2026-10-11\task-4`.

| Check | Exact command/cách chạy | Outcome và giới hạn |
| --- | --- | --- |
| Baseline commit | `git rev-parse HEAD` tại root đọc | PASS:3cb9b6ff12195b432e1c5c9d69b0db9b56699229; không commit/index mutation |
| Git working-tree status | `git status --short`; `git -c core.bare=false -c core.worktree=E:/Sach/Sua/NekoBot status --short` | BLOCKED: executor trả must-be-run-in-a-work-tree; explicit --work-tree cũng gặp cwd permission. Không gọi repo clean dựa trên scan này. Baseline clean là handoff của integrator, không validation độc lập của task này |
| Read rules/path | `Get-Content -LiteralPath AGENTS.md`; `Test-Path -LiteralPath docs/reviews/grok-ux-state-map-20261011.md` | PASS: rules đọc; đích chưa tồn tại trước tạo (False). Không overwrite file có sẵn |
| Reference inventory | `rg --files -uu reference/extracted/grok-0.68.1/client/dist` | PASS: đọc inventory. Entry+CSP được đọc dưới dạng text; không execute |
| License/maps inventory | `rg --files -uu reference/extracted/grok-0.68.1/client -g '*LICENSE*' -g '*license*' -g '*NOTICE*' -g '*.map'` | PASS scan/NO MATCH; rg exit1 chỉ nghĩa không match, không xác nhận license permission |
| Static contexts | `python C:/Users/Admin/Documents/Codex/2026-10-11/task-4/inspect_static.py <relative-file> <literal-patterns> --start <offset> --radius <N> --limit <N>` | PASS: đọc UTF-8, tính L/C. Một lần in tiếng Việt bị cp1252 UnicodeEncodeError; sửa output JSON ensure_ascii=True và rerun. Không dùng byte/JS UTF-16 offsets |
| Source SHA | `Get-FileHash -Algorithm SHA256 -LiteralPath <selected-files>` | PASS: primary reference + Neko file hashes bên dưới, snapshot xác định được |
| Evidence anchors + deliverable | `python C:/Users/Admin/Documents/Codex/2026-10-11/task-4/verify_review.py --root E:/Sach/Sua/NekoBot --document C:/Users/Admin/Documents/Codex/2026-10-11/task-4/docs/reviews/grok-ux-state-map-20261011.md --output C:/Users/Admin/Documents/Codex/2026-10-11/task-4/static-evidence-validation.json` | PASS:44 literal anchors+17 source regions; 4 Neko files byte-match `git show <baseline>:<file>`; 0 candidate secret-pattern matches. Không là UI test. Initial checker FAIL do tìm số54/39 trong JS StyleX, sửa checker để trace class→CSS và expression20×2−1; rerun PASS |
| UX/runtime tests/screenshots | Không chạy | NOT RUN theo scope. Không claim pixel/native/phone/provider/cloud acceptance |

SHA-256 của source đã đọc (lowercase tương đương uppercase output):

| File | SHA-256 |
| --- | --- |
| R/index-ByKx0M82.js | df12e610a991666f90081984460c22636409f4a80a7c690ef94f1b327092c8df |
| R/index.eager-app-aiFqpW8R.js | eacc26cdb08b53e7ad5e1543ef4b76da377ecc1d833b4d62e1f170fb00169eee |
| R/index-DLJ0gwBH.css | 1449959e247b6413b5ced6559c91d4d0821e4fab79ff4e09ac7cb61008c3800c |
| N/app.js | e64fceee4f95893091ea970fb0ccd2444e2f6c0457a8c531e9686cd65a4e3d8e |
| N/connections.js | 99c79407cc39df4f0e25b9ae7e72c6c330d4a4c29962758eb7f41cba419c80b3 |
| N/style.css | 849679d31a25af463aab90a347c3d1f96023215acd9c31860b953bbeea28c86b9 |
| N/index.html | 73f47a6306dbcfed8074cb68361ac7b37ba7e0f3622e05ca85e8f92b0b4e8688 |

## 11. Handoff và các nhầm lẫn cần sửa ở tài liệu cũ

Chỉ sở hữu `docs/reviews/grok-ux-state-map-20261011.md`. Không sửa `docs/ux-reference-mapping.md` của worker khác; đề nghị implementer sửa các mô tả sau trong tài liệu của họ:

- Email composer evidence→actual prompt editor (section5).
- Safe-form1/5/15s “thresholds”→wake reconciliation, không motion (section7).
- `cancelReply()`→clear reply target, runtime interrupt riêng (section5/6).
- Plugin detail finance account≠provider key account switching (section7).
- Raw theme/library/commerce responsive tokens≠app computed pixels; target248/740 là Neko authored design, reference app280/960 là thông số đọc được (section3).

Phát hiện được gửi sớm tới UI qua `mcp__codex_app__send_message_to_thread` (cloud_threads.send_message không hiện trong active tools), tool trả threadId thành công. UI phản hồi đang sửa message ACK/draft/focus race, bot generation fence và duplicate/stale refresh. Chỉ là handoff/acknowledgement, chưa là validation patch. Thử gửi source parent `01a118a0-97fb-7590-8a6e-8c734519c0ed` qua cùng tool trả thread-not-found; kết quả cuối được bàn giao bằng final của task.

Remaining gates: patch implementation/test bởi UI worker; actual screenshots và review visual; native window slot phối hợp integrator; real phone; true provider quality/capability và cloud sync theo authorization riêng. Tài liệu không tự đóng các gates này.

Không tạo local Git checkpoint nên không có commit tài liệu trong task này. Candidate document được scan các dấu private key, provider/GitHub/AWS token mà không in matched values; kết quả0 pattern match chỉ là scan hẹp của tài liệu tự viết, không phải audit secrets của repo. File được tạo bằng exclusive create sau kiểm tra path chưa tồn tại; hash copy và final anchor verification được ghi trong report ngoài repo của task4.
