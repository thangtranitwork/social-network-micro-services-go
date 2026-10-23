# Kế hoạch phát triển Ứng dụng Di động (Mobile App) - Social Network

Tài liệu này vạch ra toàn bộ lộ trình, kiến trúc kỹ thuật, phân rã tính năng (task breakdown) và giải pháp tích hợp cho ứng dụng di động của hệ thống mạng xã hội `social-network-go`.

---

## 1. Mục tiêu & Định hướng Kỹ thuật

### 1.1 Mục tiêu
- Xây dựng ứng dụng di động chạy mượt mà trên cả hai nền tảng **iOS** và **Android**.
- Tận dụng 100% hệ thống Microservices hiện có thông qua **API Gateway** (`:11111`).
- Đảm bảo trải nghiệm thời gian thực (Real-time): Tin nhắn Chat, Thông báo đẩy (FCM), Cuộc gọi thoại & video (WebRTC P2P).
- Tối ưu hóa hiệu năng di động: Giảm tiêu thụ pin, nén ảnh/video phía client, lưu cache offline, cuộn mượt 60/120 FPS.

### 1.2 So sánh & Đề xuất Công nghệ (Tech Stack)

| Tiêu chí | **Phương án A: Flutter (Dart)** *(Khuyên dùng)* | **Phương án B: React Native (Expo / Bare)** |
| :--- | :--- | :--- |
| **Hiệu năng UI** | Cực kỳ mượt mà, render trực tiếp qua Skia/Impeller, không phụ thuộc Native Bridge. | Rất tốt với New Architecture (Fabric/TurboModules), nhưng list ảnh nặng cần tối ưu kỹ. |
| **WebRTC & Calling** | Plugin `flutter_webrtc` cực kỳ ổn định, quản lý vòng đời camera/mic tốt. | Cần cấu hình `react-native-webrtc` + cấu hình native CallKit / ConnectionService phức tạp hơn. |
| **Tận dụng code cũ** | Viết mới code Dart, chỉ tái sử dụng API contract/DTO logic. | Tái sử dụng được tư duy React, hooks, và logic từ `social-network-ui` (Next.js). |
| **Push Notification** | `firebase_messaging` + `flutter_local_notifications` chuẩn hóa cao. | `@react-native-firebase/messaging` + `notifee`. |
| **Đánh giá chung** | **Lựa chọn tối ưu cho Social App** có media nặng, stories, call và animation phức tạp. | **Lựa chọn tối ưu về tốc độ triển khai** nếu đội ngũ quen thuộc với React/TypeScript. |

---

## 2. Kiến trúc Hệ thống Mobile (Mobile Architecture)

### 2.1 Sơ đồ tương tác với Backend Microservices

```
┌────────────────────────────────────────────────────────┐
│                   Mobile Client                        │
│   (Presentation Layer: Bloc/Riverpod/Zustand + UI)     │
└──────────────┬───────────────────┬─────────────────────┘
               │ HTTP/REST         │ WebSocket (Chat/Call)
               ▼                   ▼
┌────────────────────────────────────────────────────────┐
│            API Gateway (Go - Port 11111)               │
│  - JWT Auth Middleware   - Rate Limit    - Reverse Proxy│
└───────┬───────────┬──────────────┬─────────────┬───────┘
        │           │              │             │
        ▼           ▼              ▼             ▼
   Auth Service  User Service   Post Service  Chat & Call Svc
   (:11112)      (:11113)       (:11114)      (:11115)
        │                                        │
        ▼                                        ▼
   File Service (MinIO)                   WebRTC P2P Media
   (:11118)                               (Audio/Video Mesh)
```

### 2.2 Các Module Cốt lõi Phía Client (Core Modules)

1. **Network & Auth Layer**:
   - HTTP Client (Dio / Axios) cấu hình BaseURL động (Dev / Staging / Production).
   - **JWT Interceptor**:
     - Tự động gắn header `Authorization: Bearer <access_token>`.
     - Cơ chế **Silent Refresh Token Queue**: Khi nhận mã `401 Unauthorized`, tạm dừng các request khác, gọi `POST /v1/auth/refresh` để nhận Access Token mới rồi tự động retry các request đang chờ mà không làm văng người dùng ra ngoài.
2. **Realtime WebSocket Manager**:
   - Quản lý vòng đời kết nối: Tự động kết nối lại (Exponential Backoff), Heartbeat/Ping-pong để tránh rớt socket khi chuyển mạng (Wifi <-> 4G) hoặc chuyển trạng thái Background/Foreground.
   - Định tuyến sự kiện: Tin nhắn mới, trạng thái đang gõ (typing), trạng thái online/offline, tín hiệu cuộc gọi (`CALL_OFFER`, `CALL_ANSWER`, `CALL_CANDIDATE`, `CALL_HANGUP`).
3. **WebRTC Calling Engine & Native Call UI**:
   - Tích hợp `CallKit` (iOS) và `Android ConnectionService / Fullscreen Intent` để khi có cuộc gọi đến, điện thoại rung chuông toàn màn hình ngay cả khi máy đang khóa màn hình.
   - Điều khiển Audio Route: Chuyển đổi giữa Loa ngoài (Speaker), Loa trong (Earpiece), và Tai nghe Bluetooth.
4. **Push Notification (FCM Service)**:
   - Đăng ký thiết bị và đồng bộ Device Token lên `fcm-service` khi đăng nhập.
   - Xử lý 3 trạng thái thông báo: Foreground (hiển thị in-app banner), Background (hiển thị system tray), Terminated (mở đúng màn hình chi tiết khi click thông báo - Deep Linking).
5. **Media Pipeline**:
   - Chọn ảnh/video từ thư viện máy hoặc chụp trực tiếp.
   - **Nén ảnh trước khi tải lên (Client-side Compression)**: Nén ảnh JPEG/WebP xuống dung lượng < 500KB để tiết kiệm băng thông 4G và giảm tải lưu trữ MinIO.
   - Tải file lên `file-service` theo cơ chế Multipart hoặc Presigned URL.
6. **Local Caching & Offline Support**:
   - Lưu trữ an toàn (Secure Storage / Keychain / Keystore): Lưu Refresh Token, thông tin nhạy cảm.
   - Key-Value / Local Database (MMKV / SQLite): Cache thông tin cá nhân của User, danh sách bài viết gần nhất, bản nháp tin nhắn (drafts).

---

## 3. Lộ trình Triển khai Chi tiết (Phases & Milestones)

### Giai đoạn 1: Khởi tạo Kiến trúc Nền tảng (Foundation & Core)
- **Task 1.1**: Khởi tạo project (Flutter hoặc React Native/Expo), cấu hình Bundle ID, App Icon, Splash Screen, Safe Area, Multi-environment (`.env.dev`, `.env.prod`).
- **Task 1.2**: Thiết lập Design System: Bảng màu (Hỗ trợ Dark Mode / Light Mode), Typography, nút bấm, input, avatar components chuẩn hóa.
- **Task 1.3**: Xây dựng Network Client & Interceptor (Dio/Axios): BaseURL, Logging, Timeout, Token Refresh Handler.
- **Task 1.4**: Triển khai Lưu trữ an toàn (Secure Storage) & State Management cho Session người dùng.

### Giai đoạn 2: Xác thực & Quản lý Tài khoản (Auth & Onboarding)
- **Task 2.1**: Màn hình Đăng nhập (Username/Email & Password), xử lý lưu phiên đăng nhập.
- **Task 2.2**: Màn hình Đăng ký đa bước (Multi-step Register): Nhập thông tin -> Xác thực mã OTP qua Email.
- **Task 2.3**: Quên mật khẩu & Đặt lại mật khẩu (`/v1/auth/forgot-password`, `/v1/auth/reset-password`).
- **Task 2.4**: Tích hợp Đăng nhập bằng Google (Google Sign-In OAuth).
- **Task 2.5**: Xác thực sinh trắc học (Vân tay / FaceID) để mở khóa nhanh ứng dụng.

### Giai đoạn 3: Bảng tin (Newsfeed) & Tương tác Bài viết
- **Task 3.1**: Cấu trúc Tab điều hướng chính (Bottom Navigation Bar): Bảng tin (Home), Tìm kiếm (Explore), Tạo bài viết (+), Tin nhắn (Chat), Cá nhân (Profile).
- **Task 3.2**: Bảng tin chính (Feed):
  - Kéo để làm mới (Pull-to-refresh).
  - Phân trang cuộn vô tận (Infinite Scroll Pagination với cursor/offset).
  - Card bài viết: Render avatar, tên, thời gian tương đối, nội dung văn bản, Carousel ảnh dạng lưới, Trình phát video tự động pause/play khi cuộn vào khung nhìn.
- **Task 3.3**: Tương tác bài viết: Thả cảm xúc (Like/Love/Haha,...), Chia sẻ bài viết, Lưu bài viết.
- **Task 3.4**: Màn hình Tạo bài viết:
  - Chọn nhiều ảnh/video, chụp ảnh trực tiếp.
  - Tự động nén ảnh client-side trước khi upload lên `file-service`.
  - Chọn chế độ hiển thị (Public, Friends, Private).
- **Task 3.5**: Chi tiết bài viết & Bình luận:
  - Hiển thị danh sách bình luận dạng cây (hỗ trợ reply lồng nhau).
  - Khung nhập bình luận kèm emoji và chọn ảnh đính kèm.
- **Task 3.6**: Tin 24h (Stories):
  - Thanh tròn danh sách Stories ở đầu Feed.
  - Màn hình Story Viewer: Thanh tiến trình tự chạy (5s/story), chạm trái/phải để chuyển story, giữ màn hình để pause.
  - Đăng Story mới (ảnh/video).

### Giai đoạn 4: Mạng xã hội & Trang cá nhân (Social Graph & Profile)
- **Task 4.1**: Màn hình Trang cá nhân của tôi (My Profile) & Người khác (User Profile):
  - Ảnh bìa, ảnh đại diện, tiểu sử, số lượng bạn bè.
  - Tab lưới bài viết, tab ảnh/video đã đăng.
- **Task 4.2**: Quản lý Bạn bè:
  - Danh sách bạn bè hiện tại.
  - Gợi ý kết bạn (`/v1/friends/suggested` từ Recommendation Service).
  - Danh sách lời mời kết bạn (Chấp nhận / Từ chối).
  - Chức năng Chặn (Block) người dùng.
- **Task 4.3**: Chỉnh sửa hồ sơ cá nhân: Đổi avatar, đổi cover (tích hợp crop ảnh), sửa tên hiển thị, bio.

### Giai đoạn 5: Nhắn tin Thời gian thực (Real-time Chat)
- **Task 5.1**: Màn hình Danh sách Hội thoại (Chat List):
  - Hiển thị danh sách Direct Chat & Group Chat.
  - Tin nhắn mới nhất, thời gian, số lượng tin nhắn chưa đọc (Unread badge).
  - Trạng thái Online / Offline của đối phương.
- **Task 5.2**: Màn hình Khung chat (Chat Room):
  - Cuộn mượt với danh sách tin nhắn đảo ngược (Inverted List).
  - Bubble tin nhắn phân biệt bên gửi / bên nhận.
  - Trạng thái tin nhắn: Đang gửi (Clock), Đã gửi (1 tick), Đã nhận (2 ticks xám), Đã xem (2 ticks xanh).
  - Gửi tin nhắn văn bản, icon emoji.
  - Gửi hình ảnh, video ngắn, ghi âm tin nhắn thoại (Voice note).
  - Hiển thị thông báo "Đang soạn tin nhắn..." (Typing indicator).
- **Task 5.3**: Quản lý Chat nhóm:
  - Tạo nhóm chat mới, đặt tên nhóm, ảnh nhóm.
  - Thêm thành viên, xóa thành viên, rời nhóm.

### Giai đoạn 6: Cuộc gọi Thoại & Video (WebRTC Calling)
- **Task 6.1**: Tích hợp WebRTC Engine & Lấy ICE Servers cấu hình (`/v1/call/ice-servers`).
- **Task 6.2**: Luồng Gọi đi (Outgoing Call):
  - Phát âm thanh chuông chờ (Ringtone).
  - Gửi tín hiệu `CALL_OFFER` qua WebSocket và chờ phản hồi.
- **Task 6.3**: Luồng Cuộc gọi đến (Incoming Call):
  - Nhận sự kiện `CALL_OFFER` qua WebSocket (khi app đang mở) hoặc qua FCM High-Priority Data Message (khi app đang tắt/background).
  - Đổ chuông toàn màn hình (Tích hợp CallKit cho iOS, Fullscreen Intent cho Android).
  - Nút Trả lời (Accept) và Từ chối (Decline).
- **Task 6.4**: Màn hình Trong cuộc gọi (In-Call Screen):
  - Hiển thị luồng video đối phương (Remote stream) và camera bản thân (Local PIP stream).
  - Bật/tắt Micro, Bật/tắt Camera, Đổi camera trước/sau, Đổi loa trong/loa ngoài.
  - Đếm thời gian cuộc gọi.
  - Xử lý ngắt cuộc gọi (`CALL_HANGUP`) và dọn dẹp WebRTC PeerConnection an toàn.

### Giai đoạn 7: Tìm kiếm, Thông báo đẩy & Cài đặt (Search & FCM)
- **Task 7.1**: Tìm kiếm tổng hợp:
  - Tìm kiếm người dùng theo username/tên.
  - Tìm kiếm bài viết theo từ khóa (gọi qua Search Service / Elasticsearch).
- **Task 7.2**: Trung tâm thông báo (Notification Screen):
  - Danh sách thông báo: Có ai tương tác bài viết, bình luận, gửi lời mời kết bạn.
  - Đánh dấu đã đọc tất cả, click vào thông báo điều hướng tới bài viết/trang cá nhân tương ứng.
- **Task 7.3**: Đăng ký FCM Push Notification:
  - Đồng bộ FCM Token lên `fcm-service`.
  - Nhận thông báo nền ngay cả khi tắt ứng dụng.
- **Task 7.4**: Cài đặt ứng dụng:
  - Đổi mật khẩu, kích hoạt bảo mật 2 lớp (2FA).
  - Tùy chỉnh thông báo, cài đặt ngôn ngữ (Tiếng Việt / English).
  - Đăng xuất an toàn.

### Giai đoạn 8: Tối ưu, Kiểm thử & Đóng gói Phát hành
- **Task 8.1**: Kiểm thử tích hợp & Tối ưu hiệu năng:
  - Virtualization cho Feed list để tránh tràn RAM khi lướt bài dài.
  - Image Cache (Disk & Memory) để không tải lại ảnh đã xem.
- **Task 8.2**: Tích hợp Crash Reporting (Firebase Crashlytics hoặc Sentry) để theo dõi lỗi trên thiết bị thực tế.
- **Task 8.3**: Xây dựng kịch bản CI/CD (Fastlane / GitHub Actions) tự động build:
  - Android: `APK` và `AAB` (Android App Bundle).
  - iOS: `IPA` cho TestFlight / App Store.

---

## 4. Kế hoạch Phân bổ Nhân sự & Thời gian (Ước lượng)

| Sprint / Tuần | Trọng tâm | Deliverable |
| :---: | :--- | :--- |
| **Tuần 1** | Khởi tạo Project, Network, Auth & Security | Đăng nhập, Đăng ký, Quản lý Token hoạt động ổn định. |
| **Tuần 2** | UI Framework, Feed, Post & Stories | Lướt Feed, xem ảnh/video, tạo bài viết mới kèm ảnh. |
| **Tuần 3** | Profile, Friends & Search | Trang cá nhân, kết bạn, tìm kiếm người dùng. |
| **Tuần 4** | Realtime Chat (1-1 & Group) | Nhắn tin text, ảnh, voice note, typing realtime qua WebSocket. |
| **Tuần 5** | WebRTC Calling (Audio & Video) | Gọi điện 1-1, chuông reo màn hình khóa, đàm thoại video. |
| **Tuần 6** | FCM Push Notifications, Cài đặt & Polish | Nhận thông báo nền, thông báo cuộc gọi, đa ngôn ngữ. |
| **Tuần 7** | Testing, Tối ưu RAM/Pin, Build Release | Bản Beta TestFlight & APK/AAB hoàn chỉnh. |

---

## 5. Rủi ro Kỹ thuật & Biện pháp Phòng ngừa

1. **Cuộc gọi nền khi ứng dụng bị tắt / khóa màn hình**:
   - *Rủi ro*: Hệ điều hành iOS và Android (đặc biệt từ Android 12+) hạn chế tiến trình chạy ngầm, cuộc gọi WebRTC có thể không kích hoạt được nếu chỉ dùng WebSocket.
   - *Giải pháp*: Sử dụng **FCM High-Priority Data Push** kết hợp **VoIP Push / CallKit** (iOS) và **Fullscreen Intent** (Android) để đánh thức ứng dụng và hiển thị màn hình chuông reo trước khi thiết lập kết nối WebRTC.
2. **Tiêu tốn dung lượng & Pin khi lướt mạng xã hội**:
   - *Rủi ro*: Người dùng lướt hàng trăm bài viết chứa ảnh độ phân giải cao và video dẫn đến nóng máy, lag và tốn dung lượng di động.
   - *Giải pháp*: Luôn dùng thư viện Caching thông minh (`cached_network_image` / `FastImage`), chỉ tự động phát video khi item nằm ở trung tâm màn hình, nén ảnh phía client trước khi upload.
3. **Đứt kết nối WebSocket khi đổi mạng (Wifi sang 4G)**:
   - *Rủi ro*: Socket bị rơi vào trạng thái "half-open" (mất kết nối nhưng client chưa nhận biết).
   - *Giải pháp*: Triển khai chu kỳ Heartbeat (Ping/Pong) mỗi 25-30 giây, kết hợp lắng nghe sự kiện `ConnectivityChanged` của thiết bị để chủ động tái kết nối tức thì.
