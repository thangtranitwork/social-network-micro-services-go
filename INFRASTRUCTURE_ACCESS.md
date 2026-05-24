# Hướng Dẫn Truy Cập Hạ Tầng (Infrastructure Access Guide)

Tài liệu này cung cấp hướng dẫn chi tiết cách truy cập vào các cơ sở dữ liệu, message broker và dịch vụ lưu trữ được cấu hình trong dự án thông qua file `docker-compose.yml`.

Tất cả các dịch vụ đều được chạy dưới dạng Docker container. Hãy đảm bảo bạn đã khởi động chúng bằng lệnh:
```bash
docker-compose up -d
```

---

## 1. PostgreSQL (Auth Database)
- **Container Name**: `postgres-auth`
- **Host**: `localhost`
- **Port**: `5432`
- **Database**: `auth_db`
- **User**: `postgres`
- **Password**: `postgres`

**Công cụ đề xuất:**
- DBeaver, DataGrip, pgAdmin.
- **CLI**: `docker exec -it postgres-auth psql -U postgres -d auth_db`

---

## 2. Neo4j (Graph Database - Social & Content)
- **Container Name**: `neo4j-graph`
- **Host**: `localhost`
- **Browser Port (HTTP)**: `7474`
- **Bolt Port**: `7687`
- **User**: `neo4j`
- **Password**: `password`

**Cách truy cập:**
- **UI (Browser)**: Mở trình duyệt và truy cập http://localhost:7474
- Sử dụng thông tin đăng nhập ở trên để kết nối vào giao diện của Neo4j và thực thi các câu lệnh Cypher.

---

## 3. MongoDB (Chat History Database)
- **Container Name**: `mongodb-chat`
- **Host**: `localhost`
- **Port**: `27017`
- **Auth**: Không có xác thực (No Auth) theo cấu hình hiện tại.
- **Connection URI**: `mongodb://localhost:27017`

**Công cụ đề xuất:**
- MongoDB Compass, Robo 3T (Studio 3T).
- **CLI**: `docker exec -it mongodb-chat mongosh`

---

## 4. Redis (Cache, OTP, Token)
- **Container Name**: `redis-cache`
- **Host**: `localhost`
- **Port**: `6379`
- **Password**: Không có

**Công cụ đề xuất:**
- Redis Insight, Another Redis Desktop Manager.
- **CLI**: `docker exec -it redis-cache redis-cli`

---

## 5. Kafka (Event Broker) & Zookeeper
- **Kafka Container Name**: `kafka-broker`
- **Zookeeper Container Name**: `zookeeper`
- **Kafka Bootstrap Server (Host:Port)**: `localhost:9092`
- **Zookeeper Port**: `2181`

**Cách truy cập và kiểm tra Kafka:**
Bạn có thể sử dụng các lệnh từ bên trong container `kafka-broker`:

- *Liệt kê các topic:*
  `docker exec -it kafka-broker kafka-topics --list --bootstrap-server localhost:9092`

- *Theo dõi messages trong một topic (Consumer):*
  `docker exec -it kafka-broker kafka-console-consumer --bootstrap-server localhost:9092 --topic <tên_topic> --from-beginning`

- *Gửi message thủ công (Producer):*
  `docker exec -it kafka-broker kafka-console-producer --bootstrap-server localhost:9092 --topic <tên_topic>`

**Công cụ giao diện đề xuất:**
- Offset Explorer, Confluent Control Center (nếu cài thêm), hoặc Kafdrop.

---

## 6. MinIO (Object Storage)
- **Container Name**: `minio-storage`
- **API Host**: `localhost:9000` (dành cho SDK/Code)
- **Console Host (UI)**: `localhost:9001`
- **Access Key / User**: `minioadmin`
- **Secret Key / Password**: `minioadmin`

**Cách truy cập:**
- **UI (Console)**: Mở trình duyệt và truy cập http://localhost:9001, đăng nhập bằng Access Key và Secret Key ở trên để quản lý các bucket (vd: upload/xóa ảnh, video).
