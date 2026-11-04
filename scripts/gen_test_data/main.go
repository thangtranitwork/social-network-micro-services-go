package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Auth DB Models
type Account struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email      string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Password   string    `gorm:"type:varchar(255);not null"`
	Role       string    `gorm:"type:varchar(50);default:'USER';not null"`
	IsVerified bool      `gorm:"type:boolean;default:false;not null"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (Account) TableName() string { return "accounts" }

// User DB Models
type UserEntity struct {
	ID                      string         `gorm:"primaryKey;type:varchar(64)"`
	Email                   string         `gorm:"type:varchar(255);uniqueIndex"`
	GivenName               string         `gorm:"type:varchar(64)"`
	FamilyName              string         `gorm:"type:varchar(64)"`
	Username                string         `gorm:"type:varchar(32);uniqueIndex"`
	Bio                     string         `gorm:"type:text"`
	Birthdate               time.Time
	ProfilePictureID        string         `gorm:"type:varchar(255)"`
	EmailNotifications      bool           `gorm:"default:true"`
	PushNotifications       bool           `gorm:"default:true"`
	DigestFrequency         string         `gorm:"type:varchar(20);default:'DAILY'"`
	NextChangeNameDate      time.Time
	NextChangeBirthdateDate time.Time
	NextChangeUsernameDate  time.Time
	CreatedAt               time.Time      `gorm:"autoCreateTime"`
	UpdatedAt               time.Time      `gorm:"autoUpdateTime"`
	DeletedAt               gorm.DeletedAt `gorm:"index"`
}

func (UserEntity) TableName() string { return "users" }

type FriendEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	UserID    string    `gorm:"type:varchar(64);index:idx_user_friend,unique"`
	FriendID  string    `gorm:"type:varchar(64);index:idx_user_friend,unique"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (FriendEntity) TableName() string { return "friends" }

// Post DB Models
type PostEntity struct {
	ID           string         `gorm:"primaryKey;type:varchar(64)"`
	UserID       string         `gorm:"type:varchar(64);index:idx_post_user"`
	Content      string         `gorm:"type:text"`
	Privacy      string         `gorm:"type:varchar(20);default:'PUBLIC'"`
	LikeCount    int            `gorm:"default:0"`
	CommentCount int            `gorm:"default:0"`
	ShareCount   int            `gorm:"default:0"`
	CreatedAt    time.Time      `gorm:"autoCreateTime;index:idx_post_created"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (PostEntity) TableName() string { return "posts" }

type CommentEntity struct {
	ID        string         `gorm:"primaryKey;type:varchar(64)"`
	PostID    string         `gorm:"type:varchar(64);index:idx_comment_post"`
	UserID    string         `gorm:"type:varchar(64);index:idx_comment_user"`
	Content   string         `gorm:"type:text"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (CommentEntity) TableName() string { return "comments" }

type PostLikeEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	PostID    string    `gorm:"type:varchar(64);index:idx_post_like,unique"`
	UserID    string    `gorm:"type:varchar(64);index:idx_post_like,unique"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (PostLikeEntity) TableName() string { return "post_likes" }

// Story DB Models
type StoryEntity struct {
	ID        string         `gorm:"primaryKey;type:varchar(64)"`
	UserID    string         `gorm:"type:varchar(64);index:idx_story_user"`
	MediaURL  string         `gorm:"type:varchar(512)"`
	MediaType string         `gorm:"type:varchar(50);default:'IMAGE'"`
	Caption   string         `gorm:"type:text"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	ExpiresAt time.Time      `gorm:"index:idx_story_expires"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (StoryEntity) TableName() string { return "stories" }

type UserInfo struct {
	ID         string
	Email      string
	Username   string
	GivenName  string
	FamilyName string
	Bio        string
	Birthdate  time.Time
}

func ensureDatabaseExists(dsn, dbName string) {
	re := regexp.MustCompile(`dbname=([^\s]+)`)
	postgresDSN := re.ReplaceAllString(dsn, "dbname=postgres")
	sysDB, err := gorm.Open(postgres.Open(postgresDSN), &gorm.Config{})
	if err != nil {
		return
	}
	sqlInstance, _ := sysDB.DB()
	if sqlInstance != nil {
		defer sqlInstance.Close()
	}

	var count int64
	sysDB.Raw("SELECT count(*) FROM pg_database WHERE datname = ?", dbName).Scan(&count)
	if count == 0 {
		sysDB.Exec(fmt.Sprintf("CREATE DATABASE %s;", dbName))
		log.Printf("Created PostgreSQL Database: %s", dbName)
	}
}

func getDSNForDB(baseDSN, dbName string) string {
	re := regexp.MustCompile(`dbname=([^\s]+)`)
	return re.ReplaceAllString(baseDSN, "dbname="+dbName)
}

func main() {
	_ = godotenv.Load("auth-service/.env")
	_ = godotenv.Overload(".env")

	baseDSN := os.Getenv("POSTGRES_DSN")
	if baseDSN == "" {
		baseDSN = "host=localhost user=postgres password=postgres dbname=postgres port=15432 sslmode=disable"
	}
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "bolt://localhost:17687"
	}
	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		neo4jPass = "password"
	}

	log.Println("Starting comprehensive test data generation for PostgreSQL & Neo4j...")

	// 1. Ensure databases exist
	for _, dbName := range []string{"auth_db", "user_db", "post_db", "story_db"} {
		ensureDatabaseExists(baseDSN, dbName)
	}

	// Connect to GORM DBs
	authDB, err := gorm.Open(postgres.Open(getDSNForDB(baseDSN, "auth_db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to auth_db: %v", err)
	}
	userDB, err := gorm.Open(postgres.Open(getDSNForDB(baseDSN, "user_db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to user_db: %v", err)
	}
	postDB, err := gorm.Open(postgres.Open(getDSNForDB(baseDSN, "post_db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to post_db: %v", err)
	}
	storyDB, err := gorm.Open(postgres.Open(getDSNForDB(baseDSN, "story_db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to story_db: %v", err)
	}

	// Auto Migrate
	_ = authDB.AutoMigrate(&Account{}, &UserEntity{}, &FriendEntity{}, &PostEntity{}, &CommentEntity{}, &PostLikeEntity{}, &StoryEntity{})
	_ = userDB.AutoMigrate(&UserEntity{}, &FriendEntity{})
	_ = postDB.AutoMigrate(&PostEntity{}, &CommentEntity{}, &PostLikeEntity{})
	_ = storyDB.AutoMigrate(&StoryEntity{})

	// 2. Connect to Neo4j
	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
	if err != nil {
		log.Printf("Warning: Could not connect to Neo4j: %v", err)
	} else {
		defer driver.Close(ctx)
	}

	// Generate BCrypt hash for "123456Aa@"
	hashedPassBytes, err := bcrypt.GenerateFromPassword([]byte("123456Aa@"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}
	hashedPass := string(hashedPassBytes)

	// 3. Define 100 Test Accounts
	var allUsers []UserInfo

	// 10 Famous / Preserved Users (test-1 to test-10)
	famous := []struct {
		Email, Username, Given, Family, Bio, Birthdate string
	}{
		{"test-1@test.com", "obama", "Barack", "Obama", "44th President of the US. Community organizer, author, and father.", "1961-08-04"},
		{"test-2@test.com", "trump", "Donald", "Trump", "45th President of the US. Entrepreneur and real estate developer.", "1946-06-14"},
		{"test-3@test.com", "biden", "Joe", "Biden", "46th President of the US. Lifelong public servant.", "1942-11-20"},
		{"test-4@test.com", "putin", "Vladimir", "Putin", "President of Russia. Martial arts enthusiast.", "1952-10-07"},
		{"test-5@test.com", "xijinping", "Xi", "Jinping", "General Secretary and President. Focusing on modernization.", "1953-06-15"},
		{"test-6@test.com", "macron", "Emmanuel", "Macron", "President of France. European unity advocate.", "1977-12-21"},
		{"test-7@test.com", "zelenskyy", "Volodymyr", "Zelenskyy", "President of Ukraine. Standing for freedom and democracy.", "1978-01-25"},
		{"test-8@test.com", "hochiminh", "Ho", "Chi Minh", "Vietnamese revolutionary leader and national hero.", "1890-05-19"},
		{"test-9@test.com", "lincoln", "Abraham", "Lincoln", "16th President of the US. Preserved the Union and abolished slavery.", "1809-02-12"},
		{"test-10@test.com", "washington", "George", "Washington", "First President of the US. Father of his country.", "1732-02-22"},
	}

	for _, f := range famous {
		bDate, _ := time.Parse("2006-01-02", f.Birthdate)
		uID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(f.Email)).String()
		allUsers = append(allUsers, UserInfo{
			ID:         uID,
			Email:      f.Email,
			Username:   f.Username,
			GivenName:  f.Given,
			FamilyName: f.Family,
			Bio:        f.Bio,
			Birthdate:  bDate,
		})
	}

	// 90 Generic Test Users (test-11@test.com to test-100@test.com -> user_1 to user_90)
	firstNames := []string{"Alex", "John", "David", "Emily", "Michael", "Sarah", "Daniel", "Jessica", "James", "Sophia", "Nam", "Linh", "Huy", "Trang", "Minh", "Thu", "Tuan", "Mai", "Duc", "Hoa"}
	lastNames := []string{"Smith", "Johnson", "Brown", "Taylor", "Miller", "Wilson", "Anderson", "Thomas", "Nguyen", "Tran", "Le", "Pham", "Hoang", "Phan", "Vu", "Dang", "Bui", "Do", "Ho", "Ngo"}
	bios := []string{
		"Software engineer passionate about Go, distributed systems, and cloud architecture.",
		"Frontend enthusiast exploring React, Next.js, and web performance optimization.",
		"Coffee lover, tech geek, and full-stack developer building cool products.",
		"Data science hobbyist & backend learner. Constantly learning new technologies.",
		"Photography lover and traveler. Sharing moments and perspectives.",
		"Product designer crafting user-centric experiences and clean UI.",
		"DevOps engineer automating pipelines and managing Kubernetes clusters.",
		"Cybersecurity researcher and open source contributor.",
	}

	for i := 1; i <= 90; i++ {
		email := fmt.Sprintf("test-%d@test.com", i+10)
		username := fmt.Sprintf("user_%d", i)
		given := firstNames[(i-1)%len(firstNames)]
		family := lastNames[((i-1)*3)%len(lastNames)]
		bio := bios[(i-1)%len(bios)]
		bDate := time.Date(1990+(i%15), time.Month((i%12)+1), (i%28)+1, 0, 0, 0, 0, time.UTC)
		uID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(email)).String()

		allUsers = append(allUsers, UserInfo{
			ID:         uID,
			Email:      email,
			Username:   username,
			GivenName:  given,
			FamilyName: family,
			Bio:        bio,
			Birthdate:  bDate,
		})
	}

	log.Printf("Prepared %d test users + Admin account...", len(allUsers))

	// Clean previous test data to avoid unique constraint conflicts
	log.Println("Cleaning previous non-admin test data...")
	authDB.Unscoped().Where("email != ?", "admin@admin.com").Delete(&Account{})
	for _, db := range []*gorm.DB{authDB, userDB} {
		db.Unscoped().Where("email != ?", "admin@admin.com").Delete(&UserEntity{})
		db.Unscoped().Where("1 = 1").Delete(&FriendEntity{})
	}
	for _, db := range []*gorm.DB{authDB, postDB} {
		db.Unscoped().Where("1 = 1").Delete(&PostLikeEntity{})
		db.Unscoped().Where("1 = 1").Delete(&CommentEntity{})
		db.Unscoped().Where("1 = 1").Delete(&PostEntity{})
	}
	for _, db := range []*gorm.DB{authDB, storyDB} {
		db.Unscoped().Where("1 = 1").Delete(&StoryEntity{})
	}

	if driver != nil {
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			return tx.Run(ctx, "MATCH (n) DETACH DELETE n", nil)
		})
		session.Close(ctx)
	}

	// 4. Seed Admin Account
	adminUUID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	authDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"password", "role", "is_verified"}),
	}).Create(&Account{
		ID:         adminUUID,
		Email:      "admin@admin.com",
		Password:   hashedPass,
		Role:       "ADMIN",
		IsVerified: true,
		CreatedAt:  time.Now(),
	})

	for _, db := range []*gorm.DB{authDB, userDB} {
		db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"email", "given_name", "family_name", "username", "bio"}),
		}).Create(&UserEntity{
			ID:         "00000000-0000-0000-0000-000000000000",
			Email:      "admin@admin.com",
			GivenName:  "System",
			FamilyName: "Admin",
			Username:   "admin",
			Bio:        "System Administrator",
			CreatedAt:  time.Now(),
		})
	}

	// 5. Seed 100 Accounts & Users in Batch
	log.Println("Seeding 100 accounts into auth_db and user_db...")
	var accountsBatch []Account
	var usersBatch []UserEntity

	for _, u := range allUsers {
		uUUID, _ := uuid.Parse(u.ID)
		accountsBatch = append(accountsBatch, Account{
			ID:         uUUID,
			Email:      u.Email,
			Password:   hashedPass,
			Role:       "USER",
			IsVerified: true,
			CreatedAt:  time.Now().Add(-time.Duration(rand.Intn(60*24)) * time.Hour),
		})

		usersBatch = append(usersBatch, UserEntity{
			ID:         u.ID,
			Email:      u.Email,
			GivenName:  u.GivenName,
			FamilyName: u.FamilyName,
			Username:   u.Username,
			Bio:        u.Bio,
			Birthdate:  u.Birthdate,
			CreatedAt:  time.Now().Add(-time.Duration(rand.Intn(60*24)) * time.Hour),
		})
	}

	if err := authDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{"password", "role", "is_verified"}),
	}).CreateInBatches(accountsBatch, 50).Error; err != nil {
		log.Printf("Warning: error upserting accounts: %v", err)
	}

	for _, db := range []*gorm.DB{authDB, userDB} {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"email", "given_name", "family_name", "username", "bio", "deleted_at"}),
		}).CreateInBatches(usersBatch, 50).Error; err != nil {
			log.Printf("Warning: error upserting users: %v", err)
		}
	}

	// Seed Neo4j User Nodes
	if driver != nil {
		log.Println("Syncing users to Neo4j Graph...")
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			userMapList := make([]map[string]interface{}, len(allUsers))
			for i, u := range allUsers {
				userMapList[i] = map[string]interface{}{
					"id":         u.ID,
					"username":   u.Username,
					"email":      u.Email,
					"givenName":  u.GivenName,
					"familyName": u.FamilyName,
				}
			}
			query := `
				UNWIND $users AS u
				MERGE (n:User {id: u.id})
				SET n.username = u.username, n.email = u.email, n.givenName = u.givenName, n.familyName = u.familyName
			`
			return tx.Run(ctx, query, map[string]interface{}{"users": userMapList})
		})
		session.Close(ctx)
	}

	// 6. Generate Realistic Friendships (Social Graph)
	log.Println("Generating friendships...")
	r := rand.New(rand.NewSource(42))
	friendPairsMap := make(map[string]bool)
	var friendsBatch []FriendEntity
	var neo4jFriendList []map[string]interface{}

	for i := 0; i < len(allUsers); i++ {
		numFriends := r.Intn(5) + 3 // 3 to 7 friends per user
		for f := 0; f < numFriends; f++ {
			targetIdx := r.Intn(len(allUsers))
			if targetIdx == i {
				continue
			}

			u1 := allUsers[i].ID
			u2 := allUsers[targetIdx].ID
			pairKey := u1 + ":" + u2
			pairRev := u2 + ":" + u1

			if !friendPairsMap[pairKey] && !friendPairsMap[pairRev] {
				friendPairsMap[pairKey] = true
				friendPairsMap[pairRev] = true

				friendsBatch = append(friendsBatch,
					FriendEntity{UserID: u1, FriendID: u2, CreatedAt: time.Now()},
					FriendEntity{UserID: u2, FriendID: u1, CreatedAt: time.Now()},
				)

				neo4jFriendList = append(neo4jFriendList, map[string]interface{}{
					"u1": u1,
					"u2": u2,
				})
			}
		}
	}

	if len(friendsBatch) > 0 {
		for _, db := range []*gorm.DB{authDB, userDB} {
			_ = db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(friendsBatch, 100)
		}
	}

	if driver != nil && len(neo4jFriendList) > 0 {
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				UNWIND $pairs AS p
				MATCH (a:User {id: p.u1}), (b:User {id: p.u2})
				MERGE (a)-[:FRIEND]-(b)
			`
			return tx.Run(ctx, query, map[string]interface{}{"pairs": neo4jFriendList})
		})
		session.Close(ctx)
	}

	// 7. Generate 200 - 500 Posts with Likes and Comments
	log.Println("Generating 250 - 350 realistic posts with likes and comments...")
	postTemplates := []string{
		"Just finished setting up our new microservices cluster with Go and Docker! The latency dropped under 10ms.",
		"If you cannot explain it simply, you do not understand it well enough. Architecture review day!",
		"Clean architecture and domain-driven design make long-term maintenance an absolute breeze.",
		"What are your favorite features in Go? Goroutines and channels still blow my mind every single day.",
		"Graph databases like Neo4j make social recommendation queries so intuitive compared to recursive SQL joins.",
		"Coffee: turning caffeine into clean code and passing unit tests since day one ☕.",
		"Optimizing database indexes today cut our search query execution time by over 75%!",
		"Simplicity is prerequisite for reliability. Keep services lean and interfaces clear.",
		"Who is joining the upcoming hackathon this weekend? Let's build something extraordinary together 🚀.",
		"Refactoring legacy code is like spring cleaning. Challenging at first, but deeply satisfying at the end.",
		"Observability matters: full tracing, structured JSON logs, and real-time profilers save hours of debugging.",
		"Excited to share our newest feature rollout! Thank you everyone for the testing feedback.",
		"Beautiful morning to review pull requests and ship new features to production.",
		"Kafka event-driven architecture makes decoupled messaging so reliable across microservices.",
		"Always write unit tests as if the person maintaining them is a violent psychopath who knows where you live 😄.",
		"Just deployed the vector search model with Elasticsearch. Image search relevance is looking great!",
		"A small tip for Go developers: always pass and honor context.Context for network and database timeouts.",
		"Sunday project update: completed WebSocket realtime chat synchronization across clients.",
		"Continuous integration and continuous deployment: the greatest gift to software engineering productivity.",
		"First solve the problem, then write the code. Measure twice, cut once.",
		"Loving the new dark mode aesthetics on the portal. The contrast and typography look so crisp!",
		"Working on database partition strategies for high-throughput social feed feeds.",
		"Great coffee, reliable wifi, and a bug that has been haunting me for 3 days finally squashed!",
		"Elasticsearch 8 vector embeddings integration complete. Semantic keyword search is lightning fast now.",
		"Happy Friday everyone! Remember: no pushing straight to master before the weekend 🏖️.",
	}

	commentTemplates := []string{
		"Bài viết rất hay và bổ ích, cảm ơn bạn đã chia sẻ!",
		"Rất đồng quan điểm với bạn về vấn đề này.",
		"Great insights! Looking forward to testing this out.",
		"Tuyệt vời quá bạn ơi 👏",
		"Could you elaborate more on the implementation details?",
		"Hoàn toàn đồng ý, kiến trúc này xử lý scale rất mượt.",
		"Awesome post! Keep up the great work 🚀.",
		"Chuẩn không cần chỉnh luôn ạ!",
		"Interesting perspective, I learned something new today.",
		"Like mạnh bài viết này! Rất thực tế.",
		"100 điểm cho phần phân tích này!",
		"Quá xịn xò luôn đồng chí ơi!",
	}

	var allPosts []PostEntity
	var allLikes []PostLikeEntity
	var allComments []CommentEntity
	var neo4jPostList []map[string]interface{}

	postSeedRand := rand.New(rand.NewSource(1337))

	// Generate 300 posts (average 3 posts per user)
	for userIdx, u := range allUsers {
		numPostsForUser := postSeedRand.Intn(3) + 2 // 2 to 4 posts each = ~300 posts total

		for pIdx := 0; pIdx < numPostsForUser; pIdx++ {
			postID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s-post-%d", u.Username, pIdx))).String()
			tmpl := postTemplates[postSeedRand.Intn(len(postTemplates))]

			// Spread timestamps across the last 30 days
			hoursAgo := postSeedRand.Intn(30*24) + 1
			minsAgo := postSeedRand.Intn(60)
			postCreatedAt := time.Now().Add(-time.Duration(hoursAgo)*time.Hour - time.Duration(minsAgo)*time.Minute)

			privacy := "PUBLIC"
			if postSeedRand.Float64() < 0.15 {
				privacy = "FRIEND"
			}

			// Generate Likes for this post (3 to 20 likes)
			numLikes := postSeedRand.Intn(18) + 3
			likedUsers := make(map[int]bool)
			for l := 0; l < numLikes; l++ {
				likerIdx := postSeedRand.Intn(len(allUsers))
				if !likedUsers[likerIdx] {
					likedUsers[likerIdx] = true
					likeTime := postCreatedAt.Add(time.Duration(postSeedRand.Intn(120)+5) * time.Minute)
					if likeTime.After(time.Now()) {
						likeTime = time.Now()
					}
					allLikes = append(allLikes, PostLikeEntity{
						PostID:    postID,
						UserID:    allUsers[likerIdx].ID,
						CreatedAt: likeTime,
					})
				}
			}

			// Generate Comments for this post (1 to 5 comments)
			numComments := postSeedRand.Intn(5) + 1
			for c := 0; c < numComments; c++ {
				commenterIdx := postSeedRand.Intn(len(allUsers))
				commentID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s-comment-%d", postID, c))).String()
				cText := commentTemplates[postSeedRand.Intn(len(commentTemplates))]
				cTime := postCreatedAt.Add(time.Duration(postSeedRand.Intn(180)+10) * time.Minute)
				if cTime.After(time.Now()) {
					cTime = time.Now()
				}

				allComments = append(allComments, CommentEntity{
					ID:        commentID,
					PostID:    postID,
					UserID:    allUsers[commenterIdx].ID,
					Content:   cText,
					CreatedAt: cTime,
				})
			}

			postEntity := PostEntity{
				ID:           postID,
				UserID:       u.ID,
				Content:      tmpl,
				Privacy:      privacy,
				LikeCount:    len(likedUsers),
				CommentCount: numComments,
				ShareCount:   postSeedRand.Intn(5),
				CreatedAt:    postCreatedAt,
			}

			allPosts = append(allPosts, postEntity)

			neo4jPostList = append(neo4jPostList, map[string]interface{}{
				"id":        postID,
				"userId":    u.ID,
				"content":   tmpl,
				"privacy":   privacy,
				"createdAt": postCreatedAt.Format(time.RFC3339),
			})
		}

		_ = userIdx
	}

	log.Printf("Inserting %d posts into post_db and auth_db...", len(allPosts))
	for _, db := range []*gorm.DB{authDB, postDB} {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "privacy", "like_count", "comment_count", "share_count", "deleted_at"}),
		}).CreateInBatches(allPosts, 100).Error; err != nil {
			log.Printf("Warning: error upserting posts: %v", err)
		}

		if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(allLikes, 200).Error; err != nil {
			log.Printf("Warning: error inserting likes: %v", err)
		}

		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"content", "deleted_at"}),
		}).CreateInBatches(allComments, 100).Error; err != nil {
			log.Printf("Warning: error upserting comments: %v", err)
		}
	}

	// Sync Posts to Neo4j
	if driver != nil && len(neo4jPostList) > 0 {
		log.Println("Syncing posts to Neo4j Graph...")
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				UNWIND $posts AS p
				MATCH (u:User {id: p.userId})
				MERGE (post:Post {id: p.id})
				SET post.content = p.content, post.privacy = p.privacy, post.createdAt = datetime(p.createdAt)
				MERGE (u)-[:POSTED]->(post)
			`
			return tx.Run(ctx, query, map[string]interface{}{"posts": neo4jPostList})
		})
		session.Close(ctx)
	}

	// 8. Generate Active Stories (30-40 stories)
	log.Println("Generating active stories...")
	storyCaptions := []string{
		"Good morning from the tech hub! ☕",
		"Working on the new release today 🚀",
		"Sunday coding session 💻",
		"Beautiful sunset after a productive day 🌅",
		"Weekend vibes with the developer community!",
		"Coffee break ☕",
		"Debugging mode: ON 🔍",
	}

	var storiesBatch []StoryEntity
	for sIdx := 0; sIdx < 35; sIdx++ {
		storyUser := allUsers[sIdx%len(allUsers)]
		storyID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s-story-%d", storyUser.Username, sIdx))).String()
		caption := storyCaptions[sIdx%len(storyCaptions)]

		storiesBatch = append(storiesBatch, StoryEntity{
			ID:        storyID,
			UserID:    storyUser.ID,
			Caption:   caption,
			MediaType: "IMAGE",
			MediaURL:  "https://images.unsplash.com/photo-1517694712202-14dd9538aa97?w=800",
			CreatedAt: time.Now().Add(-time.Duration(sIdx*30) * time.Minute),
			ExpiresAt: time.Now().Add(24*time.Hour - time.Duration(sIdx*30)*time.Minute),
		})
	}

	for _, db := range []*gorm.DB{authDB, storyDB} {
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"caption", "expires_at", "deleted_at"}),
		}).CreateInBatches(storiesBatch, 50).Error; err != nil {
			log.Printf("Warning: error upserting stories: %v", err)
		}
	}

	log.Println("================================================================")
	log.Printf("🎉 Seed complete successfully!")
	log.Printf("• Accounts & Users: %d (100 test accounts + 1 Admin)", len(allUsers)+1)
	log.Printf("• Friendships:     %d friend pairs", len(friendsBatch)/2)
	log.Printf("• Posts:           %d posts with realistic timestamps (last 30 days)", len(allPosts))
	log.Printf("• Likes:           %d reactions", len(allLikes))
	log.Printf("• Comments:        %d user comments", len(allComments))
	log.Printf("• Stories:         %d active stories", len(storiesBatch))
	log.Println("All test accounts password: 123456Aa@")
	log.Println("Admin: admin@admin.com / 123456Aa@")
	log.Println("================================================================")
}
