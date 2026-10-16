# Giao diện sáng theo ảnh và video người dùng — 2026-10-11

Bản này nằm trên nhánh riêng `ui-light-reference-f6`, bắt đầu từ `f6f30cb4707374ab1fe22600a09e5ced69d26b74`. Người dùng đã từ chối hướng graphite/mint của f6. Bản sáng phục hồi bố cục theo ảnh/video đã gửi và các token của source c9; Người dùng duyệt hướng light lúc12:43 ngày2026-10-11: “Ổn đó. Tiếp tục theo hướng này đi”. Phê duyệt này áp dụng hướng hình ảnh; native, phone và chức năng vẫn cần các gate riêng. Không dùng số gate chức năng làm phê duyệt thẩm mỹ.

## Nguồn đã kiểm tra

| Nguồn | Identity và phạm vi đã đọc |
| --- | --- |
| `image(20261011-105709).png` | Library `libfile_bfc6299f98688191aefad45568c31377`; 1280×720; SHA256 `22d88110adda78535f1d4bf0be66b9573c0aa0ff9899ccc3f728903650c4ea80`. Đã xem ảnh và crop chữ thật. |
| `neko-bot-kiem-thu-lam-viec-nhom(1).mp4` | Library `libfile_20afb13d1d588191bfbae5895186ba52`; H264, 800×450, 25fps, 38.32s; SHA256 `2ee56258c20a16d03e6341a3cab89d57205945e48e464ae3ae4432dd00e17e5c`. Đã xem 38 samples cùng các frame đầy đủ và 10 samples đoạn 3–4s. Sampling không chứng minh frame pacing hoặc easing. |
| Source c9 | `reference/extracted/app-c9dbbd7/package/app/`, revision `c9dbbd77813458977002025d05ec72acf91e2efe`. Đọc tĩnh Shell, tokens, styles, NekoBrand và kit. Không chạy archive scripts. |
| Grok client 0.68.1 | Drive ID `1QE0mOQiJadtzSLfWH_e4-A7ziBGF74Wy` đã metadata/raw-fetch và materialize bằng Library helper chính thức. ZIP 14,761,571 bytes, SHA256 `d31f6a0342693803510de086fae71a619ae07d20da9ea965e32d061c26f832e5`; CRC PASS và 651 client files khớp bản extracted. Bundle chỉ dùng đọc tĩnh; không sao chép code, logo hoặc asset chưa có quyền. |

Consumer-local files materialize trong WSL giữ metadata của helper. Các inspection copies NTFS đã so hash, không tuyên bố giữ xattrs. Signed transfer URLs không nằm trong tài liệu hoặc deliverable manifests. Byte identity của ZIP được xác minh riêng với nguồn tải; điều đó không chứng minh upstream an toàn.

Mapping chi tiết đã được analyst ghi ở `docs/reviews/old-ui-recovery-map-20261011.md` và `docs/reviews/grok-ux-state-map-20261011.md` trong repository chính. Đặc tả AGENTS vẫn là Tauri UI, Rust trusted host, Go control plane, Neko Core sở hữu agent loop. `.agents/skills` không tồn tại ở checkout được giao; dùng các skill đang được cung cấp và AGENTS hiện có.

## Hình học và hệ thống hình ảnh

| Vùng | Nguồn và triển khai |
| --- | --- |
| Nền và chữ | Pixels ảnh gốc và light tokens `packages/ui-tokens/src/tokens.css`: main `#FAFAF8`, sidebar `#ECECE9`, user `#E2E2DC`, assistant `#F0F0ED`, chữ `#1A1A1A`. |
| Sidebar | Desktop 316px như Shell.tsx:2889; search nhỏ, hàng bot 64px, tên và preview tin gần nhất. Preview, thời gian và trạng thái lấy từ work hiện có. |
| Header | 60px desktop, padding theo Shell.tsx:3603. Mobile 64px cộng safe-area. Giữ tên bot, trạng thái nối lại và nút files/work có accessible name. |
| Transcript | Bubble radius20, pad18×12, chữ15.5px/line1.45–1.5; Shell.tsx:5202,5255,7029,7045. Hiển thị sender và ngày/giờ. Không ẩn trạng thái lỗi, chờ hoặc dừng. |
| Composer | Inline 52px desktop, radius26, action32 và textarea24px, max120px; Shell.tsx:6141,6166,6292,6342. Mobile và coarse pointer nâng action lên44×44; không áp desktop32 vào điện thoại ngang. |
| Motion | Grow composer120ms cubic-bezier(.2,0,0,1) theo Shell.tsx:5888, hủy animation trước lần đo mới; reduced-motion bỏ hiệu ứng. Working glow tự viết và chỉ hiện khi work thật starting/running. Không suy timing từ wake reconciliation hoặc progress component của Grok. |

NekoPeek lockup-light SVG được lấy nguyên byte từ kit c9 và inline vào HTML để hoạt động với whitelist/CSP hiện tại. SVG source, PROVENANCE, AGPL3 và Geist OFL1.1 được giữ cùng assets. Font Latin/Latin Extended/Vietnamese lấy nguyên byte từ archive `04-huong-dan-logo-kiem-thu/NekoBot-01-Huong-dan-Logo-Kiem-thu.zip`, prefix `package/design/nekobot-brand-v1/source/fonts/`. Đây là phần runtime của kit; archive gốc vẫn giữ source và editable/export scripts. Khi phân phối kit đầy đủ cần giữ các thành phần và notices theo PROVENANCE.

Neko/Core và NekoPeek có nguồn AGPL3; Geist là OFL1.1. Rakazo primitives có Apache2 không làm toàn bộ source cũ thành Apache. Không sao chép shared avatar shape có provenance Grok chưa rõ; avatar robot CSS hiện tại được viết mới từ hình tròn, visor và hai mắt, sử dụng màu bot runtime.

## Luồng thật được giữ

Tìm kiếm chỉ lọc roster, không đổi bot đang đọc hoặc xóa bản nháp. Popup `@` gợi ý alias đã có khi nội dung chỉ là prefix ở đầu; Enter/Tab chọn alias, Escape đóng, IME và key.repeat không gửi. Runtime vẫn chỉ route prefix của current request. Menu `+` chọn lane foreground/background bằng FormData hiện có; upload chưa hỗ trợ được nói rõ trong menu.

Giữ nguyên các fence của f6: late ACK không kéo lại bot/draft/focus, outbox reject không chặn tin valid, retry giữ nonce, SSE reconnect và snapshot retry, localStorage-denied startup, safe account form hủy/late response, keyed streaming DOM, mobile modal/inert/focus-return, context capabilities/CAS/stale counters/attempt binding và stop receipt. Controls context vẫn ẩn khi capability hoặc proof không đủ. Không tạo grant hoặc gọi provider trong quá trình này.

Ảnh/video cũ có nhóm, dự án, sender đa bot và tệp trong transcript. Backend hiện tại chưa có các semantics nhóm/dự án/upload/cancel-all đó. Fixture ảnh dùng một bot đang chọn, ba work đã hoàn tất và một work nền ở bot khác; các câu nhắc nhóm/dự án chỉ là dữ liệu minh họa từ ảnh. Header không giả tên nhóm, không thêm nút có backend giả. Right panel hiện là task/files thật; chưa phục hồi computer thumbnail/Take control của app cũ.

## Kiểm chứng và giới hạn

Các report sau khi checkpoint nằm trong thư mục evidence mang tên revision, ghi exact code commit, workingTree, command, environment và SHA256. Commands:

```text
python -X utf8 scripts/test_ui_redesign.py --commit <exact-commit> --context-fixture
node scripts/ui_light_pixels.mjs
python -X utf8 scripts/compare_ui_pixels.py --reference <image-original> --evidence <pixel-evidence-dir>
node --check native/internal/web/assets/app.js
git diff --check
```

Regression runner dùng Go local journal và Rust DPAPI host có sẵn; context cases là owned CDP snapshot/endpoint stub, không phải proof native stop hoặc model evaluation. Pixel runner dùng server/API/SSE fixture riêng, Chrome headless/profile/port loopback riêng. Before là bốn Git objects f6 và after là assets mới; cả hai cùng viewport1280×720 và cùng dữ liệu. Mobile390×844, short320×320 và coarse844×390 được chụp riêng. RGB error chỉ dùng tìm khác biệt, không làm điểm chất lượng; actor/selection/state giữa ảnh người dùng và fixture có khác nhau.

Pixel fixture phục vụ font với CSP đề xuất `font-src 'self'`. Backend baseline của clone chỉ whitelist bốn file và chưa có font-src, nên font Geist trong ảnh fixture chưa chứng minh đã tải trong Go/Tauri thật. Đã báo integrator cần ba exact font paths, Content-Type `font/woff2` và CSP font-src self. Inline logo hoạt động với CSP hiện tại. Không sửa Go/Rust trong patch này.

Chưa có chấp nhận hình ảnh của người dùng, kiểm tra điện thoại thật, bàn phím ảo/IME thật, nonzero safe-area, AT speech, native Tauri của bản sáng, cloud sync hoặc multihost. Native slot do integrator phối hợp; headless không chiếm mouse/window của task runtime.


## Delta sau audit độc lập 2cda67d

Audit immutable 2cda67d ghi regression 28/28 PASS, context 28/28 PASS và features 12 PASS / 7 FAIL / 0 BLOCKED. Bảy gate lỗi thuộc sáu nhóm LIGHT-MA-01..06; chúng là kết quả lịch sử của 2c, không được đổi thành PASS bằng kết quả working tree. Report riêng nằm ở `docs/reviews/mobile-accessibility-light-20261011.md` trong repository chính.

Delta chỉ sửa UI và runner của UI. Drawer có fallback focus vào search khi roster đã lọc rỗng; search cao44px trên mobile/coarse pointer. Popup mention/lane lấy vùng còn lại phía trên composer làm giới hạn chiều cao và cuộn nội dung; active option được cuộn vào scrollport khi dùng mũi tên. Shift+Tab đóng gợi ý và cho trình duyệt quay focus về control trước, không chọn alias. Hint thường vẫn là status trong accessibility tree bằng kiểu sr-only; lỗi giữ hiển thị như trước. Hành vi Enter/Tab xuôi, IME/repeat, draft và runtime contract được giữ.

Runner pixel bổ sung bảy kiểm tra tương ứng, roster14 bot, viewport390x260 và delayed POST503; đây là fixture phòng vệ và AX exposure, chưa chứng minh AT speech hay bàn phím ảo thật. Screenshot popup thấp được giữ cùng exact commit, asset hashes và harness hash. Regression/context vẫn dùng runner61 gate riêng. Chỉ chuyển trạng thái các finding sau khi audit độc lập retest immutable delta.

Kiểm tra font tích hợp riêng đã import nguyên patch được parent duyệt `font-static-9126090-backend.patch` vào clone kiểm chứng, commit `05e87e4373d52180e13bd2b334f5cfda88bb8b27` có UI tree giống 2c. Go1.27.2 windows/amd64 font boundary test PASS và browser actual Go HTTP9 gate PASS: ba font200 đúng bytes/MIME/CSP, Vietnamese local journal, custom Geist81 glyphs. Không API stub trong lượt font đó; không Core/provider/native Tauri. Kết quả này đóng gap actual Go typography cho 2c, không đổi scope fixture của pixel runner hoặc độc lập2c audit. Code backend không thuộc delta UI này.


## Focus gap sau checkpoint12ef2f5

Audit bounded mới của auditor gốc tái hiện GAP-MA-01/02: 11 PASS / 2 FAIL và lượt xác nhận5 PASS / 2 FAIL trên immutable12ef2f5. Kết quả không vô hiệu các75 gate trước; đây là ca bổ sung về breakpoint và control bị disable/ẩn. Report gốc ở `docs/reviews/mobile-accessibility-final-gap-20261011.md`; baseline đã được retester riêng tái hiện bằng nguyên byte harness/cases.

Delta đưa focus ra `nav-open` khi chuyển sang mobile làm sidebar chứa active control trở thành inert. Chỉ control sắp mất khả dụng mới được xử lý; prompt hoặc control hợp lệ khác vẫn giữ focus. RAF dự phòng kiểm tra focusEpoch, navigationGeneration, panelGeneration và trạng thái drawer/panel hiện hành, không kéo lại focus nếu người dùng thao tác trong lúc đổi layout.

Trước khi render disable/ẩn task Cancel đang được focus trong mobile work dialog, UI chuyển focus tới Close. Cùng đường render xử lý pending POST và terminal SSE không có POST. Kiểm tra identity của active control và panel hiện đang mở diễn ra đồng bộ trước DOM mutation; không có restore callback sau ACK nên không kéo focus về request/panel/bot cũ. Chuyển tab, đóng/mở panel, đổi bot và bản nháp được giữ qua late SSE/ACK. Backend contract và preview desktop/mobile không đổi.

Own regression thêm9 gate focus, bao gồm empty/nonempty roster, focus khác còn dùng được, pending cancel, terminal ACK/SSE, unsolicited terminal, người dùng đổi control, mở lại files panel và đổi bot/draft. Các cancellation cases mới là CDP stub, không proof native Core cancellation. Evidence exact commit, commands và scope được freeze riêng; visual/mobile physical/AT/native/cloud acceptance vẫn pending.


Từ phê duyệt12:43, tiếp tục hướng light đã giao; không đổi palette/layout hoặc thay preview khi sửa focus. Evidence trước thời điểm này giữ nguyên trạng thái lịch sử pending. Handoff mới ghi `visualDirection=approved`, không đổi các scope native/phone/AT/backend/cloud chưa được nghiệm thu.


Trace trong own regression xác nhận CSS `visibility:hidden` phát `focusout` trước callback media-query. UI chỉ giữ một marker cho focus mất vì layout, kèm focus/navigation/panel generation; marker được tiêu thụ ở breakpoint handler. Blur khi control vẫn visible không tạo marker. Ca explicit blur và prompt focus bảo đảm không lấy lại focus người dùng đã rời đi.
