-- Chạy một lần khi container Postgres khởi tạo volume mới.
-- Database riêng cho test tích hợp: test được phép xóa sạch dữ liệu ở đây, không bao giờ đụng DB dev.
CREATE DATABASE livestream_test;
