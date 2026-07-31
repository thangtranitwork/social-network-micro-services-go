# Database Data Structure & Architecture (Go Microservices - SQL & Hybrid Neo4j Recommendation)

Tài liệu này mô tả chi tiết lược đồ cơ sở dữ liệu (Database Schema) mới nhất của hệ thống **Go Microservices**, áp dụng mô hình **Database-per-Service** kết hợp **Hybrid Polyglot Persistence** (PostgreSQL lưu dữ liệu giao dịch chính + Neo4j đóng vai trò Cỗ máy gợi ý Đồ thị).

---

## 1. Kiến trúc Tổng quan Lưu trữ (Polyglot Persistence)

```
+-----------------------------------------------------------------------------------+
|                                 GO MICROSERVICES                                  |
+-------------------+--------------------+--------------------+---------------------+
|   auth-service    |    user-service    |    post-service    |    story-service    |
+---------+---------+---------+----------+---------+----------+----------+----------+
          |                   |                    |                     |
          v                   v                    v                     v
   +--------------+    +--------------+     +--------------+      +--------------+
   | PostgreSQL   |    | PostgreSQL   |     | PostgreSQL   |      | PostgreSQL   |
   | (auth_db)    |    | (user_db)    |     | (post_db)    |      | (story_db)   |
   +--------------+    +--------------+     +--------------+      +--------------+
                              |                    |
                              +--------+-----------+
                                       | Đồng bộ Đồ thị (Async / Kafka)
                                       v
                             +--------------------+
                             |  Neo4j Graph Engine|
                             | (Chỉ dùng Gợi ý)   |
                             +--------------------+
```

1. **PostgreSQL (Database-per-Service - Dữ liệu chuẩn / CRUD)**:
   - `auth_db`: Quản lý tài khoản, mã xác thực OTP, token đặt lại mật khẩu.
   - `user_db`: Thông tin người dùng, danh sách bạn bè, lời mời kết bạn, danh sách chặn.
   - `post_db`: Bài viết, ảnh/video đính kèm, bình luận, lượt thích bài viết/comment, lượt chia sẻ.
   - `story_db`: Tin ngắn 24 giờ (Stories) và nhật ký lượt xem.
2. **Neo4j (Graph Recommendation Engine - Chỉ dùng Gợi ý)**:
   - Lưu trữ các Node ID nhẹ (`:User`, `:Post`, `:Topic`) và các đường nối quan hệ (`[:FRIEND]`, `[:SENT_REQUEST]`, `[:BLOCK]`, `[:POSTED]`, `[:LIKED]`, `[:COMMENTED]`, `[:SHARED]`).
   - Phục vụ 2 thuật toán: **Gợi ý Bạn bè ("People You May Know")** và **Xếp hạng Gợi ý Bảng tin (Newsfeed Ranking)**.
3. **MongoDB (Chat & Notifications)**:
   - `chat_db`: Lịch sử tin nhắn chat (Text, Media, Call log).
   - `notification_db`: Danh sách thông báo đẩy người dùng.
4. **Redis**:
   - Cache thông tin người dùng, đếm online realtime, WebRTC Signaling, Rate Limiting.

---

## 2. Chi tiết Lược đồ PostgreSQL (SQL Database)

### 2.1. Database `auth_db` (`auth-service`)

#### Bảng `accounts`
* **`id`** `UUID` *(Primary Key)* - ID tài khoản duy nhất.
* **`email`** `VARCHAR(255)` *(Unique Index, Not Null)* - Email đăng ký.
* **`password`** `VARCHAR(255)` *(Not Null)* - Mật khẩu mã hóa Bcrypt.
* **`role`** `VARCHAR(50)` *(Default: 'USER')* - Quyền hạn (`USER`, `ADMIN`).
* **`is_verified`** `BOOLEAN` *(Default: false)* - Trạng thái đã kích hoạt OTP.
* **`created_at`** `TIMESTAMP` *(Not Null)*.

#### Bảng `verify_codes`
* **`code`** `UUID` *(Primary Key)*.
* **`account_id`** `UUID` *(Index, Not Null)*.
* **`verified`** `BOOLEAN` *(Default: false)*.
* **`expiry_time`** `TIMESTAMP` *(Not Null)*.

#### Bảng `password_reset_tokens`
* **`code`** `UUID` *(Primary Key)*.
* **`account_id`** `UUID` *(Index, Not Null)*.
* **`used`** `BOOLEAN` *(Default: false)*.
* **`expiry_time`** `TIMESTAMP` *(Not Null)*.

---

### 2.2. Database `user_db` (`user-service`)

#### Bảng `users`
* **`id`** `VARCHAR(64)` *(Primary Key)* - Trùng với `account_id`.
* **`email`** `VARCHAR(255)` *(Unique Index)*.
* **`given_name`** `VARCHAR(64)` - Tên.
* **`family_name`** `VARCHAR(64)` - Họ.
* **`username`** `VARCHAR(32)` *(Unique Index)* - Tên người dùng (`@username`).
* **`bio`** `TEXT` - Tiểu sử.
* **`birthdate`** `TIMESTAMP` - Ngày sinh.
* **`profile_picture_id`** `VARCHAR(255)` - ID file ảnh đại diện.
* **`email_notifications`** `BOOLEAN` *(Default: true)*.
* **`push_notifications`** `BOOLEAN` *(Default: true)*.
* **`digest_frequency`** `VARCHAR(20)` *(Default: 'DAILY')*.
* **`next_change_name_date`** `TIMESTAMP` - Hạn đổi tên tiếp theo (sau 30 ngày).
* **`next_change_birthdate_date`** `TIMESTAMP`.
* **`next_change_username_date`** `TIMESTAMP`.
* **`created_at`** `TIMESTAMP`, **`updated_at`** `TIMESTAMP`, **`deleted_at`** `TIMESTAMP` *(Index)*.

#### Bảng `friends` (Mối quan hệ bạn bè hai chiều)
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`user_id`** `VARCHAR(64)` *(Unique Index `idx_user_friend`)*.
* **`friend_id`** `VARCHAR(64)` *(Unique Index `idx_user_friend`)*.
* **`created_at`** `TIMESTAMP`.

#### Bảng `friend_requests` (Lời mời kết bạn)
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`sender_id`** `VARCHAR(64)` *(Unique Index `idx_sender_receiver`)*.
* **`receiver_id`** `VARCHAR(64)` *(Unique Index `idx_sender_receiver`, Index `idx_receiver_status`)*.
* **`status`** `VARCHAR(20)` *(Default: 'PENDING', Index `idx_receiver_status`)* - `PENDING`, `ACCEPTED`, `DECLINED`.
* **`created_at`** `TIMESTAMP`.

#### Bảng `user_blocks` (Danh sách chặn)
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`blocker_id`** `VARCHAR(64)` *(Unique Index `idx_blocker_blocked`)* - Người bấm chặn.
* **`blocked_id`** `VARCHAR(64)` *(Unique Index `idx_blocker_blocked`)* - Người bị chặn.
* **`created_at`** `TIMESTAMP`.

---

### 2.3. Database `post_db` (`post-service`)

#### Bảng `posts`
* **`id`** `VARCHAR(64)` *(Primary Key)*.
* **`user_id`** `VARCHAR(64)` *(Index `idx_post_user`)* - Người đăng.
* **`content`** `TEXT` - Nội dung bài viết.
* **`privacy`** `VARCHAR(20)` *(Default: 'PUBLIC')* - `PUBLIC`, `FRIEND`, `PRIVATE`.
* **`like_count`** `INT` *(Default: 0)* - Tổng số lượt thích.
* **`comment_count`** `INT` *(Default: 0)* - Tổng số bình luận.
* **`share_count`** `INT` *(Default: 0)* - Tổng số lượt chia sẻ.
* **`original_id`** `VARCHAR(64)` - ID bài viết gốc (nếu là bài chia sẻ).
* **`created_at`** `TIMESTAMP` *(Index `idx_post_created`)*, **`updated_at`** `TIMESTAMP`, **`deleted_at`** `TIMESTAMP` *(Index)*.

#### Bảng `post_media` (Media đính kèm bài viết)
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`post_id`** `VARCHAR(64)` *(Index `idx_media_post`, Khóa ngoại CASCADE)*.
* **`file_id`** `VARCHAR(255)` - ID file trên File Service / MinIO.
* **`media_url`** `VARCHAR(512)` - Đường dẫn xem ảnh/video.
* **`media_type`** `VARCHAR(50)` - `IMAGE`, `VIDEO`, `FILE`.
* **`created_at`** `TIMESTAMP`.

#### Bảng `comments`
* **`id`** `VARCHAR(64)` *(Primary Key)*.
* **`post_id`** `VARCHAR(64)` *(Index `idx_comment_post`)*.
* **`user_id`** `VARCHAR(64)` *(Index `idx_comment_user`)*.
* **`parent_id`** `VARCHAR(64)` *(Index `idx_comment_parent`)* - ID comment cha (nếu là reply).
* **`content`** `TEXT` - Nội dung bình luận.
* **`like_count`** `INT` *(Default: 0)*.
* **`reply_count`** `INT` *(Default: 0)*.
* **`created_at`** `TIMESTAMP` *(Index `idx_comment_created`)*, **`updated_at`** `TIMESTAMP`, **`deleted_at`** `TIMESTAMP`.

#### Bảng `post_likes` & `comment_likes`
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`post_id` / `comment_id`** `VARCHAR(64)` *(Unique Index `idx_post_like` / `idx_comment_like`)*.
* **`user_id`** `VARCHAR(64)` *(Unique Index `idx_post_like` / `idx_comment_like`)*.
* **`created_at`** `TIMESTAMP`.

#### Bảng `post_shares`
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`post_id`** `VARCHAR(64)` *(Index `idx_post_share`)*.
* **`user_id`** `VARCHAR(64)` *(Index `idx_user_share`)*.
* **`created_at`** `TIMESTAMP`.

---

### 2.4. Database `story_db` (`story-service`)

#### Bảng `stories` (Tin 24 giờ)
* **`id`** `VARCHAR(64)` *(Primary Key)*.
* **`user_id`** `VARCHAR(64)` *(Index `idx_story_user`)*.
* **`media_url`** `VARCHAR(512)`.
* **`media_type`** `VARCHAR(50)` *(Default: 'IMAGE')* - `IMAGE`, `VIDEO`.
* **`caption`** `TEXT`.
* **`created_at`** `TIMESTAMP` *(Index `idx_story_created`)*.
* **`expires_at`** `TIMESTAMP` *(Index `idx_story_expires`)* - Hết hạn sau 24h.
* **`deleted_at`** `TIMESTAMP`.

#### Bảng `story_views` (Lượt xem tin)
* **`id`** `BIGSERIAL` *(Primary Key)*.
* **`story_id`** `VARCHAR(64)` *(Unique Index `idx_story_viewer`)*.
* **`viewer_id`** `VARCHAR(64)` *(Unique Index `idx_story_viewer`)*.
* **`created_at`** `TIMESTAMP`.

---

## 3. Chi tiết Cấu trúc Neo4j (Graph Recommendation Engine)

Neo4j **chỉ lưu cấu trúc Đồ thị siêu nhẹ** (Node ID + Cạnh mối quan hệ), không chứa văn bản hay media nặng.

### 3.1. Các Node Đồ thị (Graph Nodes)
- **`(:User)`**: `{ id: String, username: String }` *(Constraint UNIQUE trên `id` và `username`)*.
- **`(:Post)`**: `{ id: String, createdAt: Datetime }` *(Constraint UNIQUE trên `id`)*.
- **`(:Keyword)`**: `{ text: String }` *(Constraint UNIQUE trên `text`)*.

### 3.2. Các Cạnh Quan hệ (Relationships)
- **Bạn bè & Tương tác Bạn bè**:
  - `(:User)-[:FRIEND]-(:User)` - Quan hệ bạn bè hai chiều.
  - `(:User)-[:SENT_REQUEST]->(:User)` - Lời mời kết bạn đang chờ.
  - `(:User)-[:BLOCK]->(:User)` - Quan hệ chặn.
- **Tương tác Bài viết & Chủ đề**:
  - `(:User)-[:POSTED]->(:Post)` - Người đăng bài viết.
  - `(:User)-[:LIKED]->(:Post)` - Tương tác thả tim/thích.
  - `(:User)-[:COMMENTED]->(:Post)` - Tương tác bình luận.
  - `(:User)-[:SHARED]->(:Post)` - Tương tác chia sẻ.
  - `(:Post)-[:HAS_KEYWORDS]->(:Keyword)` - Phân loại từ khóa/chủ đề bài viết.

---

## 4. Chi tiết Cấu trúc MongoDB (Chat & Notification Services)

### Collection `messages` (`chat_db`)
* **`_id`** `ObjectId / String` *(Primary Key)*.
* **`chat_id`** `String` - ID phòng/cuộc trò chuyện.
* **`sender_id`** `String` - ID người gửi.
* **`recipient_id`** `String` - ID người nhận (nếu chat 1-1).
* **`content`** `String` - Nội dung tin nhắn.
* **`timestamp`** `ISODate`.
* **`type`** `String` - `TEXT`, `FILE`, `GIF`, `VOICE`, `CALL`.
* **`status`** `String` - `SENT`, `READ`.
* **`call_info`** `Document`: `call_id`, `call_at`, `end_at`, `is_answered`, `is_rejected`, `is_video_call`.

### Collection `notifications` (`notification_db`)
* **`_id`** `ObjectId / String`.
* **`action`** `String` - `LIKE_POST`, `COMMENT`, `FRIEND_REQUEST`, v.v.
* **`is_read`** `Boolean`.
* **`target_type`** `String` - `POST`, `COMMENT`, `USER`.
* **`target_id`** `String` - ID đối tượng bị tương tác.
* **`shortened_content`** `String` - Nội dung xem trước.
* **`creator_id`** `String` - ID người tạo sự kiện.
* **`receiver_id`** `String` - ID người nhận thông báo.
* **`sent_at`** `ISODate`.
