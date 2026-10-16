# Mốc source Neko Bot

[← Tài liệu](README.md)

## v0.1.0-alpha.1

Annotated tag [`v0.1.0-alpha.1`](https://github.com/meiiie/neko-bot/tree/v0.1.0-alpha.1) đánh dấu snapshot source phát triển đầu tiên được trình bày tại `meiiie/neko-bot`. Đây là bản **alpha**, chưa phải stable hoặc production. Không có GitHub Release, binary, installer, chữ ký phát hành hay deploy đi kèm mốc này.

Version của Neko Bot desktop/Tauri và Rust host trong source đều là `0.1.0`. Neko Core giữ version riêng `1.8.0`; không dùng version Core làm số bản phát hành của Neko Bot. Tag không thay đổi version hoặc hành vi runtime.

Mốc này gồm source app, Core liên quan và 61 file overlay Wiii theo [provenance](../migration/source-manifest.json), cùng README, tài liệu setup và quy tắc đóng góp. Banner mang nhận diện Neko Peek; ảnh ứng dụng là [fixture lịch sử `2a74029`](assets/README.md), không phải bằng chứng nghiệm thu toàn bộ snapshot hiện tại.

Các gate còn mở gồm saved-account witness/Core bootstrap, broker/OAuth và provider thật, cloud/đa thiết bị, điện thoại thật/AT speech, đóng gói/ký và nghiệm thu dành cho người dùng. Wiii cần base đã xác minh để ghép overlay; thư mục `wiii/` không phải repository đầy đủ. Xem [trạng thái](project-status.md), [roadmap](ROADMAP.md) và [setup](getting-started.md) trước khi chạy hoặc đóng góp.

Không có giấy phép chung mới cho mọi component; giữ nguyên [license/attribution](../README.md#attribution-và-giấy-phép). Các nhánh WIP/archive được bảo toàn và không trở thành stable nhờ tag này.
