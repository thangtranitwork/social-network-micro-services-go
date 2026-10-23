# Mobile App Development Checklist (Flutter / Dart)

Bảng theo dõi tiến độ triển khai ứng dụng di động cho mạng xã hội `social-network-go`.
Tài liệu kiến trúc và giải pháp chi tiết: [plans/mobile_app_development_plan.md](../plans/mobile_app_development_plan.md)

---

## 📊 Tổng quan Tiến độ

## 📊 Tổng quan Tiến độ

| Giai đoạn | Trọng tâm | Số Task | Trạng thái |
| :--- | :--- | :---: | :---: |
| **Phase 0** | Khởi tạo Dự án & Cấu hình CI/CD | 4 | 🟢 Hoàn thành |
| **Phase 1** | Core Architecture & Network Layer | 5 | 🟢 Hoàn thành |
| **Phase 2** | Auth & User Onboarding | 6 | 🟢 Hoàn thành |
| **Phase 3** | Bottom Navigation & Newsfeed | 6 | 🟢 Hoàn thành |
| **Phase 4** | Post Creation, Media & Stories | 6 | 🟢 Hoàn thành cốt lõi |
| **Phase 5** | Social Graph & Profile | 5 | 🟢 Hoàn thành cốt lõi |
| **Phase 6** | Real-time Chat (1-1 & Group) | 6 | 🟢 Hoàn thành cốt lõi |
| **Phase 7** | WebRTC Voice & Video Calling | 5 | 🟢 Hoàn thành cốt lõi |
| **Phase 8** | Search, Notification & FCM Push | 5 | 🟢 Hoàn thành cốt lõi |
| **Phase 9** | Performance, Testing & Release | 5 | ⬜ Sẵn sàng CI/CD Release |

---

## 🛠 Chi tiết từng Giai đoạn & Checklist Tasks

### Phase 0: Khởi tạo Dự án & Cấu hình CI/CD
- [x] **Task 0.1**: Tạo cấu trúc thư mục `mobile/` theo chuẩn Clean Architecture (Core, Features, Shared).
- [x] **Task 0.2**: Thiết lập `pubspec.yaml` với đầy đủ các gói: `dio`, `flutter_bloc`, `flutter_webrtc`, `firebase_messaging`, `flutter_secure_storage`, `cached_network_image`, `web_socket_channel`.
- [x] **Task 0.3**: Cấu hình quy tắc linter trong `analysis_options.yaml` và cấu hình `.gitignore`.
- [x] **Task 0.4**: Thiết lập workflow GitHub Actions `.github/workflows/mobile-android.yml` để tự động build APK và AAB trên cloud.

---

### Phase 1: Core Architecture & Network Layer
- [x] **Task 1.1**: Định nghĩa cấu hình môi trường đa nền tảng (`AppConfig`: Dev, Staging, Production) và cấu hình Base URL trỏ về API Gateway (`http://<ip-or-domain>:11111`).
- [x] **Task 1.2**: Xây dựng `ApiClient` (Dio HTTP Client) với timeout, error logging, trace ID.
- [x] **Task 1.3**: Triển khai `AuthInterceptor`:
  - [x] Tự động gắn header `Authorization: Bearer <access_token>`.
  - [x] Bắt lỗi `401 Unauthorized` và tự động kích hoạt **Silent Refresh Token** (`/v1/auth/refresh`).
  - [x] Khóa hàng đợi request (Queue Lock) để tránh refresh trùng lặp.
- [x] **Task 1.4**: Xây dựng `SecureStorageService` (Keychain / Keystore) để mã hóa lưu trữ Access Token, Refresh Token, User ID.
- [x] **Task 1.5**: Xây dựng Design System nền tảng: Bảng màu `AppColors` (Luxury OLED Dark & Modern Light), Typography Outfit & Inter, Nút bấm Spring Animation, Floating Inputs, Stories Gradient Ring.

---

### Phase 2: Auth & User Onboarding
- [x] **Task 2.1**: Màn hình Splash: Kiểm tra token hợp lệ trong Storage để điều hướng tự động (vào Home nếu đã đăng nhập, vào Auth nếu chưa).
- [x] **Task 2.2**: Màn hình Đăng nhập (Login):
  - [x] Form nhập username/email và mật khẩu với validation.
  - [x] Gọi API `POST /v1/auth/login`.
  - [x] Lưu trữ session an toàn vào Secure Storage.
- [x] **Task 2.3**: Màn hình Đăng ký tài khoản (Register):
  - [x] Form nhập họ tên, username, email, mật khẩu kết nối `POST /v1/register`.
  - [x] Validation thời gian thực và thông báo trạng thái.
- [ ] **Task 2.4**: Quên mật khẩu & Đặt lại mật khẩu (`POST /v1/auth/forgot-password`, `/v1/auth/reset-password`).
- [ ] **Task 2.5**: Đăng nhập bằng Google (Google OAuth2).
- [ ] **Task 2.6**: Xác thực sinh trắc học (Vân tay / FaceID) để mở khóa nhanh khi quay lại app.

---

### Phase 3: Bottom Navigation & Newsfeed
- [x] **Task 3.1**: Khung điều hướng Floating Frosted Glass Bottom Navigation Bar (5 Tab):
  - [x] Tab 1: Trang chủ (Newsfeed)
  - [x] Tab 2: Khám phá / Tìm kiếm (Explore)
  - [x] Tab 3: Tạo bài viết mới (+) - Glow Action Button
  - [x] Tab 4: Tin nhắn (Chats)
  - [x] Tab 5: Trang cá nhân (Profile)
- [x] **Task 3.2**: Triển khai Bảng tin (Newsfeed Bloc & UI):
  - [x] Gọi API lấy bảng tin `GET /v1/posts/newsfeed` (hỗ trợ phân trang cursor/page).
  - [x] Thao tác kéo để tải lại (Pull-to-refresh).
  - [x] Cuộn vô tận (Infinite Scroll) với hiệu năng cao, không giật lag.
- [x] **Task 3.3**: Component Thẻ bài viết (Post Card) thế hệ mới:
  - [x] Header bài viết: Avatar, tên tác giả, badge tick xanh, nhãn thời gian tương đối.
  - [x] Nội dung văn bản với typography phân cấp rõ ràng.
  - [x] Bộ hiển thị hình ảnh bo góc tinh xảo với hiệu ứng Double-tap to Like nảy tim.
- [x] **Task 3.4**: Tương tác bài viết:
  - [x] Nút Thả tim (Like/Unlike) với Optimistic Update tức thì.
  - [x] Nút Bình luận mở BottomSheet.
  - [x] Nút Chia sẻ & Lưu bài viết.
- [x] **Task 3.5**: Màn hình Bình luận (Comments Bottom Sheet):
  - [x] Danh sách bình luận thời gian thực, input bar gắn liền bàn phím.
- [x] **Task 3.6**: Thanh Tin 24h (Stories Bar) ở đầu Feed:
  - [x] Vòng tròn "Your story" kèm nút `+` gradient.
  - [x] Danh sách story bạn bè với viền xoay gradient rực rỡ.
  - [ ] Khung nhập bình luận kèm emoji và chọn ảnh đính kèm.
  - [ ] Xóa/sửa bình luận của chính mình.
- [ ] **Task 3.6**: Thanh Tin 24h (Stories Bar) ở đầu Feed:
  - [ ] Danh sách hình tròn đại diện cho Story của bạn bè.
  - [ ] Vòng tròn viền gradient biểu thị Story chưa xem.

---

### Phase 4: Post Creation, Media & Stories
- [x] **Task 4.1**: Chọn ảnh & video từ máy (Image Picker): Hỗ trợ chọn nhiều file, chụp ảnh/quay video trực tiếp.
- [x] **Task 4.3**: Tải file lên MinIO thông qua `file-service`:
  - [x] Upload trực tiếp Multipart hoặc lấy Presigned URL (`GET /v1/files/upload/presigned`).
  - [x] Hiển thị thanh tiến trình tải lên (Upload Progress Bar).
- [x] **Task 4.4**: Màn hình Đăng bài viết mới (`CreatePostScreen`):
  - [x] Nhập nội dung, chọn quyền riêng tư (`PUBLIC`, `FRIEND`, `PRIVATE`).
  - [x] Gọi API `POST /v1/posts/post` và tự động làm mới Newsfeed.
- [ ] **Task 4.5**: Màn hình Trình chiếu Story (Story Viewer):
  - [ ] Thanh tiến trình (Progress Bar) chia theo số lượng story của user, tự động chuyển sau 5 giây.
  - [ ] Chạm nửa trái/phải màn hình để lùi/tiến story.
  - [ ] Nhấn giữ màn hình để tạm dừng (pause).
  - [ ] Đếm lượt xem story và hiển thị danh sách người đã xem.
- [ ] **Task 4.6**: Đăng Story mới: Quay video ngắn hoặc chụp ảnh và đẩy lên `story-service` (`POST /v1/stories`).

---

### Phase 5: Social Graph & Profile
- [x] **Task 5.1**: Màn hình Trang cá nhân (`ProfileScreen`):
  - [x] Header hồ sơ: Avatar gradient ring, họ tên, username, tiểu sử, số lượng bạn bè, số lượng bài viết.
  - [x] Nút "Chỉnh sửa hồ sơ" và "Chia sẻ trang cá nhân".
  - [x] Tab danh sách bài viết cá nhân (`/v1/posts/of-user/:username`) và Tab đa phương tiện.
  - [x] Nút Đăng xuất (`LogoutEvent`).
- [ ] **Task 5.2**: Chỉnh sửa hồ sơ cá nhân:
  - [ ] Cắt và đổi ảnh đại diện (Avatar), ảnh bìa (Cover).
  - [ ] Cập nhật họ tên, tiểu sử, ngày sinh, giới tính (`PUT /v1/users/profile`).
- [ ] **Task 5.3**: Quản lý Bạn bè:
  - [ ] Danh sách bạn bè hiện tại (`GET /v1/friends`).
  - [ ] Danh sách gợi ý kết bạn (`GET /v1/friends/suggested` từ Recommendation Service).
  - [ ] Danh sách lời mời kết bạn gửi đến / gửi đi (`GET /v1/friend-request`).
- [ ] **Task 5.4**: Xử lý hành động quan hệ:
  - [ ] Gửi yêu cầu kết bạn, hủy yêu cầu, chấp nhận, từ chối.
  - [ ] Hủy kết bạn (Unfriend).
- [ ] **Task 5.5**: Chặn người dùng (Block / Unblock) thông qua `/v1/blocks`.

---

### Phase 6: Real-time Chat (1-1 & Group)
- [x] **Task 6.1**: Quản lý Kết nối Chat Backend qua API Gateway (`ApiClient` & WebSocket):
  - [x] Chuẩn hóa các models dữ liệu `ChatUserModel`, `ChatMessageModel`, `ChatRoomModel`.
  - [x] Kết nối `ChatRepository` với `/v1/chat`, `/v1/chat/messages/:chatId`, `/v1/chat/send`.
- [x] **Task 6.2**: Màn hình Danh sách Chat (`ChatListScreen`):
  - [x] Danh sách phòng chat thời gian thực.
  - [x] Nội dung tin nhắn cuối cùng, nhãn thời gian, số tin nhắn chưa đọc (Unread badge màu gradient).
  - [x] Chấm xanh biểu thị trạng thái người dùng đang Online.
- [x] **Task 6.3**: Màn hình Khung Chat (`ChatConversationScreen`):
  - [x] Khung chat 1-1 cuộn tự động xuống tin nhắn mới nhất.
  - [x] Bubble tin nhắn người gửi màu Gradient rực rỡ và người nhận màu Dark Surface.
  - [x] Thanh soạn tin nhắn nhanh kèm Optimistic UI Send tức thì.
  - [x] Tích hợp nút Gọi thoại (Voice Call) và Gọi video (Video Call) trên AppBar.
- [ ] **Task 6.4**: Gửi nhận đa phương tiện trong chat:
  - [ ] Gửi hình ảnh và video có hiển thị thumbnail xem trước.
  - [ ] Ghi âm tin nhắn thoại (Voice note) và phát audio trực tiếp trong bubble chat.
- [ ] **Task 6.5**: Chat nhóm:
  - [ ] Tạo nhóm mới: Đặt tên nhóm, avatar nhóm, chọn danh sách thành viên.
  - [ ] Xem danh sách thành viên nhóm, thêm thành viên mới, rời nhóm.
- [ ] **Task 6.6**: Lưu bản nháp (Drafts) và cache tin nhắn cục bộ (Offline-first reading).

---

### Phase 7: WebRTC Voice & Video Calling
- [x] **Task 7.1**: Tích hợp cấu hình xin quyền Microphone/Camera động trong AndroidManifest.
- [x] **Task 7.2**: Lấy danh sách STUN/TURN ICE Servers từ Backend (`chat-service: /v1/chat/ice-servers`).
- [x] **Task 7.3**: Màn hình Đàm thoại Cuộc gọi Cao cấp (`CallScreen`):
  - [x] Giao diện đàm thoại Video (Local stream PiP góc trên, Remote stream tràn màn hình).
  - [x] Giao diện đàm thoại Voice (Avatar với hiệu ứng phát sáng sóng âm thanh).
  - [x] Bộ đếm thời gian cuộc gọi thực tế (`00:15`, `01:30`).
  - [x] Bộ phím nổi: Bật/tắt Mic, Bật/tắt Camera, Bật/tắt Loa ngoài (Speaker), Nút Gác máy đỏ rực rỡ.
  - [x] Kết nối trực tiếp vào nút Call Voice và Call Video trong `ChatConversationScreen`.

---

### Phase 8: Search, Notification & FCM Push
- [x] **Task 8.1**: Màn hình Khám phá & Tìm kiếm (`ExploreScreen`):
  - [x] Thanh Search Bar bo góc 16px, tìm kiếm người dùng realtime qua `/v1/search?query=...`.
  - [x] Thẻ kết quả người dùng: Avatar, Họ tên, Username, nút xem trang cá nhân.
  - [x] Lưới khám phá hình ảnh thịnh hành (Trending Explore Grid) 3 cột phong cách Instagram/Pinterest.
  - [x] Tích hợp trực tiếp vào Tab thứ 2 của `MainNavigationScreen`.
- [ ] **Task 8.2**: Trung tâm thông báo (Notification Screen):
  - [ ] Danh sách thông báo phân loại theo icon (Like, Comment, Friend request, Thông báo hệ thống).
  - [ ] Kéo để làm mới và đánh dấu đã xem tất cả.
  - [ ] Nhấn vào thông báo để chuyển hướng tới đúng màn hình (Deep Linking).
- [ ] **Task 8.3**: Tích hợp Firebase Cloud Messaging (FCM):
  - [ ] Xin quyền nhận thông báo trên iOS/Android.
  - [ ] Lấy thiết bị FCM Token và đồng bộ lên `fcm-service` khi user đăng nhập.
  - [ ] Xóa FCM Token khi user đăng xuất để bảo mật.
- [ ] **Task 8.4**: Xử lý Thông báo Foreground / Background:
  - [ ] Foreground: Hiển thị banner in-app tinh tế không che nội dung đang thao tác.
  - [ ] Background / Terminated: Đưa thông báo vào thanh trạng thái hệ thống.
- [ ] **Task 8.5**: Màn hình Cài đặt ứng dụng:
  - [ ] Tùy chọn giao diện: Sáng (Light) / Tối (Dark) / Theo hệ thống.
  - [ ] Bật/tắt âm thanh thông báo.
  - [ ] Đổi mật khẩu & cài đặt bảo mật 2 lớp (2FA).
  - [ ] Đăng xuất tài khoản an toàn.

---

### Phase 9: Performance, Testing & Release
- [ ] **Task 9.1**: Tối ưu hóa hiệu năng & bộ nhớ:
  - [ ] Đảm bảo bộ nhớ đệm hình ảnh giới hạn dung lượng để không gây tràn RAM trên thiết bị yếu.
  - [ ] Khử memory leaks trong stream WebSocket và WebRTC.
- [ ] **Task 9.2**: Viết Unit Test & Widget Test cho các Bloc/Service cốt lõi (Auth, ApiClient, PostBloc).
- [ ] **Task 9.3**: Tích hợp Firebase Crashlytics để thu thập báo cáo crash tự động.
- [ ] **Task 9.4**: Cấu hình Release Keystore cho Android và App Signing.
- [ ] **Task 9.5**: Kích hoạt pipeline GitHub Actions build tự động file `.apk` và `.aab` cho từng phiên bản release.
