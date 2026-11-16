# Kế hoạch dùng lượt xem thực tế cho lịch sử và newsfeed ranking

Ngày lập kế hoạch 2026-11-11. Trạng thái đề xuất, chưa triển khai.

## Mục tiêu và cơ sở đánh giá

Thay thành phần phạt LOADED trong ranking bằng tín hiệu bài đã được hiển thị đủ điều kiện. Cassandra lưu từng sự kiện phục vụ lịch sử. Neo4j giữ trạng thái xem gần đây phục vụ ranking. Các tham số dưới đây là điểm khởi đầu để benchmark và đánh giá sản phẩm, chưa phải cấu hình đã được duyệt.

Đánh giá dựa trên mã nguồn local. Chưa có số liệu production, execution plan thực tế hoặc benchmark. Chưa xác nhận constraint Post.id đang tồn tại trên database chạy thực tế.

Các đường xử lý hiện tại cần được bao phủ

- `post-service/service/post.go` lấy ứng viên cho RELEVANT rồi tính điểm trong Go. Nhánh này ghi LOADED sau bước kiểm tra quyền xem.
- `post-service/service/newsfeed_scorer.go` dùng `LoadedTimes * LoadedPenalty`.
- `post-service/repository/post_repository.go` còn query default tính totalScore bằng Cypher. TIME và FRIEND_ONLY có đường ghi LOADED trong query lấy bài.
- `post-service/repository/sql_post_repository.go` triển khai repository thứ hai, dùng chung candidate query nhưng có logic ghi LOADED riêng.
- `post-service/main.go` chọn SQLPostRepository khi PostgreSQL được khởi tạo. Phải kiểm chứng cả hai implementation qua entry point bình thường.
- `post-service/service/keyword_publisher.go` có publisher Kafka bất đồng bộ. Endpoint nhận lượt xem cần xác nhận broker đã nhận trước khi báo thành công.
- Consumer search hiện dùng ReadMessage. Worker mới cần FetchMessage và commit sau khi xử lý thành công.
- `plans/newsfeed_recommendation_improvement_plan.md` mô tả một phần trạng thái cũ. Kế hoạch này đối chiếu mã nguồn hiện tại.

## Rủi ro khi cập nhật Neo4j liên tục

1. Cập nhật cùng quan hệ có thể tranh chấp write lock. Tạo quan hệ mới cũng phát sinh lock liên quan đến endpoint và cấu trúc graph. Bài viral phải được benchmark riêng. Cơ chế lock cụ thể phụ thuộc phiên bản và storage format.
2. Mỗi transaction có chi phí commit, transaction log và I/O. Ghi lượt xem có thể cạnh tranh tài nguyên với đọc newsfeed và ghi like/comment.
3. Tạo quan hệ VIEWED cho mọi cặp user–post từng xem làm graph tăng theo số cặp duy nhất. Một quan hệ tổng hợp vẫn có thể dẫn đến hàng triệu quan hệ.
4. Counter tăng trực tiếp dễ đếm trùng do retry và Kafka redelivery. Cửa sổ đếm còn cần cơ chế hết hạn hoặc tổng hợp lại.
5. Tăng concurrency hoặc kích thước batch quá mức có thể kéo dài lock, tăng deadlock và gây backlog.
6. Chuyển từng event sang Kafka rồi ghi từng event vào Neo4j chỉ chuyển tải sang nền. Throughput xử lý bền vững vẫn phải lớn hơn tốc độ sự kiện đến.

Ví dụ minh họa công suất

- 1.000 event/giây đều thuộc cặp user–post khác nhau vẫn cần khoảng 1.000 thay đổi quan hệ/giây.
- Batch 200 phần tử có thể giảm xuống khoảng 5 transaction/giây, trong điều kiện batch đầy. Số thay đổi quan hệ giữ nguyên.
- Gom theo cặp trong cửa sổ 2 giây chỉ giảm số thay đổi nếu cùng cặp xuất hiện nhiều lần trong cửa sổ.
- Có thể ghi ít hơn hiện tại nếu mỗi response trả nhiều bài nhưng người dùng chỉ nhìn vài bài. Đây là giả thuyết phải đo.

## Quyết định kiến trúc đề xuất

```text
Web/mobile đo qualified_view
  -> API trong post-service xác thực, kiểm tra và publish Kafka
      -> consumer group history lưu sự kiện vào Cassandra
      -> consumer group ranking gom theo user–post rồi cập nhật Neo4j

GET newsfeed
  -> Neo4j lấy quan hệ và trạng thái xem gần đây
  -> scorer Go cho RELEVANT hoặc query Cypher ở đường repository tương ứng

GET view-history
  -> Cassandra lấy event theo user và bucket thời gian
  -> kiểm tra quyền hiện tại và lấy thông tin bài theo batch
```

Hai consumer group đọc cùng topic độc lập. Kafka là điểm nhận sự kiện bền vững trong thời gian retention của topic. Cassandra là kho lịch sử dài hơn. Trạng thái history và ranking có thể cập nhật ở hai thời điểm khác nhau và cần đo lag riêng. API không dual-write Cassandra và Neo4j.

Bản đầu dùng `(:User)-[:VIEWED {lastViewedAt}]->(:Post)`. Dùng timestamp lớn nhất làm phép cập nhật idempotent. Chưa đưa viewCount hoặc tổng dwell time vào ranking. Nếu cần thêm đếm chính xác, phải thiết kế snapshot tổng hợp cùng version/checkpoint bền vững trước khi mở rộng.

Giữ ngữ nghĩa lịch sử từng lượt xem ở bản đầu. Yêu cầu chỉ hiển thị mỗi bài một lần cần thiết kế read model riêng, vì việc loại trùng qua nhiều trang có thể phải đọc rất nhiều sự kiện.

## Hợp đồng sự kiện và API

Endpoint đề xuất `POST /v1/posts/view-events`. DTO nằm trong package DTO tương ứng của post-service.

Mỗi event có `eventId`, `postId`, `observedAt`, `visibleDurationMs`, `visibleRatio`, `surface`, `schemaVersion`. Backend xác định user từ danh tính đã xác thực. Client giữ nguyên ID và payload khi retry. `observedAt` dùng UTC và bất biến, cũng xác định bucket Cassandra.

Thông số thử nghiệm

| Tham số | Giá trị đề xuất |
|---|---|
| Điều kiện qualified view | Ít nhất 50% diện tích hiển thị liên tục trong 2 giây |
| Điều kiện foreground | Tab đang hiển thị, app đang active |
| Phát sinh event mới | Một lần mỗi đợt hiển thị đạt điều kiện, chỉ bắt đầu đợt mới sau khi bài rời viewport |
| Flush client | Mỗi 2 giây hoặc tối đa 20 event |
| Giới hạn payload API | Tối đa 50 event và 64 KiB mỗi request |
| Thời gian client được chấp nhận | Tối đa 10 phút quá khứ và 30 giây tương lai |
| Worker flush | Mỗi 2 giây hoặc tối đa 200 cặp trong transaction |
| Giới hạn memory worker | Queue theo partition có hard cap, pause fetch khi chạm cap |
| Mục tiêu ranking freshness | p95 từ broker acknowledgement đến cập nhật Neo4j dưới 5 giây ở tải mục tiêu |
| Lưu lịch sử Cassandra | 90 ngày tính từ observedAt |
| Giữ quan hệ VIEWED | 48 giờ tính từ lastViewedAt cho cửa sổ phạt 24 giờ |

Các ngưỡng cần được phê duyệt qua review sản phẩm và benchmark. Bài cao hơn viewport cần kiểm thử riêng vì có thể không đạt 50%. Ngưỡng theo loại nội dung chỉ được thêm bằng quy tắc rõ ràng.

API kiểm tra schema, timestamp, số lượng, kích thước, postId và quyền xem theo batch. Sai dữ liệu trả 400, lỗi xác thực trả 401, vượt rate limit trả 429. Quy tắc trả lỗi quyền xem hoặc bài không còn tồn tại phải theo chuẩn hiện có và tránh tiết lộ bài riêng tư. Lỗi publish trả 503. Chỉ trả 202 sau acknowledgement của Kafka cho toàn bộ request. Nếu publish thành công một phần rồi lỗi, client retry cùng ID cho cả batch và consumer xử lý idempotent.

Event key Kafka là userId để gom lượt xem của cùng người dùng vào một partition. Producer cần cấu hình hash theo key. Worker xử lý tuần tự từng partition, giới hạn tổng worker. Không tăng số partition trực tiếp trong lúc còn backlog nếu dựa vào thứ tự theo user. Bounded retry với backoff/jitter và timeout cho từng thao tác.

Retention Kafka thử nghiệm là 7 ngày, cần xác minh dung lượng và khoảng outage phải chịu được. Alert theo tuổi backlog trước khi chạm retention. Nếu raw consumer không bắt kịp trước khi Kafka hết retention, sự kiện có thể mất khỏi history. Nếu ranking consumer vượt retention nhưng raw history còn đủ, chạy rebuild có kiểm soát từ Cassandra. Thao tác rebuild phải tôn trọng deletion marker và retention graph.

Đo từ frontend là tín hiệu hành vi có thể bị giả mạo. Áp dụng rate limit theo user và kiểm tra giá trị hợp lệ. Không dùng tín hiệu này để thanh toán impression quảng cáo. Retry của client chỉ hỗ trợ trong cửa sổ timestamp đã công bố, UI/telemetry phải phản ánh sự kiện không gửi được khi quá hạn.

## Ghi Cassandra

- Bảng `views_by_user_month`, partition key `(user_id, month)`, clustering key `(observed_at, event_id)`, thứ tự mới nhất trước. Kích thước bucket được xác nhận bằng số liệu tải, có thể dùng ngày nếu partition tháng quá lớn.
- Lưu postId và dữ liệu event cần thiết. Payload bất biến với eventId. Retry ghi cùng primary key, cùng giá trị. Không dùng counter cho luồng này.
- TTL còn lại được tính từ hạn hết hạn cố định `observedAt + retention`. Replay không kéo dài vòng đời dữ liệu. Event đã hết hạn được phân loại rõ, không tái ghi.
- Commit offset sau ghi thành công ở consistency level đã chọn theo topology. PoC kiểm chứng RF=3 và LOCAL_QUORUM nếu có đủ ba node trong DC. Cấu hình single-node local chỉ phục vụ kiểm tra chức năng.
- Compaction, repair, dung lượng replication và tombstone phải được benchmark cùng workload TTL. Chọn phiên bản Cassandra và driver Go trước implementation, xác minh compatibility qua tài liệu chính thức.
- Đọc lịch sử dùng cursor opaque chứa bucket và vị trí. Backend batch kiểm tra quyền và lọc bài bị xóa, block hoặc đổi privacy. Có giới hạn số bucket và số event quét mỗi request, trả cursor tiếp tục nếu chưa đủ bài hợp lệ.

## Ghi Neo4j và tính nhất quán

1. Gom các event cùng `(userId, postId)` trong batch, giữ observedAt lớn nhất.
2. Bỏ event đã ngoài retention ranking với reason metric rõ ràng. Event được xếp vào bucket raw vẫn có thể hợp lệ cho history.
3. Ghi batch bằng UNWIND có tham số, match User và Post theo ID đã được index. Chỉ tạo quan hệ với node hiện hữu và bài chưa xóa. Missing User hoặc Post được phân loại, Post có thể chưa sync từ PostgreSQL nên cần retry có giới hạn trước khi chuyển DLQ có acknowledgement.
4. Cập nhật `lastViewedAt = max(lastViewedAt hiện tại, observedAt)` trong transaction có bảo vệ read-modify-write bằng write lock. Khi implementation phải chứng minh trên Neo4j 5 hiện dùng. Một biểu thức CASE đọc property tự nó chưa đủ để khẳng định an toàn đồng thời.
5. Bắt đầu với một worker xử lý tuần tự mỗi partition. Kiểm thử consumer rebalance và worker cũ chưa kết thúc, không chỉ kiểm thử một writer. Sort batch theo thứ tự key ổn định để giảm deadlock, retry transaction lỗi tạm thời có giới hạn.
6. Commit Kafka offset sau transaction thành công. Chỉ commit tới offset liên tục đã hoàn thành trong mỗi partition. Không commit vượt event đang chờ flush, retry hoặc DLQ. Kafka commit lỗi sau Neo4j commit dẫn đến replay, phép max giữ nguyên kết quả.
7. Khi Neo4j lỗi, dừng tiến độ partition liên quan và báo lag. Không chuyển sang thành công giả hoặc quay lại LOADED tự động. Lỗi payload không phục hồi được phải ghi DLQ bền vững trước khi commit.
8. Worker có context, deadline, giới hạn connection/concurrency và xử lý shutdown. Event đang nằm trong RAM chưa commit sẽ được replay sau restart.

Retention graph được dọn bằng job batch nhỏ và có ngân sách I/O. Điều kiện xóa phải kiểm tra lại trong transaction có lock để không xóa quan hệ vừa được làm mới. Account/history deletion cần chặn event cũ đang nằm trong Kafka hoặc DLQ tạo lại dữ liệu. Thiết kế deletion marker hoặc epoch có hiệu lực suốt thời gian replay trước khi cung cấp tính năng xóa lịch sử.

Công thức ranking bản đầu

```text
viewPenalty = -weight × min(1, max(0, 1 - ageSinceLastView / decayWindow))
```

`weight` và `decayWindow` là cấu hình hợp lệ bắt buộc cho mode mới. Ví dụ thử nghiệm weight=40, decayWindow=24 giờ. Chưa xem thì phạt 0. Vừa xem phạt 40, sau 12 giờ phạt 20, từ 24 giờ phạt 0. Cấu hình phải bảo đảm retention graph bao phủ decayWindow cùng khoảng trễ xử lý đã cam kết. Không cộng đồng thời penalty LOADED. Các giá trị này cần được đánh giá ở shadow trước khi bật ranking.

Go và Cypher phải dùng cùng timestamp tham chiếu của request, đơn vị thời gian và công thức. Tín hiệu xem được áp dụng trước sort và phân trang. TIME và FRIEND_ONLY vẫn theo thời gian, nhận diện lượt xem từ client nhưng không thêm phạt làm thay đổi thứ tự của hai chế độ này.

## Các bước triển khai và tiêu chí hoàn thành

| Bước | Công việc và phạm vi dự kiến | Phụ thuộc | Tiêu chí kiểm chứng |
|---|---|---|---|
| 1 | Đo baseline qua profiler hiện có, thống kê event giả lập, kiểm tra index/constraint và EXPLAIN candidate/write query trên local/dev | Không | Có p50/p95/p99 feed, writes/giây, tài nguyên và baseline query plan, xác định rõ route RELEVANT/default/SQL |
| 2 | Chốt DTO, cấu hình chế độ, event schema và ngữ nghĩa qualified view trong post-service | 1 | Có fixture hợp lệ/sai, schema version và bảng mã lỗi, cấu hình thiếu hoặc sai bị từ chối rõ ràng |
| 3 | Endpoint ingestion, publisher Kafka có acknowledgement và route qua api-gateway | 2 | 202 chỉ sau broker ack, không giả mạo userId, retry batch giữ ID, lỗi từng phần được kiểm thử |
| 4 | Hook quan sát viewport cho PostCard và gắn collector ở newsfeed web | 3 | Trả 20 bài nhưng xem 3 chỉ có 3 qualified event, tab ẩn và scroll nhanh không đạt ngưỡng, remount/retry có hành vi đã định nghĩa |
| 5 | Consumer history cùng repository Cassandra và lifecycle cấu hình | 2, 3 | Restart/replay không tăng số row hoặc kéo dài retention, giới hạn partition và memory đạt yêu cầu |
| 6 | API lịch sử, cursor và batch hydrate/authorization | 5 | Phân trang qua bucket ổn định, bài đổi privacy không lộ, retry cùng event không nhân đôi dòng lịch sử |
| 7 | Consumer ranking, gom batch và repository Neo4j VIEWED | 3 | Monotonic timestamp qua duplicate/out-of-order/rebalance, pending event không bị commit sớm, backlog phục hồi sau lỗi |
| 8 | Thêm tín hiệu candidate và thay thành phần penalty trong Go/Cypher/score debug cho cả hai repository | 7 | Fixture điểm Go/Cypher tương đương, phạt trước phân trang, không phạt do chỉ load API |
| 9 | Shadow và metrics, benchmark mixed load rồi bật mode mới cho cohort nhỏ | 4, 5, 7, 8 | Đạt ngân sách latency/freshness và không mất event đã ack trong các thử nghiệm lỗi |
| 10 | Dừng ghi LOADED ở các đường feed đã chuyển đổi, triển khai retention và kiểm thử replay/deletion | 9 | Feed request hết ghi LOADED, query không còn dùng LOADED penalty ở mode mới, cleanup không xóa trạng thái vừa cập nhật |

Tách từng bước thành patch nhỏ khoảng 2–5 file. Bước có nhiều layer chia tiếp theo hợp đồng DTO đã chốt. Checkpoint sau bước 4, 8 và 10 bằng review diff và kiểm thử các acceptance criteria.

Mobile là đợt tích hợp riêng sau web. Trong shadow vẫn có thể dùng ranking hiện tại. Khi chuyển sang mode qualified_view, client chưa hỗ trợ telemetry sẽ không phát sinh penalty xem. Phải đo coverage và chọn cohort theo client version. Không diễn giải LOADED cũ thành lượt xem thật.

## Đo tải và điều kiện quyết định

- Đo đồng thời traffic đọc feed, ghi bài, like/comment và lượt xem. Bao gồm phân bố user bình thường, user hoạt động nhiều, bài viral, nhiều cặp duy nhất và nhiều event trùng cùng cặp.
- Chạy lần lượt tải mục tiêu và gấp đôi tải mục tiêu trên cùng tài nguyên baseline. Ghi nhận tốc độ tạo quan hệ mới, cập nhật quan hệ cũ và dọn retention riêng.
- Ví dụ 10.000 user, mỗi user có 100 cặp VIEWED còn hiệu lực tạo khoảng một triệu quan hệ. Retention 48 giờ giới hạn tuổi dữ liệu, chưa đủ chứng minh graph nhỏ. Cần mô hình dung lượng theo distinct pair thực tế và throughput cleanup.
- Gate đề xuất ở tải mục tiêu là p95 feed tăng không quá 10% so với baseline, ranking freshness p95 dưới 5 giây, backlog không tăng liên tục và không có lỗi correctness. Ngân sách tài nguyên phải nằm trong dung lượng triển khai được cấp. Các gate được xác nhận sau bước 1.
- Metrics gồm event nhận/ack/reject, Kafka lag và tuổi event cũ nhất chưa xử lý, Cassandra write latency/errors, Neo4j transaction latency/lock wait/retries, số cặp trước/sau gom, VIEWED cardinality, feed latency và tuổi projection.
- Khi không đạt gate, hạ worker concurrency hoặc giảm batch để đo lại, hạn chế tốc độ ingest rõ ràng và điều chỉnh yêu cầu freshness qua review. Nếu throughput unique pair vẫn vượt sức Neo4j, đánh giá lưu trạng thái xem ở Redis và batch enrich trước Go scorer RELEVANT. Phương án đó cần thay đổi đường Cypher scoring tương ứng và là quyết định kiến trúc tiếp theo.

## Kiểm thử và rollout

Kiểm thử có mục đích cho công thức decay, malformed payload, authentication, retry cùng event, publish một phần, writer crash trước/sau commit, out-of-order, rebalance, missing graph node, quyền xem và retention race. Integration tests chạy trên dịch vụ local/dev chuyên dụng.

Lệnh dự kiến khi triển khai

```bash
free -h
go test ./post-service/... ./api-gateway/...
go test -race ./post-service/consumer/... ./post-service/service/...
go build ./post-service/... ./api-gateway/...
go vet ./post-service/... ./api-gateway/...
```

Package consumer là package dự kiến được tạo ở bước 5/7. Kiểm tra RAM ngay trước build và chạy race với mức concurrency phù hợp tài nguyên. Kiểm thử browser phải bao gồm viewport, tab background, bài dài, infinite scroll, component remount và retry mạng.

Rollout dùng cấu hình rõ ràng với các mode LOADED hiện tại, SHADOW và QUALIFIED_VIEW. SHADOW tính điểm mới để so sánh nhưng vẫn phục vụ điểm hiện tại. QUALIFIED_VIEW dùng điểm mới duy nhất. Khi lỗi hạ tầng, hiển thị lỗi/metric theo hợp đồng và giữ lag quan sát được, không tự đổi mode. Rollback là thao tác vận hành có chủ đích đổi cấu hình đã kiểm tra. Nếu cần rollback về LOADED sau khi ngừng ghi, phải nêu rõ dữ liệu LOADED bị thiếu trong thời gian đó.

Các quyết định còn cần xác nhận khi review kế hoạch là ngưỡng qualified view cho từng loại nội dung, SLA freshness, retention, tập client ban đầu và tải mục tiêu. Không có thao tác deploy, migration, tạo index hoặc thay đổi dữ liệu nào được thực hiện trong bước lập kế hoạch này.

## Tài liệu tham chiếu

- [Neo4j concurrent data access](https://neo4j.com/docs/operations-manual/current/database-internals/concurrent-data-access/) mô tả write lock, tranh chấp và deadlock. Implementation cần đối chiếu Neo4j 5 đang chạy trong repo.
- [kafka-go explicit commits](https://github.com/segmentio/kafka-go#explicit-commits) mô tả FetchMessage, CommitMessages và commit offset cao nhất theo partition.
- [Cassandra data modeling](https://cassandra.apache.org/doc/stable/cassandra/developing/data-modeling/intro.html) hướng dẫn thiết kế theo truy vấn và giới hạn partition.
- [Cassandra compaction](https://cassandra.apache.org/doc/stable/cassandra/managing/operating/compaction/overview.html) mô tả TTL và tombstone.
- [Đo thời gian hiển thị trên web](https://developer.mozilla.org/en-US/docs/Web/API/Intersection_Observer_API/Timing_element_visibility) minh họa kết hợp IntersectionObserver và Page Visibility API.
