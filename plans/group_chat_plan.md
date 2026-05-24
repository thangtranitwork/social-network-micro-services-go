# Kế hoạch triển khai tính năng Group Chat

Bản kế hoạch này tập trung hoàn toàn vào việc triển khai tính năng **Group Chat** (Trò chuyện nhóm) trong `chat-service`.

---

## 1. Phân tích hiện trạng
Dựa vào mã nguồn hiện tại của `chat-service`:
- **Database**: Sử dụng **Neo4j** để quản lý các phòng chat (`Chat`) và mối quan hệ thành viên (`IS_MEMBER_OF`), sử dụng **MongoDB** để lưu lịch sử tin nhắn (`Message`).
- **WebSocket**: Hiện tại WebSocket chỉ phục vụ đồng bộ hóa tin nhắn real-time 1-1. Luồng nhận tin nhắn (`HandleIncomingMessages`) và API danh sách chat (`GetChatList`) đang bị hardcode cho mô hình 1-1 (luôn phân giải một `recipientID` hoặc đối tượng `target` duy nhất).
- **Hàm hỗ trợ**: Rất may mắn là các hàm `GetChatMembers` và `BroadcastToChat` trong `chat_service.go` đã được thiết kế sẵn sàng hỗ trợ lấy danh sách nhiều thành viên và broadcast tin nhắn đến nhiều kết nối online.

---

## 2. Thay đổi Database & Mô hình dữ liệu

### 2.1 Neo4j (Mối quan hệ thành viên)
- Bổ sung các thuộc tính cho Node `Chat` để phân biệt Group Chat và Direct Chat:
  - `isGroup` (boolean): `true` nếu là phòng chat nhóm, `false` nếu là chat 1-1.
  - `name` (string): Tên của nhóm (nếu `isGroup` là true).
  - `avatar` (string): Đường dẫn ảnh đại diện của nhóm.
  - `adminId` (string): ID của người tạo/quản trị viên nhóm.
- Mối quan hệ giữ nguyên: `(User)-[:IS_MEMBER_OF]->(Chat)`.

### 2.2 MongoDB (Dữ liệu tin nhắn)
- Model `Message` giữ nguyên vì đã có trường `chat_id` liên kết phòng chat.
- **Trạng thái đọc tin nhắn (`status`)**:
  - Đối với Group Chat, trạng thái `SENT` / `READ` dùng chung sẽ không còn phù hợp để theo dõi từng cá nhân.
  - *Giải pháp Phase 1*: Với group chat, tin nhắn được đánh dấu mặc định là `SENT`. Hoặc ta có thể nâng cấp trường `status` ở MongoDB thành mảng các user đã đọc: `read_by: []string` chứa ID của những người đã xem tin nhắn đó.

---

## 3. Thiết kế REST API Mới (Group Management)

Cần bổ sung các handler trong `ChatHandler`:

### 3.1 Tạo phòng chat nhóm
- **Endpoint**: `POST /v1/chats/groups`
- **Request Body**:
  ```json
  {
    "name": "Team Devs",
    "memberIds": ["user_id_1", "user_id_2"]
  }
  ```
- **Logic**:
  1. Tạo node `Chat` mới trong Neo4j với `isGroup: true`, `name: "Team Devs"`, `adminId: currentUserId`.
  2. Tạo liên kết `[:IS_MEMBER_OF]` từ Admin và tất cả các `memberIds` gửi lên tới node `Chat` vừa tạo.

### 3.2 Thêm thành viên vào nhóm
- **Endpoint**: `POST /v1/chats/groups/:chatId/members`
- **Request Body**:
  ```json
  {
    "memberIds": ["user_id_3"]
  }
  ```
- **Logic**: Tạo liên kết `[:IS_MEMBER_OF]` từ danh sách User tới node Chat tương ứng. (Nên phân quyền chỉ Admin hoặc thành viên cũ mới được add).

### 3.3 Xóa thành viên / Rời nhóm
- **Endpoint**: `DELETE /v1/chats/groups/:chatId/members/:userId`
- **Logic**: Xóa liên kết `[:IS_MEMBER_OF]` giữa User và Chat trong Neo4j. Nếu Admin rời nhóm, tự động chuyển quyền Admin cho người tham gia lâu nhất hoặc người tiếp theo.

### 3.4 Sửa thông tin nhóm
- **Endpoint**: `PUT /v1/chats/groups/:chatId`
- **Request Body**:
  ```json
  {
    "name": "New Team Name",
    "avatar": "/uploads/group_avatar.png"
  }
  ```

---

## 4. Cập nhật các API hiện tại

### 4.1 Cập nhật API lấy danh sách Chat (`GET /v1/chat`)
- Sửa hàm `GetChatList` trong `chat_service.go`.
- Thay đổi câu lệnh Cypher để lấy cả Direct Chat và Group Chat của user hiện tại:
  ```cypher
  MATCH (currentUser:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)
  OPTIONAL MATCH (chat)<-[:IS_MEMBER_OF]-(target:User)
  WHERE chat.isGroup = true OR target.id <> $userId
  RETURN chat.id, chat.isGroup, chat.name, chat.avatar, target.id, target.username, target.givenName, target.familyName, target.profilePictureId
  ```
- Cập nhật struct `ChatRoom` trả về cho Frontend:
  - Nếu `chat.isGroup == true`, trả về `Name` và `Avatar` lấy từ node `Chat`. Trường `Target` có thể set là `nil` hoặc chứa thông tin tóm tắt danh sách thành viên.

---

## 5. Thay đổi luồng WebSocket (Realtime)

Trong hàm `HandleIncomingMessages` (`chat_service.go`):
- **Hiện tại**: Khi nhận tin nhắn qua WebSocket, hệ thống phân giải `recipientID` từ `username` nhận được, sau đó gọi `GetOrCreateDirectChat` để lấy `chatID`.
- **Cập nhật**:
  - Phía Client gửi lên WebSocket Payload cần đính kèm trực tiếp `chatId` (cho cả Direct và Group Chat).
  - Backend kiểm tra xem `chatId` tồn tại và User gửi tin nhắn có thuộc về chat đó không (`IsMemberOfChat`).
  - Khi lưu tin nhắn vào MongoDB, đặt `RecipientID` là chuỗi rỗng `""` hoặc bỏ qua kiểm tra này đối với Group Chat.
  - Sử dụng hàm `BroadcastToChat(chatID, enrichedMessage)` để gửi tin nhắn real-time tới **tất cả** thành viên khác của phòng chat đang kết nối online.

---

## 6. Lộ trình thực hiện (Roadmap)

1. **Step 1: DB & Model Updates**
   - Định nghĩa lại cấu trúc dữ liệu `ChatRoom` phục vụ cả chat nhóm và chat đơn.
   - Thêm các thuộc tính phục vụ group chat cho Node `Chat` trong database Neo4j.
2. **Step 2: Viết câu lệnh Neo4j (Cypher) cho Group**
   - Viết các hàm Go tương tác với Neo4j: `CreateGroupChat`, `AddMembersToGroup`, `RemoveMemberFromGroup`, `CheckGroupAdmin`.
3. **Step 3: Cập nhật API lấy danh sách Chat**
   - Cập nhật hàm `GetChatList` để trả về danh sách có cả Group Chat với định dạng UI phù hợp.
4. **Step 4: Cập nhật luồng WebSocket**
   - Sửa đổi hàm `HandleIncomingMessages` để chấp nhận gửi tin nhắn qua `chatId` trực tiếp cho Group Chat và broadcast.
5. **Step 5: Hoàn thiện REST API quản lý nhóm**
   - Thêm routes và handlers cho việc Tạo nhóm, Thêm/Xóa thành viên, cập nhật thông tin nhóm.
6. **Step 6: Kiểm thử**
   - Tạo group 3 người, gửi tin nhắn và kiểm chứng:
     - Tin nhắn lưu thành công vào MongoDB với đúng `chat_id`.
     - Các thành viên online đều nhận được tin nhắn real-time qua WebSocket.
     - Lịch sử chat và danh sách chat hiển thị chính xác.
