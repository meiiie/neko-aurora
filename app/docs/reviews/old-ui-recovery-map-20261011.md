# Old UI recovery map — 2026-10-11

## Kết luận để implement ngay

Nguồn App c9/485 có đủ shell, roster, composer inline, bubble, bot picker, panel, integrations và bộ nhận diện Neko để phục hồi có chọn lọc. Phần UI nền gần như không đổi giữa hai snapshot. Không có bằng chứng ở lượt này cho phép gọi đây là bản UI thất lạc `d61`, hoặc khẳng định các ảnh tài liệu là ảnh Neko chạy tại c9/485. Hình/video người dùng vừa chọn có quyền quyết định hướng phục hồi; UI worker đang đối chiếu các attachment đó.

Thứ tự triển khai đề nghị:

1. **P0: khóa reference và giữ state fixes**. Đối chiếu attachment được người dùng chọn với ảnh/source dưới đây; giữ sửa ACK khi đổi bot, draft/focus, generation fence khi tạo bot và stale response/error. Đổi skin không được trả lại các race đã tìm thấy.
2. **P1: phục hồi khung chat cũ**. Shell ba vùng, roster nhỏ, header tên bot gọn, transcript rộng thoáng, composer inline bo 26 px, nút tròn; panel phải mở theo yêu cầu. Bảng ở mục 4 cho exact source/line và mapping tới UI Tauri hiện tại.
3. **P1: phục hồi nhận diện có provenance**. Dùng kit Neko Peek light/dark/micro, giữ AGPL và nguồn artwork, Geist OFL. Đừng tự vẽ lại logo cat khác chỉ để giống ảnh.
4. **P2: bot picker/create và account/plugin surfaces**. Dùng cấu trúc cũ, chuyển hành vi qua contract Rust/Go hiện tại; trạng thái chưa hỗ trợ vẫn ghi đúng khả năng.
5. **P2/P3: motion, artifacts, responsive**. Lấy timing thật của source cũ; kiểm tra lại focus, reading position, reduced motion, tiếng Việt. Chỉ mở rộng artifact gallery/preview hoặc computer pane khi host hiện tại thực sự hỗ trợ.

Đây là **bản đồ nguồn và checklist phục hồi**, chưa phải patch UI, render nghiệm thu hay test runtime. Analyst không sửa UI/runtime/index, không chạy bundle/ZIP, không mở app/browser, không tải thêm attachment.

## 1. Phạm vi và khóa nguồn

Mọi path `A/...` dưới đây viết tắt cho:

`E:\Sach\Sua\NekoBot\reference\extracted\app-c9dbbd7\package\app\...`

Mọi path `R/...` viết tắt cho repo hiện tại:

`E:\Sach\Sua\NekoBot\...`

| Nguồn | Commit/identity | Cách đọc và giới hạn |
|---|---|---|
| App c9 extracted | `c9dbbd77813458977002025d05ec72acf91e2efe` | `reference/extracted/app-c9dbbd7/package/app-source-manifest.json`; 2,199 tracked files; SHA-256/Git blob của 31 file chọn lọc được xác minh. Manifest tự ghi development snapshot, không phải release/production acceptance. |
| App 485 ZIP | `485765f195f4c7dad2ce61c087b9af026a4e7b71` | Hai source ZIP trong `reference/archive/original/NekoBot-tat-ca-ban-cu-20261011/02-app-485765f/`; đọc member bằng Python `zipfile`, không chạy hoặc nhập source vào repo. |
| Core paired của hai App | `3d5725f222815e15b12a212e372cf17057a459c0` | Identity trong manifest. Không audit backend hoặc rerun Core trong lượt này. |
| UI Neko baseline | `3cb9b6ff12195b432e1c5c9d69b0db9b56699229` | Bốn web assets tại `R/native/internal/web/assets/{app.js,connections.js,style.css,index.html}` byte-identical với committed blob lúc đọc. |
| Workspace HEAD lúc đọc | `da2ca89c8e461a564d2dc6751ae623bfc7da1f97` | `git rev-parse HEAD`; không đồng nhất HEAD này với commit render ảnh hoặc nhánh UI worker. |
| UI worker | `01a12a58-38b6-77de-a3d4-7f44118a7022` | Worker báo đang sửa tại `task-3/ui-work-3cb9b6f`. Analyst chỉ gửi nguồn/mapping, không thay file của worker. |
| Attachment mới | `libfile_bfc6299f98688191aefad45568c31377`, `libfile_20afb13d1d588191bfbae5895186ba52` | UI worker phụ trách materialize/inspect. Analyst chưa xem nội dung attachment, không tự nhận là ảnh/video đã hiểu. |

Đối chiếu tĩnh của worker song song: App485 có **2,174 source members**; **2,098 giống byte**, **76 thay đổi**, không có file485 bị thiếu ở c9; c9 thêm25 file. Các nhóm `packages/ui-tokens` (9), `packages/ui-web` (41), `packages/chat-ui` (19), `apps/desktop` (50) giống byte toàn bộ. Các con số là file inventories theo nhóm, không phải số test hoặc mức fidelity.

Trong web source, 255 file khác giữ nguyên; phần đổi tập trung vào `NekoSuiteDialog`, `neko-status`, `Shell`, `shell/dialogs`, tests liên quan và locale en/vi. Vì vậy phục hồi token/composer/shared primitives từ485 hayc9 không đòi hỏi rollback toàn bộ App. Shell còn khác integration nên cần chọn từng vùng render, tránh thay cả file7,000+ dòng.

Full comparison được lưu riêng tại `C:\Users\Admin\Documents\Codex\2026-10-11\task-4\app485-c9-static-comparison.json`, SHA-256 `b5f6193e0199554feb6501e0759cae1619c1e01a60301611294dffe93707c782`. Worker xác minh 2,174/2,174 member của485 về byte count, SHA-256 và Git blob SHA-1; fingerprint tính lại đúng `907c9de074aa93f5f8604aa11a72efbb6355224eb942ebd097bdfab7b46b945a`. Đây là kiểm tra byte/manifest, chưa kiểm tra commit objects/history hoặc authorship độc lập.

Archive485 part01 SHA-256 `87e343ed43088d0ffa15e6d38d688ad673383e4da0464af07d6ec3f722b15270`; part02 `876c49de90a79fe0bed44d52e891c04eef457580b634139eb95ef1e4538893d0`. Manifest485 SHA-256 `8ee00508d658efb445954dae83d242bad0809336884cc7e70ab2e44e006dacc1`; manifestc9 `b27406c37341643c7c110471ea32662aba4fe04a170ac064c71376a58dde3b01`.

Các delta nên giữ khi lấy lại view skin:

- `Shell.tsx`: attachment capability/filter/notice; hunk old→new 2050→2052, 2064→2067, 2229→2267, 5955→6010, 6090→6148. Geometry của composer vẫn nhận ra được; không lấy lại attach UI bỏ kiểm tra capability.
- `NekoSuiteDialog.tsx:L161`: provider configured nhưng isolation chưa verified.
- `shell/dialogs.tsx:L365`: clear chat giữ saved memory/files/routines/recovery; copy phải nói đúng tác động.
- `lib/neko-status.ts:L6,L23–52`: trạng thái isolation `none|unverified`, tránh trình bày thành verified.

## 2. Bằng chứng ảnh và ranh giới “UI cũ”

### Những ảnh đã xem bằng đọc file tĩnh

| Asset | Metadata đọc được | Quan sát pixel, không phải CSS đo live |
|---|---|---|
| `A/docs/readme-hero.png` | 1280×640; 594,326B; SHA-256 `1515a0cd275e9cb1ea76288ba4270211f2ff5b128fd9f3e2fbd172b980abb0dd` | Marketing composite Rakazo với phần giới thiệu trắng và mockup ba cột phía dưới. Không dùng headline/marketing composition làm shell sản phẩm Neko. |
| `A/docs/screenshots/web-computer.png` | 2880×1800; 122,812B; SHA-256 `0a1943539c8d30e710411be7d2fd0098214aab0cf0935bc01c74dc45ebff0684` | Sidebar roster, selected Chief với avatar màu; giữa là header bot và hai bubble ngắn, composer inline dưới cùng; bên phải computer screenshot/Take control/Routines. Không thấy Neko Peek trong ảnh này. |
| `A/docs/media/artifacts-tab/list-view.png` | 1600×900; 131,675B; SHA-256 `785329487ff81f57f52676550d93030036f224acf58f65fd1b2a6eb1daf9d209` | Rail nhỏ, danh sách artifact, preview lớn, Download/fullscreen, badge “isolated preview”. Là bằng chứng bố cục ảnh tài liệu; không phải chứng nhận sandbox preview của native Neko hiện tại. |
| `R/.build/evidence/3cb9b6f/desktop-render.png` | 1440×1000; 54,295B; SHA-256 `e57525006d2b859f5428223f7104b6e07edac1fcfd570725a04a79b8ecb6bff6` | Hai cột; Meiiie/Davin avatar chữ cái, welcome text/cards lớn, composer hai hàng ở đáy, nút gửi mint hình vuông bo góc. |
| `R/preview/desktop.png` | 1440×1000; 58,689B; SHA-256 `5fb0d3abed893ba5d0b52462db538ca7c7f8f1a16f6925754368fa58cc9db9f9` | Welcome giữa màn hình, cat icon khác kit cũ, suggestions dạng pill, composer lớn nằm gần welcome; vẫn avatar chữ cái. Không có file evidence tại path này khóa chính xác source commit trong lượt đọc. |

`web-computer.png`, `readme-hero.png` và `docs/screenshots/demo.mp4` ở App485 có bytes giống c9. `A/README.md:L25` giữ `README.upstream.md`, `README.upstream.md:L6` dùng ảnh hero dưới tên Rakazo. Việc ảnh nằm trong snapshot c9 **không xác định thời điểm ảnh được chụp**.

`A/docs/screenshots/demo.mp4` chỉ được đọc bytes/hash: 4,414,968B, SHA-256 `b13dc43c65b648c81d8d748d9a224a948af05104d4be52e2f7ba34b3b19cfb7f`. Không phát video, chưa biết timing của clip; không dùng clip làm evidence motion đã quan sát.

### Những gì có thể và chưa thể kết luận

- **Đọc được:** source cũ có composer inline và optional panel; kit nhận diện Neko có light/dark/micro; ảnh tài liệu cũ có bố cục ba cột.
- **Quan sát được:** hai ảnh Neko ở repo đã xem khác ảnh `web-computer` về avatar, composer, welcome và mật độ chrome.
- **Đề nghị:** giữ khung tương tác cũ và kit hợp lệ, thay renderer bằng data/host contracts mới. Đây là mapping triển khai, không phải đo mức độ người dùng thích từng chi tiết.
- **Chưa xác định:** ảnh/video attachment có đúng snapshot nào, `d61` còn nguồn hay đã mất, ảnh nào người dùng gọi là “bản trước”, capture của App c9 có đúng mọi token hay không.
- **Không tự chọn theo ảnh khi source mâu thuẫn:** `web-computer.png` có user bubble sáng; source c9 dark theme ghi `chatUser="#22242B"`. Nếu người dùng chọn bubble trắng theo attachment, ghi đó là quyết định từ reference ảnh, không gán ngược nó thành literal token c9.

## 3. Reuse và license boundaries

| Nhóm | Nguồn/notice cụ thể | Có thể phục hồi thế nào |
|---|---|---|
| Upstream App/UI primitives | `A/LICENSE`; `A/THIRD_PARTY_NOTICES.md:L5–9`; Rakazo start `2c54c2b6827cc8988cc0ad9c904748db22710eff` | Notice nói nền App, UI primitives và adapters derive Apache-2.0. Giữ license/attribution và ghi file sửa. Không suy ra toàn bộ snapshot/asset mới đều Apache. |
| Neko integration và Core | `A/THIRD_PARTY_NOTICES.md:L13–18`; SPDX trên file Neko mới | AGPL-3.0-only additions. Phục hồi visual/view layer tách với backend cũ nhưng không tuyên bố process separation bỏ nghĩa vụ license. |
| Neko Peek brand kit | `A/apps/web/public/brand/neko-peek/PROVENANCE.md:L5–23`; `A/THIRD_PARTY_NOTICES.md:L20–28` | Artwork từ Wiii `6b1c30b15b4c67e3c841ae870e6989577fc228f8`, canonical `docs/assets/brand/neko-family-v1/`, AGPL-3.0. Kit public có canonical copies+license; giữ editable/export kit từ companion archive khi phân phối. |
| Geist font/outlined wordmark | Brand `PROVENANCE.md:L25–33`; license `licenses/Geist-OFL-1.1.txt`; `A/apps/web/src/styles.css:L3,L15` | Wordmark đã outline, trọng lượng620; font package `@fontsource-variable/geist@5.3.0` OFL1.1. Runtime UI font phải có glyph Việt và license đi kèm; outlined wordmark không thay font văn bản. |
| Bot avatar renderer | `A/packages/ui-web/src/bot-avatar.tsx:L1–34,L169–190` | Hành vi status, size, identity, variant là nguồn UI có thể đọc để map. **Chưa xác minh quyền của từng bảng path/image** chỉ từ LICENSE App. |
| Avatar shape table mang tên Grok | `A/packages/core/src/bot-avatar-shapes.ts:L1–4`; imports/alias `GROK_SHAPES` ở avatar:L28–34 | Provenance riêng chưa thấy trong các notice đã đọc. Không copy bảng path, hình Grok hoặc mượn tên “shipped” làm bằng chứng cấp phép. Ưu tiên cat kit có provenance hoặc independently authored/avatar procedural có nguồn xác minh. |
| Grok extracted bundle/assets | `R/reference/extracted/grok-0.68.1/client/dist` | Chỉ đọc UX/geometry/state như doc Grok trước. Không execute, copy JS/assets/SVG/font không có license rõ. Tên biến Grok trong App cũ không làm cho assets Grok được phép tái dùng. |
| Screenshots/demo clip | `A/docs/...`; upstream README | Dùng như reference quan sát. Không đưa ảnh UI tài liệu vào shipped app như artwork; provenance capture không tự được chứng minh bằng snapshot manifest. |

Kit phục hồi nhận diện rõ nhất:

- `A/apps/web/src/components/neko/NekoBrand.tsx`: hai lockup light/dark, span `sr-only` Neko Aurora; img `alt=""`/`aria-hidden`; compact112×32, thường140×40.
- `A/apps/web/public/brand/neko-peek/nekobot-lockup-{light,dark}.svg`: sidebar/empty state.
- `.../nekobot-mark-micro-{light,dark}.svg`: vùng nhỏ nơi wordmark không vừa; chọn bản optical thay thu nhỏ toàn bộ lockup.
- `.../nekobot-app-icon-{512,192,180}.png`: app/export assets, giữ notice; không tự coi logo inline đang có ở native là cùng nguồn.
- `.../source/upstream-neko-peek-{mark,mark-on-dark,mark-mono}.svg`, `PROVENANCE.md`, `licenses/AGPL-3.0.txt`, `licenses/Geist-OFL-1.1.txt`: giữ cùng kit.

Đã tìm và xác minh companion kit tại:

`R/reference/archive/original/NekoBot-tat-ca-ban-cu-20261011/04-huong-dan-logo-kiem-thu/NekoBot-01-Huong-dan-Logo-Kiem-thu.zip`

SHA-256 `13c2843b964884181d949b3aba5da5e255342802a45412fcbf02568b8bbd3c9d`. Member prefix `package/design/nekobot-brand-v1/` có `PROVENANCE.md`, `README.md`, `VERIFICATION.md`, `source/upstream-*.svg`, editable construction SVG/JSON, `assets/svg/`, `licenses/`, export scripts và `source/fonts/geist-{latin,latin-ext,vietnamese}-wght-normal.woff2`. Mười file public kit c9 (provenance, hai license, ba canonical SVG, hai lockup, hai micro SVG) giống từng byte với companion ZIP. Đọc member/hash, không chạy export scripts hay HTML guide. Nếu cần phục hồi font Việt, có nguồn local trong kit, không cần network fetch.

**Check trước khi đưa file vào product:** chọn đúng variant, xác minh hash source/notice, giữ companion editable kit, record provenance, không sửa ZIP/reference. Không deploy/push/paid calls/secrets/grants trong tác vụ này.

## 4. Recovery checklist — file/component → native Neko

Thông số trong cột source là literal code/CSS. Quy đổi utility Tailwind như `size-8=32px`, `py-3=12px`, `md:px-7=28px`, `max-h-25=100px` theo hệ mặc định 4px là **quy đổi từ source**, chưa phải computed style của app chạy. Explicit `[Npx]`, numeric TS và ms là số đọc trực tiếp. Nếu font-size/root theme khác cần đo lại.

### P1 — Layout, type, spacing, navigation

| Checklist | Source chính xác | Mapping tới native hiện tại và tiêu chí |
|---|---|---|
| [ ] Khôi phục shell ngang ba vùng khi panel mở | `A/apps/web/src/pages/Shell.tsx:L2822–2826,L2885–2897,L3590–3594,L3808–3820`; `ResizableSidePanel.tsx:L6–16,L81` | Native `.app`, `.sidebar`, `main`, `.work-panel` tại `R/native/internal/web/assets/style.css:L2`. Baseline hiện tại là grid 248 + main, work panel dạng fixed overlay 320. Đề nghị dùng roster ở đầu + chat dạng flex + right dock tùy chọn; không bật panel luôn mở. Không copy nguyên khối React shell/RPC cũ. |
| [ ] Sidebar có collapse/restore và responsive overlay | Shell:L2888–2897 collapse→width0/inert; restore:L3615–3625; mobile backdrop:L2877–2883 | Giữ native `#sidebar`, `#nav-open`, drawer focus scope/inert hiện tại. Source cũ có sidebar desktop 316px; mobile `min(100%-48px,316px)`. Nếu attachment chọn rail khác, record số đo từ ảnh; không gán Grok 280 vào App 316. |
| [ ] Roster gọn, tên+preview, avatar đủ nhận diện | Shell:L2999 scroll gap0.5/pad10px; row:L3263 gap12px/pad10px/radius-xl; selection trướcL3144 | Native `#bots`, `.bot-button`, `.bot-label`, `.bot-activity`. Baseline có row min 60px, initials 34px; phục hồi hierarchy tên/preview và selected surface, giữ IDs/aria-pressed/keyboard. Không thêm nhóm/pin/reorder giả nếu API chưa có. |
| [ ] Header bot gọn | Shell:L3603 pad17px dọc/22px ngangdesktop; label:L3647 font16 medium; avatar:L3642 size26; action:L3669 30×34/radius9 | Native `.topbar`, `#bot-name`, `.header-avatar`, `.topbar-actions`. Baseline topbar 76px. Hướng phục hồi là name + avatar/action context gọn; status chi tiết vào panel thích hợp. Giữ trạng thái unavailable/pending nhìn thấy khi người dùng cần hiểu hành động. |
| [ ] Khôi phục dark/light token đồng nhất | `A/packages/ui-tokens/src/index.ts:L51–120,L126,L128`; generated `tokens.css`; `A/apps/web/src/styles.css:L3,L15` | Native `style.css:L1` có palette mint/Segoe riêng. App dark: background #0B0C0E/sidebar #111215/card #141518/border #1E2026/foreground #ECECEE/primary #F1F1EF; light #FAFAF8/sidebar #ECECE9/foreground #1A1A1A. Giữ nguồn token duy nhất, không nhét overlay palette khác. |
| [ ] Khôi phục type thực tế | styles.css:L15 Geist; Shell transcript/composer15.5px và line24px, title16px; labels13–14px | Geist/OFL source hợp lệ hoặc font Việt được user/reference chọn. Kiểm tra "Đồng nghiệp", "Tài khoản", "Đang tạo.", wrapping; không dùng outlined Latin wordmark để hiển thị nội dung. |

### P1 — Transcript và composer

| Checklist | Source chính xác | Mapping/acceptance |
|---|---|---|
| [ ] Giữ transcript thoáng, không thành dashboard giữa chat | Shell Transcript:L5202 gap8px/pad16×20mobile,28×24desktop; frames:L5255–5258 user84%,bot88% với desktop cap trừ8rem | Native `.conversation`, `.messages`, `.turn`, `.message`; hiện max 740/turn margin 30px. Khôi phục bubble grouping/negative space theo attachment, không tự kéo hero/suggestions vào mọi trạng thái có lịch sử. |
| [ ] Bubble user và bot có texture/padding đúng | Shell:L7025–7045: userend,botstart; radius20px/pad18×12; user15.5/1.45,bot15.5/1.5; usertoken/chat-muted | Native `.message.user>p`, `.assistant>p` hiện có user tail 4px và bot plaintext. Đổi visual có chọn lọc, giữ render literal-safe và IDs. Nếu dùng user bubble white theo ảnh, ghi quyết định từ ảnh chứ không gọi là c9 dark token. |
| [ ] Composer idle thực sự inline | Shell:L6137–6143 grid`auto minmax(0,1fr) auto`,radius26px,bg-background,compactpy9px/gap14px/ps12px/pe10px; attach:L6166 size32; send:L6342 size32whitecircle | Native `#composer`, `#prompt`, `.composer-footer`, `#send` hiện có textarea min 66px/2 rows/radius 16/nút 36 vuông mint. Đề nghị compact một hàng khi rỗng/ngắn, textarea mở rộng khi nhiều dòng; menu tác vụ/chế độ giữ semantics foreground/background hiện tại. |
| [ ] Expansion có hysteresis, không rung khi chip xuất hiện | Shell:L334 (single-line36px),L336 (gap14px),L338 (slack16px); đo chiều cao L5611–5664, field span3 khi expanded L6174 | Triển khai đo textarea/toggle class tại native `resizePrompt`; trạng thái dài/chips chuyển field lên trên và controls xuống dưới. Không chỉ dùng CSS cố định một hàng làm mất text/chip. Root không áp lại thuật toán React. |
| [ ] Chiều cao và type textarea | Shell:L6291 rows1;L6292 min24,max-h25(~100),15.5px/24px,minwidth8rem; mobile CSS:L96–98 font16 | Native `#prompt` giữ maxlength 32768/literal data. Thử ngắn, dài, newline, IME, paste và viewport 390. Native hiện có required thuộc textarea: nếu sau này attachment-only send thì contract phải hỗ trợ thật trước khi bỏ required. |
| [ ] Send/stop khác vai trò | Shell:L6314–6334 running vẫn send + riêngStop square;L5570 canSend;L6268 mention-keyresultsend | Native `#send` và task cancel hiện có. Không biến Escape thành cancel runtime; giữ cancel action có tên/identity task. Stop chỉ hiển thị khi runtime đang chạy và endpoint hiện hỗ trợ; late cancel không được làm response của bot khác hiện lên. |
| [ ] Error/reply/attachment nằm cạnh composer | Error:L5973–5993 rolealert/dismiss→focus;reply:L5995;chips:L6024–6035;file:L6145–6157 filtercapability | Native `#hint`, connection sheet và outbox state. Restore inline error/attachment surface bằng native data thực; không giả toolbar khả năng chưa có. Error không tự mất khi đổi unrelated state, dismiss không chiếm focus từ dialog khác. |
| [ ] Giữ reply focus/announcement | Shell:L5924–5955: replytargetchangefocus,clear-setstatus50ms;cancelkilltimer | Là reference hành vi, không map sang pending request animation. Native chưa có reply target: chỉ thêm nếu data/schema hỗ trợ; state fix ACK/draft/focus hiện tại vẫn ưu tiên. |

### P2 — Picker, create, context panels

| Checklist | Source | Mapping/giới hạn |
|---|---|---|
| [ ] Create picker gọn và search riêng | `A/apps/web/src/pages/shell/bot-picker.tsx:L15–87,L47–61` widthmin320/vw-32,input14,avatar22,nomatch | Native `#create-bot`, `#bot-dialog`, `#bot-form`. Có thể tách popup picker chọn existing bot + New bot; giữ danh sách bot thật, không cần kéo dependency Command React vào native chỉ để trông giống. |
| [ ] Popup đóng trước khi mở panel/create | Shell:L2940–2956; menuContent`data-closed:animate-none` | Tránh cùng tồn tại popup và dialog gây focus/z-index race. Native cần generation fence khi dismiss/reopen create; worker đã nhận finding. |
| [ ] Create form đơn giản, async disable/error local | `shell/bot-panel.tsx:L91–128,L130–196`:name,title,description,computerMode,submitting,rolealert | Map name/description/alias theo current native bot contract. Không tự thêm computerMode (team/private) vào backend mới, không dùng old RPC. 409/retry giữ same nonce/draft; error không reset form. |
| [ ] Optional right panel có clamp và keyboard resize | `ResizableSidePanel.tsx:L6–16,L41–46,L81` min320/default384/max1000/reservechat320 | Native work panel 320 overlay hiện tại: dock desktop và sheet mobile là hướng phục hồi. Do source old max formula có floor 320, ở narrow width phải dùng responsive overlay để không ăn chat. Drag width có pointer capture; keyboard separator rules xem source L88 trở đi, verify khi implement. |
| [ ] Computer/routine đúng capability | Shell:L3669computertoggle,L3808panel,L3814pad20×17 | Native hiện có Go/Core tasks, không đồng nghĩa "Agent computer" realtime. Đặt title Công việc/context trong khung cũ; nếu không có computer host thì không hiển thị ảnh desktop giả/Take control/Run now giả. |
| [ ] Artifact list/preview là surface có thể phục hồi sau | `A/apps/web/src/pages/Artifacts.tsx:L46–47,L214–261` list/grid/filter;canceloldlistingL118,L144 | Native current `.artifact` là download/card, không phải HTML preview isolated. Giữ artifact paths/identity và file read host hiện tại; thêm list gallery chỉ khi API có supported listing. External/untrusted HTML không execute để tạo preview. |

### P2 — Account/plugin surfaces, credential form

| Checklist | Source | Mapping/acceptance |
|---|---|---|
| [ ] Integrations browse/accounts/details có hierarchy cũ | `PluginsOverlay.tsx:L797–838` Dialog1080×760,cappedviewport32px,radius16;tabsYouraccounts/Browseapps;detailL584–630 back+identity+accountscard | Native `#connections-dialog`, tabs/account cards/stack form trong `index.html`, `connections.js`. Áp hierarchy vào local provider/accounts/plugins thực; không mặc định sheet 1080px nếu attachment khác. Giữ mobile sheet max-height/focus restore. |
| [ ] Account status rõ và action riêng | Plugins:L358–367 Connected/Waiting/Needsattention/Disconnected;renderaccountsL946,L963;toolsL742–772 | Native local ready không đồng nghĩa OAuth connected; feature disabled vẫn disabled. Reconnect/disconnect/switch không tạo grant mới trong task reference. Catalog/brand logos cũ không tự được import. |
| [ ] Credential input giữ password, clear khi context đổi | ModelSettingsOverlay.tsx:L1236–1237 password/new-password;setApiKeyemptyL433,L502;selectionrevisionL632,L635 | Native `connections.js` đang clear secret và ignore stale account result. Chỉ copy hierarchy/padding; không chuyển secret sang state chat/localStorage/log. Một account có thể đang pending; result late không được đổi account user đang chọn. |
| [ ] Capability limit được nói đúng | ModelSettingsOverlay:L1545 subscription sign-in cannotchoosebetweensavedaccounts yet | Đây là giới hạn của old source, không phải chứng nhận native account support. Giữ status/action theo current host và update fence; task runtime owner phụ trách thực thi. |

## 5. Motion có source thật

| Motion | Source/giá trị đọc được | Quy tắc áp dụng |
|---|---|---|
| Composer compact↔expanded | Shell:L5880 reduced motion;L5888 **120ms, cubic-bezier(.2,0,0,1)**;L5910–5915 capture on screen/cancel old animation | Chuyển từ chiều cao đang nhìn thấy, neo đáy; send/clear draft snaps back theo L5876–5883. Không cố animate empty clear khiến focus/scroll rung. |
| Right panel width | ResizableSidePanel:L81 **150ms ease-out**, tắt width transition khi dragging | Resize bằng pointer không có lag 150ms; chỉ animate khi open/close. |
| Picker info reveal | bot-picker:L115,L143 **150ms opacity**;hover/focus within/focus visible | Focus keyboard phải hiển thị control, không ẩn và chỉ hiện khi hover. |
| Jump latest button | Shell:L5378 **200ms, cubic-bezier(.22,1,.36,1)** opacity/transform/background;reduced motion none | Giữ vị trí đọc lịch sử khi user cuộn lên; không autoscroll với mọi late event. |
| Avatar working ring | `packages/ui-web/src/styles.css:L58–78`: **1.2s, cubic-bezier(.4,0,.2,1), infinite**, vòng chỉ visible working | Chỉ phục hồi renderer/asset có provenance; busy chỉ lấy actual run state. |
| Organic avatar idle/working | ui-web styles:L104–198; idle5.6s,eyes idle5.2–6.8s;family working1.1/1.35/1.6/1.8/2.4s **ease-in-out**;variable seeded patterns | Không dùng table Grok chưa license verified; motion procedural vẫn cần review source/notice. Không đồng nhất với Grok bundle working 2.4s. |
| Oldtaskcard pulse | Shell:L6905 **1.2s ease-in-out infinite**; `styles.css:L45–52` opacity1→.3→1 | Source old là task status pulse. Hiển thị running/needs attention/state thật, thêm reduced motion nếu port card. |
| Reduced motion avatar | ui-web styles:L517–539 animation none + transform/path rest | CSS fallback phải tắt ring/eyes/body, không chỉ tắt panel. |

**Phân biệt với doc Grok:** `grok-ux-state-map-20261011.md` ghi composer actual `chunk-prompt-editor-*`; `wakeDelivery` 1/5/15s là reconciliation timer, không animation. Không trộn App old 120ms composer với Grok 300ms pane hoặc email-editor chunk.

## 6. State invariants cần giữ khi phục hồi visual

Checklist này map findings đã gửi UI worker; đây là yêu cầu hành vi, không tuyên bố oldReact giải quyết mọi case tốt hơn.

- [ ] **Đổi bot giữa lúc gửi:** ACK cập nhật work đúng bot/request; không đè draft bot mới, không tự navigate sang bot khác, không focus composer trong dialog, không snap reading position.
- [ ] **Draft per bot và lane:** draft/scroll/following gắn view identity; switch back khôi phục đúng draft; ACK chỉ clear đúng revision/input đã send. Không dùng một pending boolean làm vô hiệu hóa các bot khác.
- [ ] **Retry/duplicate:** outbox durable giữ nonce; text/lane conflict nói rõ; visual “Đã gửi” chỉ sau ACK, không tự đặt task completed từ animation end.
- [ ] **Create dismiss/reopen:** generation/request fence chặn late old response reset form, close dialog mới, đè error mới hoặc switch bot trái ý.
- [ ] **Refresh/error late:** request generation/revision fence chặn stale account/catalog/bot results; refresh coalesce tránh duplicate fetch/error counter cũ.
- [ ] **Cancel/stop:** action với request identity rõ; running → cancel requested → final state theo host; final response đến muộn lưu đúng thread và hiện đúng state, không assert cancelled chỉ vì UI đóng panel.
- [ ] **Transcript DOM stability:** keyed messages/tasks; event update không replace whole conversation đang focus/selection/scroll; đọc older messages không bị kéo đáy, Jump latest xử lý explicit.
- [ ] **Connections secret safety UX:** password field; clear on provider/account change, dismiss, submit; never chat/outbox; stale result ignored. Kiểm tra cảm nhận focus/error/disabled, không audit backend trùng runtime worker.
- [ ] **Capability honesty:** files/attachments/plugins/computer/memory actions chỉ bật nếu host hỗ trợ thật; disabled import/pending provider vẫn nói đúng. Empty/error/loading/cancel không render thành succeeded từ fake fixtures.
- [ ] **Accessibility/phone scope:** IME Enter không submit; focus order/menu/drawer/dialog hợp lý; mobile 44px hit areas/safe area; phone acceptance cần thiết bị thật, desktop emulation chỉ là bounded check.

## 7. So sánh nguồn cũ với phần đã thay đổi/mất trong ảnh Neko

| Phần | App old evidence | 3cb9/preview quan sát | Phục hồi cụ thể và giới hạn |
|---|---|---|---|
| Logo | c9 `NekoBrand` dùng canonical Peek light/dark/optical kit | Native cat icon/mark khác kit, wordmark text “Neko Aurora” | Restore NekoBrand assets/provenance, không copy Rakazo hero header. |
| Avatar | Shared identity/color/variant/status renderer; old Chief image có round avatar | Meiiie/Davin initials tile | Restore personality/avatar phù hợp attachment; asset rights phải rõ, không tự import Grok SVG. |
| Composer | Inline pill radius 26, small white circle actions, conditional expansion | 2-row box mint send, hero-centered new preview | Restore one-line idle and small controls, still multiline accessible; giữ lane semantics hiện tại. |
| Header/right context | Compact bot header, optional computer/routines dock | Large 76px header + fixed task overlay hidden | Adapt compact header + context dock; context phải hiển thị actual tasks, không giả computer. |
| Empty state | c9 empty only no bot: NekoBrand + create CTA; selected bot Transcript + Composer | Chosen bot welcome with hero/cards | Dùng attachment để quyết định empty behavior; không suy ra c9 có selected bot hero. Không mất state bot selected/message feed. |
| Artifact surface | Old gallery/list + preview image + filters | Card download/preview button có capability giới hạn | Recover metadata hierarchy trước; isolated preview claim cần implementation/tests, tách riêng với visual. |
| Theme/type | Semantic dark/light + Geist, small 14–16 UI | Mint/Segoe, larger welcome headline | Chọn rõ old tokens/type hoặc attachment directive; giữ current architecture. |

Các khác biệt trên **không đủ để suy ra phiên bản thất lạc**. Không thấy Git/source `d61` trong scope này; không đặt nhãn “đã khôi phục d61”. Đích đúng là phục hồi những thành phần cụ thể được attachment/source xác nhận và kiểm tra lại hành vi.

## 8. Evidence/validation và acceptance tiếp theo

### Validation của tài liệu này

Environment: Windows PowerShell; Python chuẩn đọc UTF-8/ZIP/PNG metadata, SHA-256 và Git blob SHA-1. Không npm/pnpm/Bun install, không test/App bundles chạy, không paid provider, không browser/native control.

Lệnh xác minh tài liệu và source:

```powershell
python 'C:\Users\Admin\Documents\Codex\2026-10-11\task-4\old_ui_evidence.py' --root 'E:\Sach\Sua\NekoBot' --output 'C:\Users\Admin\Documents\Codex\2026-10-11\task-4\old-ui-evidence-validation.json' --document 'C:\Users\Admin\Documents\Codex\2026-10-11\task-4\docs\reviews\old-ui-recovery-map-20261011.md'
git rev-parse HEAD
```

Supporting report: `C:\Users\Admin\Documents\Codex\2026-10-11\task-4\old-ui-evidence-validation.json` chứa selected source hashes/manifest matches, selected 485 ZIP member comparisons, PNG metadata, current assets against 3cb9, anchor checks và secret pattern counts. Source 31 files kiểm tra SHA-256 + Git blob theo c9 manifest; các anchor cuối tài liệu kiểm tra exact line/term. Kết quả `PASS` chỉ dành cho **static document/source evidence**, không UI/runtime acceptance. Candidate scan private key/provider/AWS/GitHub token patterns không in values; không Git checkpoint, stage/commit.

### Historical QA không được mang sang UI vừa phục hồi

- Guide ZIP `R/reference/archive/original/NekoBot-tat-ca-ban-cu-20261011/01-phase2-app-c9dbbd7/NekoBot-phase2-c9dbbd7-guide-evidence.zip`, member `package/guide/phase2/evidence/phase2-a36c045-ui-verification.json` (SHA-256 `b3956c059e77e0c1c4383721e6d6175dbd28b4ff5ad991e6b2126cff91e72ebf`) có **browser blocked, zero UI assertions, no fresh screenshots**; focused 19 component tests là historical a36c045, không phải c9 visual pass.
- c9 manifest giữ exact-revision claims; product/Core tests không chứng minh visual/source images capture tại c9. Tài liệu chỉ verify archive evidence hash, không rerun tests hoặc cộng counts.
- `R/.build/evidence/3cb9b6f/ui-headless.json` ghi pass bounded browser checks nhưng `visualAcceptance=screenshots_require_review`, `phone=desktop_browser_emulation_only`, `nativeTauriVisual=not_tested`. Các state checks là 3cb9 contract baseline, không nghiệm thu skin c9 mới.
- `preview/desktop.png` chỉ có PNG hash/quan sát ở lượt này. Người dùng bác preview là feedback direction; không thể báo check passed đồng nghĩa visual accepted.

### Checklist acceptance do UI/runtime owners chạy sau khi patch

- [ ] Lock exact patch commit và source/attachment identity; ảnh desktop welcome, chat short/long, composer expanded, bot picker/create, context panel, connections/error cùng viewport so sánh reference.
- [ ] Check responsive 390px, wide 1440px, reduced motion, light/dark, Việt diacritics; record native Tauri riêng với browser emulation.
- [ ] Re-run only meaningful state cases: switch during ACK, dismiss/reopen create, stale error/result, retry/outbox, late cancel, response wrong thread, draft and focus, scroll reading position.
- [ ] Verify imported assets/license kit và secret pattern scan candidate files trước checkpoint; không include ZIP/bundle assets chưa license.
- [ ] Record exact code commit, command, environment, pass/fail/blocked; không deploy/push/live provider without separate auth.

## Static anchor ledger

Các comment dưới đây là exact path/line/term cho script kiểm tra; chúng không phải code lấy từ bundle hoặc asset copy.

<!-- anchor:apps/web/src/pages/Shell.tsx|2826|relative flex h-full -->
<!-- anchor:apps/web/src/pages/Shell.tsx|2889|max-w-[316px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|2999|gap-0.5 -->
<!-- anchor:apps/web/src/pages/Shell.tsx|3263|py-[10px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|3603|py-[17px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|3647|text-[16px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|3669|h-[30px] w-[34px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|3747|key={inGroup -->
<!-- anchor:apps/web/src/pages/Shell.tsx|5202|md:px-7 md:py-6 -->
<!-- anchor:apps/web/src/pages/Shell.tsx|5255|max-w-[84%] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|5888|duration: 120 -->
<!-- anchor:apps/web/src/pages/Shell.tsx|5976|role="alert" -->
<!-- anchor:apps/web/src/pages/Shell.tsx|6141|rounded-[26px] -->
<!-- anchor:apps/web/src/pages/Shell.tsx|6166|size-8 -->
<!-- anchor:apps/web/src/pages/Shell.tsx|6292|text-[15.5px] leading-6 -->
<!-- anchor:apps/web/src/pages/Shell.tsx|6330|onStop -->
<!-- anchor:apps/web/src/pages/Shell.tsx|6342|bg-white text-black -->
<!-- anchor:apps/web/src/pages/Shell.tsx|7029|bg-chat-user -->
<!-- anchor:apps/web/src/pages/Shell.tsx|7045|rounded-[20px] -->
<!-- anchor:apps/web/src/pages/ResizableSidePanel.tsx|6|MIN_WIDTH = 320 -->
<!-- anchor:apps/web/src/pages/ResizableSidePanel.tsx|7|DEFAULT_WIDTH = 384 -->
<!-- anchor:apps/web/src/pages/ResizableSidePanel.tsx|81|duration-150 ease-out -->
<!-- anchor:apps/web/src/pages/shell/bot-picker.tsx|47|min(320px -->
<!-- anchor:apps/web/src/pages/shell/bot-picker.tsx|85|size={22} -->
<!-- anchor:apps/web/src/pages/shell/bot-panel.tsx|113|!name.trim() || submitting -->
<!-- anchor:apps/web/src/pages/PluginsOverlay.tsx|806|w-[1080px] -->
<!-- anchor:apps/web/src/pages/ModelSettingsOverlay.tsx|1236|type="password" -->
<!-- anchor:apps/web/src/styles.css|15|Geist Variable -->
<!-- anchor:apps/web/src/styles.css|81|min-height: 44px -->
<!-- anchor:packages/ui-tokens/src/index.ts|52|#0B0C0E -->
<!-- anchor:packages/ui-tokens/src/index.ts|62|#22242B -->
<!-- anchor:packages/ui-tokens/src/index.ts|73|#111215 -->
<!-- anchor:packages/ui-tokens/src/index.ts|126|0.75rem -->
<!-- anchor:packages/ui-web/src/bot-avatar.tsx|28|GROK_SHAPES = SHIPPED_BOT_AVATAR_SHAPES -->
<!-- anchor:packages/ui-web/src/styles.css|68|1.2s cubic-bezier -->
<!-- anchor:packages/ui-web/src/styles.css|517|prefers-reduced-motion -->
<!-- anchor:apps/web/src/components/neko/NekoBrand.tsx|7|nekobot-lockup-light.svg -->
<!-- anchor:apps/web/public/brand/neko-peek/PROVENANCE.md|31|Open Font License 1.1 -->
<!-- anchor:THIRD_PARTY_NOTICES.md|18|AGPL-3.0-only additions -->
