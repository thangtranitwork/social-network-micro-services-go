# Plan: Nâng cấp Cơ chế Gợi ý Bài viết Newsfeed

## Summary

Mục tiêu là làm cơ chế gợi ý bài viết ổn định, đúng quyền riêng tư và dễ nâng cấp hơn. Hướng đi đề xuất là không bỏ ngay rule-based ranking hiện tại, mà biến nó thành baseline sạch, đo được và có test. Sau đó tách scoring ra khỏi Cypher để dễ điều chỉnh weight, A/B test và mở đường cho recommendation service hoặc ML reranker ở phase sau.

## Current State

- Frontend gọi `GET /v1/posts/newsfeed` với `type`: `RELEVANT`, `TIME`, `FRIEND_ONLY`.
- `post-service` đang xử lý ranking trực tiếp trong `Neo4jPostRepository.GetSuggestedPosts`.
- `RELEVANT` dùng heuristic score từ:
  - độ mới bài viết,
  - like/comment/share count,
  - quan hệ bạn bè, bạn chung, request friend, view profile,
  - keyword interest qua `HAS_KEYWORDS` và `INTERACT_WITH`,
  - penalty cho bài đã từng load qua `LOADED`.
- `TIME` chủ yếu sort theo `createdAt DESC`.
- `FRIEND_ONLY` chỉ lấy bài của bạn bè và sort mới nhất.
- `post-service` đã publish keyword events và inject `KafkaKeywordPublisher`.
- `ai-service` đã consume `post-events` và ghi quan hệ `Post-[:HAS_KEYWORDS]->Keyword` vào Neo4j.

## Problems To Fix First

1. Quan hệ `FRIEND` dùng không nhất quán:
   - Tạo friendship bằng pattern vô hướng `MERGE (u1)-[:FRIEND]-(u2)`.
   - Một số query newsfeed lại dùng hướng `[:FRIEND]->`.
   - Rủi ro: thiếu bài bạn bè hoặc tính sai quyền xem bài `FRIEND`.

2. `PostsLoaded` đang là no-op:
   - Service gọi `KeywordInteractor.PostsLoaded`, nhưng implementation hiện chỉ `return nil`.
   - Query `RELEVANT` vẫn tự `MERGE LOADED`, nhưng hook này gây hiểu nhầm và chưa có test rõ ràng.

3. Scoring nằm trong Cypher dài:
   - Khó test từng thành phần điểm.
   - Khó chỉnh weight hoặc thử nghiệm nhiều strategy.
   - Dễ tạo bug precedence trong biểu thức Cypher.

4. Chưa có test đủ cho ranking:
   - Cần test quyền riêng tư, block, friend-only, relevant score, pagination và duplicate behavior.

## Architecture Decisions

- Giữ rule-based ranking làm baseline trong ngắn hạn.
- Sửa correctness trước khi đổi kiến trúc.
- Cypher nên ưu tiên candidate generation và visibility filtering.
- Go service nên dần nhận trách nhiệm scoring để test và quan sát dễ hơn.
- Recommendation service hoặc ML reranker chỉ nên thêm sau khi baseline sạch và có metrics.
- Mọi phase phải giữ fallback behavior: nếu ranking nâng cao lỗi thì feed vẫn trả được bài hợp lệ.

## Target Design

```text
Frontend
  -> API Gateway
    -> post-service /v1/posts/newsfeed
      -> candidate query in Neo4j
      -> visibility validation
      -> baseline scorer in Go
      -> optional reranker later
      -> ads interleave
      -> author/file enrichment
```

## Implementation Plan

### Phase 1: Fix Correctness Baseline

#### Task 1: Chuẩn hóa friendship query trong newsfeed [ĐÃ HOÀN THÀNH]

**Description:** Đổi các query newsfeed dùng `FRIEND` sang pattern vô hướng nhất quán, ví dụ `[:FRIEND]-`, để khớp cách tạo quan hệ bạn bè hiện tại.

**Acceptance criteria:**
- [x] `FRIEND_ONLY` lấy đúng bài bạn bè dù hướng relationship trong Neo4j là chiều nào.
- [x] `RELEVANT` cho phép xem bài `FRIEND` đúng khi viewer là bạn của author.
- [x] Shared post kiểm tra friendship của original author không phụ thuộc chiều relationship.

**Verification:**
- [x] `go test ./post-service/...`
- [x] Thêm unit test bảo vệ query newsfeed không dùng directed `FRIEND` pattern.

**Files likely touched:**
- `post-service/repository/post_repository.go`
- `post-service/handler/handler_test.go` hoặc test repository/service mới

**Estimated scope:** M

#### Task 2: Làm rõ hoặc triển khai `PostsLoaded` [ĐÃ HOÀN THÀNH]

**Description:** Quyết định một nguồn ghi `LOADED` duy nhất. Nếu query repository đã ghi `LOADED`, bỏ hook dư hoặc đổi hook thành implementation thật và chuyển write ra service layer.

**Acceptance criteria:**
- [x] Không còn hook no-op gây hiểu nhầm.
- [x] `LOADED.times` tăng đúng khi user load feed `RELEVANT`.
- [x] Không ghi `LOADED` cho ad post synthetic.

**Verification:**
- [x] `go test ./post-service/service`
- [x] Unit test giữ lại `MERGE (viewer)-[l:LOADED]->(p)` trong relevant query.

**Files likely touched:**
- `post-service/service/keyword_publisher.go`
- `post-service/service/post.go`
- `post-service/repository/post_repository.go`

**Estimated scope:** S-M

#### Task 3: Thêm test visibility và block cho newsfeed [ĐÃ HOÀN THÀNH]

**Description:** Bổ sung test bảo vệ các rule quan trọng: public, friend, private, block, shared original post.

**Progress note:** Đã thêm query guard test cho privacy/block/origin visibility ở cả legacy suggested query và production candidate query để tránh regression khi sửa Cypher.

**Acceptance criteria:**
- [x] User không thấy bài `PRIVATE` của người khác.
- [x] User thấy bài `FRIEND` khi là bạn, không thấy khi không là bạn.
- [x] Block hai chiều đều loại bài khỏi feed.
- [x] Shared post bị ẩn original content khi viewer không có quyền xem original.

**Verification:**
- [x] Query guard test cho visibility/block/origin visibility.
- [x] `go test ./post-service/...`

**Files likely touched:**
- `post-service/repository/post_repository_test.go` hoặc test service tương ứng
- `post-service/service/post.go`

**Estimated scope:** M

### Checkpoint: Baseline Correctness

- [x] `go test ./post-service/...` pass.
- [x] `GET /v1/posts/newsfeed?type=RELEVANT` trả bài đúng privacy/block.
- [x] `GET /v1/posts/newsfeed?type=FRIEND_ONLY` không bỏ sót bạn bè do hướng relationship.
- [x] Không có duplicate bất thường khi phân trang.

### Phase 2: Tách Candidate Query và Scoring

#### Task 4: Tạo model candidate cho newsfeed [ĐÃ HOÀN THÀNH]

**Description:** Tạo cấu trúc dữ liệu trung gian chứa post và feature dùng để tính điểm, ví dụ recency, engagement, relationship, keyword, loaded count.

**Acceptance criteria:**
- [x] Query có thể trả đủ feature cần thiết cho scorer.
- [x] Mapping từ Neo4j record sang candidate rõ ràng.
- [x] Không thay đổi response JSON của API.

**Verification:**
- [x] Unit test mapping candidate.
- [x] `go test ./post-service/...`

**Files likely touched:**
- `post-service/model/model.go`
- `post-service/repository/post_repository.go`
- `post-service/service/post_service.go`

**Estimated scope:** M

#### Task 5: Tách scorer rule-based sang Go [ĐÃ HOÀN THÀNH]

**Description:** Chuyển công thức `totalScore` từ Cypher dài sang Go function testable. Cypher chỉ lấy candidate hợp lệ và feature thô.

**Progress note:** Production path `RELEVANT` đã lấy candidate thô rồi rank/paginate bằng Go scorer. Legacy helper `getSuggestedPostsQuery(PageTypeRelevant)` vẫn còn trong repository để giữ fallback query cũ, nhưng service không còn gọi path đó cho feed `RELEVANT`.

**Acceptance criteria:**
- [x] Có `ScoreNewsfeedCandidate` hoặc component tương đương.
- [x] Unit test từng thành phần điểm: recency, engagement, relationship, keyword, loaded penalty.
- [x] Ranking output gần tương đương baseline hiện tại ở mức công thức unit test.

**Verification:**
- [x] `go test ./post-service/service`
- [x] So sánh top N feed trước/sau trên fixture nhỏ.

**Files likely touched:**
- `post-service/service/newsfeed_scorer.go`
- `post-service/service/newsfeed_scorer_test.go`
- `post-service/repository/post_repository.go`

**Estimated scope:** M

#### Task 6: Chuẩn hóa weight config [ĐÃ HOÀN THÀNH]

**Description:** Đưa weight của ranking vào constants hoặc config struct thay vì hard-code rải rác.

**Acceptance criteria:**
- [x] Weight có tên rõ nghĩa.
- [x] Có default config.
- [x] Có thể chỉnh weight mà không sửa query Cypher.

**Verification:**
- [x] Unit test scorer dùng default config.
- [x] `go test ./post-service/...`

**Files likely touched:**
- `post-service/service/newsfeed_scorer.go`
- `post-service/config/config.go` nếu cần env config

**Estimated scope:** S

### Checkpoint: Scoring Baseline

- [x] Query candidate production cho `RELEVANT` dễ đọc hơn, không còn công thức score dài trong Cypher.
- [x] Scorer có unit test.
- [x] API response không đổi.
- [x] Latency không tệ hơn đáng kể so với hiện tại.

### Phase 3: Observability và Tuning

#### Task 7: Thêm metrics/log cho ranking [ĐÃ HOÀN THÀNH]

**Description:** Ghi nhận thông tin đủ để debug vì sao bài được gợi ý: candidate count, filtered count, top score components, fallback, latency.

**Progress note:** Đã thêm profiler command cho candidate query, Go scoring, mark loaded và log metadata nhỏ (`strategy`, candidate count, organic count).

**Acceptance criteria:**
- [x] Profiler có command rõ cho candidate query và scoring.
- [x] Log debug không in payload nhạy cảm.
- [x] Có thể biết feed đang dùng strategy nào.

**Verification:**
- [x] `/profiler` thấy command scoring/candidate query.
- [x] `go test ./post-service/...`

**Files likely touched:**
- `post-service/service/post.go`
- `post-service/service/newsfeed_scorer.go`
- `profiler/` nếu cần metric helper mới

**Estimated scope:** S-M

#### Task 8: Thêm debug endpoint nội bộ cho score breakdown [ĐÃ HOÀN THÀNH]

**Description:** Tạo endpoint admin/debug để xem breakdown score của một user/feed trong môi trường dev hoặc admin-only.

**Acceptance criteria:**
- [x] Endpoint chỉ mở cho admin hoặc debug guard.
- [x] Trả danh sách candidate với score components.
- [x] Không lộ nội dung private không được phép xem.

**Verification:**
- [x] `go test ./api-gateway/... ./post-service/...`
- [x] Gateway handler test forward internal profiler token cho post-service debug guard.

**Files likely touched:**
- `post-service/handler/post_handler.go`
- `post-service/service/post.go`
- `api-gateway/router/router.go`

**Estimated scope:** M

### Checkpoint: Tuning Ready

- [x] Có score breakdown để chỉnh weight.
- [x] Có profiler/log đủ để so sánh latency.
- [x] Có test bảo vệ privacy/block.

### Phase 4: Optional Recommendation Service / ML Reranker

#### Task 9: Định nghĩa interface reranker

**Description:** Thêm interface nội bộ để post-service có thể gọi reranker tùy chọn sau khi đã có candidate list.

**Acceptance criteria:**
- [ ] Nếu reranker lỗi/timeout/empty result, fallback về rule-based order.
- [ ] Reranker không được tạo bài mới ngoài candidate list.
- [ ] Reranker không được bypass visibility filtering.

**Verification:**
- [ ] Unit test fallback.
- [ ] Unit test reranker trả thiếu/duplicate candidate.

**Files likely touched:**
- `post-service/service/newsfeed_reranker.go`
- `post-service/service/post.go`

**Estimated scope:** M

#### Task 10: Tách recommendation-service sau khi baseline ổn

**Description:** Nếu cần mở rộng, tạo service riêng chỉ nhận candidate features và trả lại ordered IDs. Không để service này quyết định quyền xem.

**Acceptance criteria:**
- [ ] Contract chỉ gồm candidate IDs/features, viewer context tối thiểu, trace ID.
- [ ] post-service vẫn là source of truth cho visibility.
- [ ] Có timeout ngắn và fallback.

**Verification:**
- [ ] Contract test.
- [ ] Integration test fallback khi recommendation-service down.

**Files likely touched:**
- `pb/` hoặc internal proto mới
- `post-service/service/`
- service mới nếu được quyết định

**Estimated scope:** L, cần plan riêng trước khi làm

## Test Plan

- Backend:
  - `go test ./post-service/...`
  - `go test ./api-gateway/...`
  - `go test ./ai-service`

- Manual:
  - Tạo user A/B/C.
  - A và B là bạn, A và C không là bạn.
  - B tạo bài `PUBLIC`, `FRIEND`, `PRIVATE`.
  - C không thấy `FRIEND`/`PRIVATE`, A thấy `FRIEND`.
  - Block A-B, A không còn thấy bài B.
  - Load `RELEVANT` nhiều lần, bài đã load giảm thứ hạng.
  - Like/comment/share bài có keyword, feed sau đó ưu tiên chủ đề liên quan hơn.

- Frontend:
  - `cd social-network-ui && npm run build`
  - Trang `/home` đổi filter `RELEVANT`, `TIME`, `FRIEND_ONLY` không lỗi và không duplicate rõ ràng.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Đổi query friend làm thay đổi feed hiện tại | Medium | Thêm test fixture và rollout theo phase 1 trước |
| Tách score khỏi Cypher làm tăng dữ liệu trả về | Medium | Giới hạn candidate window, benchmark latency |
| Keyword graph chưa đủ dữ liệu | Medium | Giữ recency/relationship/engagement là baseline mạnh |
| ML/reranker tạo lỗi privacy | High | Chỉ rerank candidate đã được post-service lọc quyền xem |
| Feed bị lặp do pagination + score thay đổi | Medium | Dùng loaded penalty, xem xét cursor/keyset pagination phase sau |

## Open Questions

- Có cần feed `RELEVANT` ưu tiên bài bạn bè hơn bài public trending không?
- `FRIEND_ONLY` có cần ranking theo engagement/keyword hay giữ newest-only?
- Có cần ẩn bài đã load hoàn toàn sau N lần, hay chỉ trừ điểm?
- Có cần lưu event impression/click riêng thay vì dùng `LOADED`?
- Có cần recommendation-service riêng trong repo này hay nên giữ trong `post-service` đến khi đủ dữ liệu?

## Proposed Order

1. Task 1: Chuẩn hóa friendship query.
2. Task 2: Làm rõ `PostsLoaded`.
3. Task 3: Test visibility/block.
4. Task 4-6: Tách candidate/scorer.
5. Task 7-8: Observability/debug.
6. Task 9-10: Reranker/recommendation-service nếu thật sự cần.
