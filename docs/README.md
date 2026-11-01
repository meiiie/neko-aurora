# Tài liệu Neko Aurora

[← README](../README.md)

## Bắt đầu

- [Setup/build Windows, Tauri, Core và xử lý lỗi thường gặp](getting-started.md).
- [Chọn AI: GLM, ChatGPT và danh sách model theo kết nối](ai-setup.md).
- [Nghiên cứu luồng model và hội thoại OpenMausBot, 2026-11-01](research/provider-flow-openmaus-2026-11-01.md).
- [Workspace mới: điều hướng, tìm nhanh và ngữ cảnh bot](workspace.md).
- [Nghiên cứu giao diện và chức năng OpenMausBot](research/openmaus-interface-2026-10-30.md).
- [Thư viện trang: bản nháp, phiên bản và phục hồi lần lưu](pages.md).
- [Pages: kiểm thử local và các phần còn thiếu](research/grokbot-pages-2026-10-29.md).
- [Lịch theo khoảng thời gian: cách dùng và giới hạn](routines.md).
- [Kiến trúc và ranh giới trách nhiệm](architecture.md).
- [Trạng thái nền tảng và phạm vi kiểm chứng](project-status.md).
- [Mốc source alpha và version](releases.md).
- [Roadmap và acceptance gates](ROADMAP.md).
- [Đóng góp](../CONTRIBUTING.md), [hỗ trợ](../SUPPORT.md), [bảo mật](../SECURITY.md).
- [Nguồn banner/logo, badges và ảnh fixture theo phiên bản](assets/README.md).

## Source và bằng chứng lịch sử

`app/docs/` chứa contract, delivery và audit theo từng commit. Đường dẫn/log/binary được nhắc trong tài liệu lịch sử có thể không đi kèm checkout; đó không phải hướng dẫn cài đặt có sẵn. Bằng chứng local/fixture giữ đúng scope, không tự chuyển thành release hiện hành.

- [Product gap matrix](../app/docs/product-gap-matrix-20261011.md).
- [Desktop instance contract](../app/docs/desktop-local-instance-20261012.md).
- [Core instructions](../core/AGENTS.md) và [Core testing](../core/docs/process/TESTING.md).
- [Giao tiếp agent: cách dùng, hợp đồng và giới hạn](../app/docs/agent-network-20261025.md).
- [Máy tính cloud: worker riêng, Vault và phạm vi kiểm chứng](../app/docs/cloud-browser-20261025.md).
- [Nghiên cứu Grok Bot: phần đã áp dụng, giới hạn và backlog](grokbot-adoption-2026-10-28.md).
- [Wiii overlay/base manifest](../migration/wiii-integration.json).
- [Source provenance manifest](../migration/source-manifest.json).

Không có một giấy phép chung mới cho toàn bộ snapshot. Xem [attribution/license map](../README.md#attribution-và-giấy-phép) và từng component trước khi tái sử dụng source.
