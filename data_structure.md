# Database Data Structure & Architecture (Go Migration vs. Legacy Java)

Tài liệu này chi tiết hóa cấu trúc dữ liệu thực tế của hệ thống, được đối chiếu trực tiếp với các Class Entity nguyên bản từ dự án **Java (Legacy)** và các thay đổi/tối ưu hóa trong bản chuyển đổi **Go (Microservices)**.

---

## 1. Mối quan hệ kiến trúc: Java (Legacy) vs Go (Microservices)
Trong bản Java cũ, hầu hết thực thể bao gồm cả Tin nhắn (`Message`), Cuộc gọi (`Call`), Nhóm chat (`Chat`), Thông báo (`Notification`) và Tài khoản (`Account`) đều được mô hình hóa dưới dạng các Node trong **Neo4j**. 
Khi chuyển đổi sang **Go microservices**, kiến trúc được phân tách thành **Polyglot Persistence** để tối ưu hóa hiệu năng:
*   **PostgreSQL** đảm nhận: Xác thực tài khoản (`Account`, `VerifyCode`, `PasswordResetToken`).
*   **MongoDB** đảm nhận: Chat và Cuộc gọi (`messages`), Thông báo (`notifications`).
*   **Neo4j** giữ lại: Đồ thị quan hệ cốt lõi (`User`, `Post`, `Comment`, `File`, `Keyword`).

---

## 2. Chi tiết cấu trúc PostgreSQL (Go Auth-Service)
*Phục vụ việc đăng nhập, bảo mật và khôi phục tài khoản.*

### Bảng `accounts`
*   **`id`** `UUID` (Primary Key)
*   **`email`** `VARCHAR(255)` (Unique Index, Not Null)
*   **`password`** `VARCHAR(255)` (Not Null) - *Mã hóa bcrypt*
*   **`role`** `VARCHAR(50)` (Not Null, Default: `'USER'`)
*   **`is_verified`** `BOOLEAN` (Not Null, Default: `false`)
*   **`created_at`** `TIMESTAMP` (Not Null)

### Bảng `verify_codes`
*   **`code`** `UUID` (Primary Key)
*   **`account_id`** `UUID` (Index, Not Null)
*   **`verified`** `BOOLEAN` (Not Null, Default: `false`)
*   **`expiry_time`** `TIMESTAMP` (Not Null)

### Bảng `password_reset_tokens`
*   **`code`** `UUID` (Primary Key)
*   **`account_id`** `UUID` (Index, Not Null)
*   **`used`** `BOOLEAN` (Not Null, Default: `false`)
*   **`expiry_time`** `TIMESTAMP` (Not Null)

---

## 3. Chi tiết cấu trúc Neo4j (Graph Database - Core Social Network)
Đối chiếu trực tiếp với các Entity Java `@Node`.

### Node `Account`
*   **Thuộc tính:** `id` (UUID), `email` (String), `password` (String), `role` (AccountRole), `isVerified` (Boolean).
*   **Mối quan hệ:**
    *   `(Account)-[:HAS_VERIFY_CODE]->(VerifyCode)`
    *   `(Account)-[:HAS_INFO]->(User)`

### Node `VerifyCode`
*   **Thuộc tính:** `code` (UUID), `verified` (Boolean), `expiryTime` (LocalDateTime).

### Node `User`
*   **Thuộc tính:** `id` (UUID), `givenName` (String), `familyName` (String), `username` (String), `birthdate` (LocalDate), `bio` (String), `friendCount` (int), `blockCount` (int), `requestSentCount` (int), `requestReceivedCount` (int), `createdAt` (ZonedDateTime), `nextChangeNameDate` (LocalDate), `nextChangeBirthdateDate` (LocalDate), `nextChangeUsernameDate` (LocalDate).
*   **Mối quan hệ:**
    *   `(User)-[:HAS_PROFILE_PICTURE]->(File)`
    *   `(User)-[:FRIEND]-(User)` (Mối quan hệ kèm thuộc tính `Friend` - xem mục 3.1)
    *   `(User)-[:SENT_FRIEND_REQUEST]->(User)` (Mối quan hệ kèm thuộc tính `Request` - xem mục 3.1)
    *   `(User)-[:BLOCK]->(User)` (Mối quan hệ kèm thuộc tính `Block` - xem mục 3.1)

### Node `Post`
*   **Thuộc tính:** `id` (UUID), `content` (String), `likeCount` (int), `shareCount` (int), `commentCount` (int), `createdAt` (ZonedDateTime), `updatedAt` (ZonedDateTime), `deletedAt` (ZonedDateTime), `privacy` (PostPrivacy - PUBLIC, FRIEND, PRIVATE).
*   **Mối quan hệ:**
    *   `(User)-[:POSTED]->(Post)` (Incoming từ User tới Post)
    *   `(User)-[:LIKED]->(Post)` (Incoming từ User tới Post)
    *   `(Post)-[:ATTACH_FILES]->(File)`
    *   `(Post)-[:SHARED]->(Post)` (Trỏ tới bài viết gốc - Shared Post)
    *   `(Post)-[:HAS_KEYWORDS]->(Keyword)`

### Node `Comment`
*   **Thuộc tính:** `id` (UUID), `content` (String), `likeCount` (int), `replyCount` (int), `createdAt` (ZonedDateTime), `updatedAt` (ZonedDateTime).
*   **Mối quan hệ:**
    *   `(Post)-[:HAS_COMMENT]->(Comment)` (Incoming từ Post tới Comment)
    *   `(User)-[:COMMENTED]->(Comment)` (Incoming từ User tới Comment)
    *   `(User)-[:LIKED]->(Comment)` (Incoming từ User tới Comment)
    *   `(Comment)-[:REPLIED]->(Comment)` (Trỏ tới comment cha gốc)
    *   `(Comment)-[:ATTACH_FILE]->(File)`

### Node `File`
*   **Thuộc tính:** `id` (String), `name` (String), `contentType` (String).
*   **Mối quan hệ:**
    *   `(User)-[:UPLOAD_FILE]->(File)` (Incoming từ User)

### Node `Keyword`
*   **Thuộc tính:** `text` (String - Khóa chính `@Id`, trong Java dùng thuộc tính `text` thay vì `word`), `score` (int).

### Node `Chat` (Đại diện cho phòng Chat/Cuộc hội thoại nhóm hoặc 1-1)
*   **Thuộc tính:** `id` (UUID), `createdAt` (ZonedDateTime).
*   **Mối quan hệ:**
    *   `(User)-[:IS_MEMBER_OF]->(Chat)` (Thành viên tham gia phòng chat)
    *   `(Chat)-[:HAS_MESSAGE]->(Message)` (*Chỉ tồn tại ở bản Java cũ, sang bản Go đã chuyển sang lưu tin nhắn ở MongoDB*)

---

### 3.1. Thuộc tính trên các Quan hệ Neo4j (RelationshipProperties)
Java định nghĩa các thuộc tính trực tiếp nằm trên đường nối quan hệ (Relationship), đây là phần dữ liệu vô cùng quan trọng:

*   **Quan hệ `[:FRIEND]` (Entity class `Friend`):**
    *   `id` (Long - Generated)
    *   `uuid` (UUID - Random)
    *   `createdAt` (ZonedDateTime)
*   **Quan hệ `[:SENT_FRIEND_REQUEST]` (Entity class `Request`):**
    *   `id` (Long)
    *   `uuid` (UUID)
    *   `sentAt` (ZonedDateTime)
*   **Quan hệ `[:BLOCK]` (Entity class `Block`):**
    *   `id` (Long)
    *   `uuid` (UUID)

---

## 4. Chi tiết cấu trúc MongoDB (Go Chat/Notification Services)
Trong bản Go, dữ liệu realtime và tốc độ cao này được chuyển dịch hoàn toàn sang MongoDB để cải thiện latency.

### Collection `messages`
*   **`_id`** `ObjectId / String` (Primary Key)
*   **`chat_id`** `String` (UUID nhóm/cuộc trò chuyện)
*   **`sender_id`** `String` (UUID người gửi)
*   **`recipient_id`** `String` (UUID người nhận nếu là 1-1)
*   **`content`** `String` (Nội dung)
*   **`timestamp`** `ISODate` (Thời gian gửi)
*   **`type`** `String` (TEXT, FILE, GIF, VOICE)
*   **`status`** `String` (SENT, READ)
*   **`call_info`** `Document` (Chỉ dành cho tin nhắn cuộc gọi - Kế thừa từ class `Call` của Java):
    *   `call_id` (String)
    *   `call_at` (ISODate)
    *   `end_at` (ISODate)
    *   `is_answered` (Boolean)
    *   `is_rejected` (Boolean)
    *   `is_video_call` (Boolean)

### Collection `notifications`
*   **`_id`** `ObjectId / String`
*   **`action`** `String` (LIKE_POST, COMMENT, FRIEND_REQUEST, v.v.)
*   **`is_read`** `Boolean`
*   **`target_type`** `String` (POST, COMMENT, USER)
*   **`target_id`** `String` (UUID của đối tượng bị tương tác)
*   **`shortened_content`** `String` (Tóm tắt thông báo hiển thị nhanh)
*   **`creator_id`** `String` (UUID người tạo thông báo)
*   **`receiver_id`** `String` (UUID người nhận thông báo)
*   **`sent_at`** `ISODate`

---

## 5. Chi tiết cấu trúc SQLite (Chỉ dùng lưu Log Online)
Tương ứng với Java `sqlite/OnlineUserLog.java`, dùng để vẽ biểu đồ thống kê lượng truy cập trong Admin Dashboard:

### Bảng `online_user_logs`
*   **`id`** `INTEGER` (Primary Key - Autoincrement)
*   **`timestamp`** `DATETIME`
*   **`online_count`** `INTEGER`

---

## 6. Redis & Kafka Architecture

### 6.1. Redis Caching & Signaling
*   `user_online_counter:<userID>`: `Int` (Số lượng Socket client đang kết nối)
*   `last_online:<userID>`: `String` (Thời điểm ngắt kết nối cuối cùng)
*   `online_user_count`: `Int` (Bộ đếm atomic tổng lượng user online)
*   `call_signaling:*` / `webrtc_room:*`: Pub/Sub điều phối luồng WebRTC

### 6.2. Kafka Event Stream Broker
*   Topic `user.events`: Đồng bộ hóa trạng thái tài khoản PostgreSQL -> Neo4j User Node.
*   Topic `post.events`: Đồng bộ trạng thái bài viết/tương tác -> Notification Service & AI Service.
*   Topic `notification.events`: Đẩy tin nhắn realtime qua Socket / WebPush.
*   Topic `ai.events`: Trigger quét kiểm duyệt tự động hình ảnh/văn bản.
