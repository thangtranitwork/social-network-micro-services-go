# Social Network Mobile Application (Flutter)

Ứng dụng di động mạng xã hội đa nền tảng (iOS & Android) kết nối trực tiếp với hệ sinh thái Microservices của `social-network-go`.

---

## 🚀 Tính năng Cốt lõi
- **Bảng tin & Tương tác**: Lướt Newsfeed mượt mà, xem video tự động, thả reaction đa dạng, bình luận lồng nhau.
- **Tin 24h (Stories)**: Trình chiếu thanh tiến trình 5s tự động, vuốt chuyển story, nhấn giữ tạm dừng.
- **Trang cá nhân & Mạng lưới bạn bè**: Hồ sơ cá nhân, gợi ý bạn bè thông minh, quản lý bạn bè và danh sách chặn.
- **Chat Real-time**: Nhắn tin 1-1 và Group Chat, trạng thái online, gửi ảnh/video/voice note, chỉ báo soạn tin.
- **Cuộc gọi WebRTC**: Cuộc gọi thoại & video P2P chất lượng cao với màn hình chuông reo toàn màn hình.
- **Thông báo đẩy (FCM)**: Đánh thức ứng dụng khi có tin nhắn hoặc cuộc gọi đến qua Firebase Cloud Messaging.

---

## 📁 Cấu trúc Thư mục Chuẩn (Clean Architecture)

```text
mobile/
├── .github/workflows/          # CI/CD tự động build APK/AAB
├── android/                    # Native Android project configuration
├── ios/                        # Native iOS project configuration
├── assets/                     # Icons, hình ảnh và phông chữ
│   ├── icons/
│   └── images/
├── lib/
│   ├── main.dart               # Entrypoint khởi tạo app, bloc, router
│   ├── core/                   # Module dùng chung toàn bộ ứng dụng
│   │   ├── constants/          # API endpoints, Theme colors, Assets paths
│   │   ├── network/            # Dio HTTP Client, Auth Interceptor, Retry Queue
│   │   ├── storage/            # Secure Storage (Keychain/Keystore) lưu Token
│   │   ├── websocket/          # WebSocket Service (Realtime Chat & Signaling)
│   │   ├── webrtc/             # WebRTC PeerConnection & Call Manager
│   │   └── utils/              # Formatters, Date helpers, Image compression
│   ├── features/               # Các chức năng nghiệp vụ độc lập
│   │   ├── auth/               # Login, Register, OTP verification, Forgot pass
│   │   ├── feed/               # Newsfeed list, Post card, Reactions
│   │   ├── post/               # Tạo bài viết, nén ảnh, upload MinIO
│   │   ├── story/              # Trình chiếu story, tạo story 24h
│   │   ├── chat/               # Chat list, chat room, voice notes
│   │   ├── call/               # Màn hình chuông reo, đàm thoại video WebRTC
│   │   ├── profile/            # Trang cá nhân, danh sách bạn bè
│   │   ├── search/             # Tìm kiếm người dùng, bài viết
│   │   └── notification/       # Danh sách thông báo, cấu hình FCM
│   └── shared/                 # Widgets dùng chung (Buttons, Inputs, Dialogs)
├── CHECKLIST.md                # Bảng theo dõi tiến độ chi tiết từng task
├── pubspec.yaml                # Khai báo dependencies
└── analysis_options.yaml       # Quy chuẩn linter Dart
```

---

## ⚙️ Cấu hình Môi trường Kết nối API Gateway

Mặc định ứng dụng kết nối tới **API Gateway** của hệ thống:
- **Local Dev (Android Emulator)**: `http://10.0.2.2:11111`
- **Local Dev (iOS Simulator / Thiết bị thật cùng mạng)**: `http://<IP-may-tinh>:11111`
- **Staging / Production**: `https://api.<ten-mien-cua-ngai>.com`

---

## 🛠 Lệnh Phát triển Nhanh

```bash
# 1. Cài đặt các thư viện
flutter pub get

# 2. Chạy ứng dụng trên thiết bị ảo / thật
flutter run

# 3. Phân tích mã nguồn
flutter analyze

# 4. Chạy Unit test
flutter test

# 5. Build file APK release cục bộ
flutter build apk --release --split-per-abi
```

---

## 🤖 Tự động hóa CI/CD với GitHub Actions
Dự án đã được tích hợp workflow tại `.github/workflows/mobile-android.yml`.
Mỗi khi Ngài push code lên nhánh `main` hoặc tạo Git tag `mobile-v*`:
- GitHub Actions trên cloud sẽ tự động chạy lint, kiểm thử và biên dịch ra file **`.apk`** và **`.aab`**.
- File cài đặt sẵn sàng tải về trong mục **Actions -> Artifacts** hoặc **Releases** trên GitHub mà không tốn 1MB RAM nào của máy Ngài!
