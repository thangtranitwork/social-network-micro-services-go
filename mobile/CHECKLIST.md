# Mobile App Development Checklist (Flutter / Dart)

Bảng theo dõi tiến độ triển khai ứng dụng di động cho mạng xã hội `social-network-go`.
Tài liệu kiến trúc và giải pháp chi tiết: [plans/mobile_app_development_plan.md](../plans/mobile_app_development_plan.md)

---

## 📊 Tổng quan Tiến độ

| Giai đoạn | Trọng tâm | Số Task | Trạng thái |
| :--- | :--- | :---: | :---: |
| **Phase 0** | Khởi tạo Dự án & Cấu hình CI/CD | 4 | 🟡 Đang chuẩn bị |
| **Phase 1** | Core Architecture & Network Layer | 5 | ⬜ Chưa bắt đầu |
| **Phase 2** | Auth & User Onboarding | 6 | ⬜ Chưa bắt đầu |
| **Phase 3** | Bottom Navigation & Newsfeed | 6 | ⬜ Chưa bắt đầu |
| **Phase 4** | Post Creation, Media & Stories | 6 | ⬜ Chưa bắt đầu |
| **Phase 5** | Social Graph & Profile | 5 | ⬜ Chưa bắt đầu |
| **Phase 6** | Real-time Chat (1-1 & Group) | 6 | ⬜ Chưa bắt đầu |
| **Phase 7** | WebRTC Voice & Video Calling | 5 | ⬜ Chưa bắt đầu |
| **Phase 8** | Search, Notification & FCM Push | 5 | ⬜ Chưa bắt đầu |
| **Phase 9** | Performance, Testing & Release | 5 | ⬜ Chưa bắt đầu |

---

## 🛠 Chi tiết từng Giai đoạn & Checklist Tasks

### Phase 0: Khởi tạo Dự án & Cấu hình CI/CD
- [ ] **Task 0.1**: Tạo cấu trúc thư mục `mobile/` theo chuẩn Clean Architecture (Core, Features, Shared).
- [ ] **Task 0.2**: Thiết lập `pubspec.yaml` với đầy đủ các gói: `dio`, `flutter_bloc`, `flutter_webrtc`, `firebase_messaging`, `flutter_secure_storage`, `cached_network_image`, `web_socket_channel`.
- [ ] **Task 0.3**: Cấu hình quy tắc linter trong `analysis_options.yaml` và cấu hình `.gitignore`.
- [ ] **Task 0.4**: Thiết lập workflow GitHub Actions `.github/workflows/mobile-android.yml` để tự động build APK và AAB trên cloud.

---

### Phase 1: Core Architecture & Network Layer
- [ ] **Task 1.1**: Định nghĩa cấu hình môi trường đa nền tảng (`AppConfig`: Dev, Staging, Production) và cấu hình Base URL trỏ về API Gateway (`http://<ip-or-domain>:11111`).
- [ ] **Task 1.2**: Xây dựng `ApiClient` (Dio HTTP Client) với timeout, error logging, trace ID.
- [ ] **Task 1.3**: Triển khai `AuthInterceptor`:
  - [ ] Tự động gắn header `Authorization: Bearer <access_token>`.
  - [ ] Bắt lỗi `401 Unauthorized` và tự động kích hoạt **Silent Refresh Token** (`/v1/auth/refresh`).
  - [ ] Khóa hàng đợi request (Queue Lock) để tránh refresh trùng lặp.
- [ ] **Task 1.4**: Xây dựng `SecureStorageService` (Keychain / Keystore) để mã hóa lưu trữ Access Token, Refresh Token, User ID.
- [ ] **Task 1.5**: Xây dựng Design System nền tảng: Bảng màu `AppColors` (Hỗ trợ Dark/Light mode), Typography, Button, Input, Skeleton Loader components.

---

### Phase 2: Auth & User Onboarding
- [ ] **Task 2.1**: Màn hình Splash: Kiểm tra token hợp lệ trong Storage để điều hướng tự động (vào Home nếu đã đăng nhập, vào Auth nếu chưa).
- [ ] **Task 2.2**: Màn hình Đăng nhập (Login):
  - [ ] Form nhập username/email và mật khẩu với validation.
  - [ ] Gọi API `POST /v1/auth/login`.
  - [ ] Lưu trữ session an toàn vào Secure Storage.
- [ ] **Task 2.3**: Màn hình Đăng ký tài khoản (Register):
  - [ ] Đăng ký bước 1: Họ tên, email, username, mật khẩu (`POST /v1/register`).
  - [ ] Đăng ký bước 2: Xác thực mã OTP qua Email (`POST /v1/register/verify-otp`).
- [ ] **Task 2.4**: Quên mật khẩu & Đặt lại mật khẩu:
  - [ ] Yêu cầu gửi mã OTP (`POST /v1/auth/forgot-password`).
  - [ ] Xác nhận OTP và đặt mật khẩu mới (`POST /v1/auth/reset-password`).
- [ ] **Task 2.5**: Đăng nhập bằng Google (Google OAuth2).
- [ ] **Task 2.6**: Xác thực sinh trắc học (Vân tay / FaceID) để mở khóa nhanh khi quay lại app.

---

### Phase 3: Bottom Navigation & Newsfeed
- [ ] **Task 3.1**: Khung điều hướng Bottom Navigation Bar (5 Tab):
  - [ ] Tab 1: Trang chủ (Newsfeed)
  - [ ] Tab 2: Khám phá / Tìm kiếm (Explore)
  - [ ] Tab 3: Tạo bài viết mới (+)
  - [ ] Tab 4: Tin nhắn (Chats)
  - [ ] Tab 5: Trang cá nhân (Profile)
- [ ] **Task 3.2**: Triển khai Bảng tin (Newsfeed Bloc & UI):
  - [ ] Gọi API lấy bảng tin `GET /v1/posts/newsfeed` (hỗ trợ phân trang cursor/page).
  - [ ] Thao tác kéo để tải lại (Pull-to-refresh).
  - [ ] Cuộn vô tận (Infinite Scroll) với hiệu năng cao, không giật lag.
- [ ] **Task 3.3**: Component Thẻ bài viết (Post Card):
  - [ ] Header bài viết: Avatar, tên tác giả, nhãn thời gian, menu tùy chọn (Lưu, Báo cáo, Ẩn).
  - [ ] Nội dung văn bản có format link và hashtag.
  - [ ] Bộ hiển thị hình ảnh đa ảnh (Carousel / Grid 1-4 ảnh) với zoom ảnh (Photo View).
  - [ ] Trình phát video (Video Player) tự động phát/dừng khi cuộn vào khung nhìn.
- [ ] **Task 3.4**: Tương tác bài viết:
  - [ ] Nút Reaction: Thả cảm xúc (`LIKE`, `LOVE`, `HAHA`, `WOW`, `SAD`, `ANGRY`) với hiệu ứng nảy icon.
  - [ ] Nút Chia sẻ (Share Post) tạo liên kết bài viết.
  - [ ] Nút Lưu bài viết (Bookmark / Saved posts).
- [ ] **Task 3.5**: Màn hình Chi tiết bài viết & Bình luận:
  - [ ] Danh sách bình luận dạng phân cấp cây (nested comments / replies).
  - [ ] Khung nhập bình luận kèm emoji và chọn ảnh đính kèm.
  - [ ] Xóa/sửa bình luận của chính mình.
- [ ] **Task 3.6**: Thanh Tin 24h (Stories Bar) ở đầu Feed:
  - [ ] Danh sách hình tròn đại diện cho Story của bạn bè.
  - [ ] Vòng tròn viền gradient biểu thị Story chưa xem.

---

### Phase 4: Post Creation, Media & Stories
- [ ] **Task 4.1**: Chọn ảnh & video từ máy (Image Picker): Hỗ trợ chọn nhiều file, chụp ảnh/quay video trực tiếp.
- [ ] **Task 4.2**: Module nén ảnh phía Client (`flutter_image_compress`): Tự động nén ảnh về < 500KB trước khi upload để tiết kiệm băng thông di động.
- [ ] **Task 4.3**: Tải file lên MinIO thông qua `file-service`:
  - [ ] Upload trực tiếp Multipart hoặc lấy Presigned URL (`GET /v1/files/upload/presigned`).
  - [ ] Hiển thị thanh tiến trình tải lên (Upload Progress Bar).
- [ ] **Task 4.4**: Màn hình Đăng bài viết mới:
  - [ ] Nhập nội dung, tag bạn bè, chọn quyền riêng tư (`PUBLIC`, `FRIENDS`, `PRIVATE`).
  - [ ] Gọi API `POST /v1/posts`.
- [ ] **Task 4.5**: Màn hình Trình chiếu Story (Story Viewer):
  - [ ] Thanh tiến trình (Progress Bar) chia theo số lượng story của user, tự động chuyển sau 5 giây.
  - [ ] Chạm nửa trái/phải màn hình để lùi/tiến story.
  - [ ] Nhấn giữ màn hình để tạm dừng (pause).
  - [ ] Đếm lượt xem story và hiển thị danh sách người đã xem.
- [ ] **Task 4.6**: Đăng Story mới: Quay video ngắn hoặc chụp ảnh và đẩy lên `story-service` (`POST /v1/stories`).

---

### Phase 5: Social Graph & Profile
- [ ] **Task 5.1**: Màn hình Trang cá nhân (Profile Screen):
  - [ ] Header hồ sơ: Ảnh bìa, ảnh đại diện, tiểu sử, số lượng bạn bè, nút kết bạn/nhắn tin.
  - [ ] Tab lưới bài viết cá nhân, Tab hình ảnh/video đã tải lên.
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
- [ ] **Task 6.1**: Quản lý Kết nối WebSocket (`WebSocketService`):
  - [ ] Kết nối WebSocket tới `ws://<domain>:11111/v1/chat/ws` kèm header `Authorization` hoặc query token.
  - [ ] Cơ chế tự động kết nối lại khi mất mạng (Heartbeat Ping-Pong mỗi 25s).
- [ ] **Task 6.2**: Màn hình Danh sách Chat (Chat List Screen):
  - [ ] Danh sách phòng chat Direct và Group Chat.
  - [ ] Nội dung tin nhắn cuối cùng, nhãn thời gian, số tin nhắn chưa đọc (Unread badge).
  - [ ] Chấm xanh biểu thị người dùng đang Online.
- [ ] **Task 6.3**: Màn hình Khung Chat (Chat Conversation Screen):
  - [ ] Danh sách tin nhắn đảo chiều (Inverted List) cuộn siêu mượt.
  - [ ] Bubble tin nhắn phân biệt bên gửi (màu nổi bật) và bên nhận (màu nền dịu).
  - [ ] Biểu tượng trạng thái tin: Đang gửi (đồng hồ), Đã gửi (1 tick), Đã nhận (2 ticks xám), Đã đọc (2 ticks xanh).
  - [ ] Chỉ báo đang soạn tin nhắn: "Đối phương đang soạn tin...".
- [ ] **Task 6.4**: Gửi nhận đa phương tiện trong chat:
  - [ ] Gửi hình ảnh và video có hiển thị thumbnail xem trước.
  - [ ] Ghi âm tin nhắn thoại (Voice note) và phát audio trực tiếp trong bubble chat.
- [ ] **Task 6.5**: Chat nhóm:
  - [ ] Tạo nhóm mới: Đặt tên nhóm, avatar nhóm, chọn danh sách thành viên.
  - [ ] Xem danh sách thành viên nhóm, thêm thành viên mới, rời nhóm.
- [ ] **Task 6.6**: Lưu bản nháp (Drafts) và cache tin nhắn cục bộ (Offline-first reading).

---

### Phase 7: WebRTC Voice & Video Calling
- [ ] **Task 7.1**: Tích hợp Thư viện `flutter_webrtc` & cấu hình xin quyền Microphone/Camera động.
- [ ] **Task 7.2**: Lấy danh sách STUN/TURN ICE Servers từ API `GET /v1/call/ice-servers`.
- [ ] **Task 7.3**: Luồng Gọi đi (Outgoing Call):
  - [ ] Tạo WebRTC `PeerConnection`, tạo SDP Offer.
  - [ ] Gửi bản tin signaling `CALL_OFFER` qua WebSocket kèm `callType` (`VOICE` hoặc `VIDEO`).
  - [ ] Phát âm thanh chuông chờ (Ringtone).
- [ ] **Task 7.4**: Luồng Cuộc gọi đến (Incoming Call):
  - [ ] Lắng nghe sự kiện `CALL_OFFER` từ WebSocket (khi app đang chạy) hoặc từ FCM High Priority Push (khi app tắt).
  - [ ] Đổ chuông toàn màn hình.
  - [ ] Nút Chấp nhận (Answer) -> Khởi tạo SDP Answer và trao đổi ICE Candidates.
  - [ ] Nút Từ chối (Decline / Busy) -> Gửi tín hiệu `CALL_HANGUP`.
- [ ] **Task 7.5**: Màn hình Đàm thoại (Active Call Screen):
  - [ ] Render video người gọi và người nhận (Local video dạng khung nhỏ PIP, Remote video tràn màn hình).
  - [ ] Các nút thao tác: Bật/tắt mic, Bật/tắt camera, Đổi camera trước/sau, Đổi loa ngoài (Speakerphone).
  - [ ] Bộ đếm thời gian cuộc gọi thực tế.
  - [ ] Tự động dọn dẹp giải phóng camera/mic và đóng kết nối khi kết thúc cuộc gọi.

---

### Phase 8: Search, Notification & FCM Push
- [ ] **Task 8.1**: Màn hình Tìm kiếm (Global Search):
  - [ ] Tìm kiếm người dùng theo username/tên hiển thị.
  - [ ] Tìm kiếm bài viết theo từ khóa (gọi qua Search Service / Elasticsearch).
  - [ ] Lưu lịch sử tìm kiếm gần nhất.
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
