# Neko Aurora application

Ứng dụng local đa bot/đa kênh: `native/` là Go control plane, `host/` là Rust trusted host, `desktop/` là Tauri shell. Neko Core sở hữu agent loop; Wiii là ADE tùy chọn riêng.

Xem [README ở root](../README.md) để build/chạy từ source, mở Tauri bằng receipt hiện hành, kiểm tra code và hiểu giới hạn/WIP. Bản source này không kèm prebuilt manifest hoặc installer; các receipt và đường dẫn máy trong tài liệu lịch sử không phải điều kiện setup có sẵn.

Source có chức năng local đã được kiểm tra trong các môi trường được ghi tại [ma trận khoảng trống](docs/product-gap-matrix-20261011.md). Không suy từ fixture local sang chất lượng model, cloud sync hay điện thoại thật. Đọc [working rules](AGENTS.md) trước khi sửa code.
