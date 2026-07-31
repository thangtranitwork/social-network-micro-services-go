package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	ID                      string    `gorm:"primaryKey;type:varchar(64)"`
	Email                   string    `gorm:"type:varchar(255);uniqueIndex"`
	GivenName               string    `gorm:"type:varchar(64)"`
	FamilyName              string    `gorm:"type:varchar(64)"`
	Username                string    `gorm:"type:varchar(32);uniqueIndex"`
	Bio                     string    `gorm:"type:text"`
	Birthdate               time.Time
	ProfilePictureID        string    `gorm:"type:varchar(255)"`
	EmailNotifications      bool      `gorm:"default:true"`
	PushNotifications       bool      `gorm:"default:true"`
	DigestFrequency         string    `gorm:"type:varchar(20);default:'DAILY'"`
	NextChangeNameDate      time.Time
	NextChangeBirthdateDate time.Time
	NextChangeUsernameDate  time.Time
	CreatedAt               time.Time `gorm:"autoCreateTime"`
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
	ID           string    `gorm:"primaryKey;type:varchar(64)"`
	UserID       string    `gorm:"type:varchar(64);index:idx_post_user"`
	Content      string    `gorm:"type:text"`
	Privacy      string    `gorm:"type:varchar(20);default:'PUBLIC'"`
	LikeCount    int       `gorm:"default:0"`
	CommentCount int       `gorm:"default:0"`
	ShareCount   int       `gorm:"default:0"`
	CreatedAt    time.Time `gorm:"autoCreateTime;index:idx_post_created"`
}

func (PostEntity) TableName() string { return "posts" }

type CommentEntity struct {
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	PostID    string    `gorm:"type:varchar(64);index:idx_comment_post"`
	UserID    string    `gorm:"type:varchar(64);index:idx_comment_user"`
	Content   string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
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
	ID        string    `gorm:"primaryKey;type:varchar(64)"`
	UserID    string    `gorm:"type:varchar(64);index:idx_story_user"`
	MediaURL  string    `gorm:"type:varchar(512)"`
	MediaType string    `gorm:"type:varchar(50);default:'IMAGE'"`
	Caption   string    `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	ExpiresAt time.Time
}

func (StoryEntity) TableName() string { return "stories" }

type President struct {
	ID         string
	Email      string
	Username   string
	GivenName  string
	FamilyName string
	Bio        string
	Birthdate  string
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
	_ = godotenv.Load(".env")

	baseDSN := os.Getenv("POSTGRES_DSN")
	if baseDSN == "" {
		baseDSN = "host=localhost user=postgres password=postgres dbname=postgres port=5432 sslmode=disable"
	}
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "neo4j://localhost:7687"
	}
	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		neo4jPass = "password"
	}

	log.Println("Starting test data generation for PostgreSQL & Neo4j...")

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
	_ = authDB.AutoMigrate(&Account{})
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

	// 3. Define Seed Users
	presidents := []President{
		{ID: "00000000-0000-0000-0000-000000000001", Email: "washington@us.gov", Username: "washington", GivenName: "George", FamilyName: "Washington", Bio: "First President of the US", Birthdate: "1732-02-22"},
		{ID: "00000000-0000-0000-0000-000000000002", Email: "adams@us.gov", Username: "adams", GivenName: "John", FamilyName: "Adams", Bio: "Second President of the US", Birthdate: "1735-10-30"},
		{ID: "00000000-0000-0000-0000-000000000003", Email: "jefferson@us.gov", Username: "jefferson", GivenName: "Thomas", FamilyName: "Jefferson", Bio: "Principal author of Declaration of Independence", Birthdate: "1743-04-13"},
		{ID: "00000000-0000-0000-0000-000000000004", Email: "lincoln@us.gov", Username: "lincoln", GivenName: "Abraham", FamilyName: "Lincoln", Bio: "Preserved the Union and abolished slavery", Birthdate: "1809-02-12"},
		{ID: "00000000-0000-0000-0000-000000000005", Email: "roosevelt@us.gov", Username: "roosevelt", GivenName: "Theodore", FamilyName: "Roosevelt", Bio: "Speak softly and carry a big stick", Birthdate: "1858-10-27"},
	}

	// Password: "password" hashed bcrypt
	hashedPass := "$2a$10$wE8w9E7M3a0aO3tq77i12.Gz9E2rYgY69X/vC6a71e7Z3n3n3n3n3"

	for _, p := range presidents {
		uID, _ := uuid.Parse(p.ID)
		bTime, _ := time.Parse("2006-01-02", p.Birthdate)

		// Seed Auth DB
		authDB.Where(Account{ID: uID}).FirstOrCreate(&Account{
			ID:         uID,
			Email:      p.Email,
			Password:   hashedPass,
			Role:       "USER",
			IsVerified: true,
			CreatedAt:  time.Now(),
		})

		// Seed User DB
		userDB.Where(UserEntity{ID: p.ID}).FirstOrCreate(&UserEntity{
			ID:         p.ID,
			Email:      p.Email,
			GivenName:  p.GivenName,
			FamilyName: p.FamilyName,
			Username:   p.Username,
			Bio:        p.Bio,
			Birthdate:  bTime,
			CreatedAt:  time.Now(),
		})

		// Seed Neo4j Node
		if driver != nil {
			session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
			_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
				return tx.Run(ctx, "MERGE (u:User {id: $id}) SET u.username = $username", map[string]interface{}{
					"id":       p.ID,
					"username": p.Username,
				})
			})
			session.Close(ctx)
		}
	}

	// Seed Friendships in SQL and Neo4j
	friendPairs := [][2]string{
		{presidents[0].ID, presidents[1].ID},
		{presidents[0].ID, presidents[2].ID},
		{presidents[1].ID, presidents[2].ID},
		{presidents[2].ID, presidents[3].ID},
		{presidents[3].ID, presidents[4].ID},
	}

	for _, pair := range friendPairs {
		userDB.Create(&FriendEntity{UserID: pair[0], FriendID: pair[1], CreatedAt: time.Now()})
		userDB.Create(&FriendEntity{UserID: pair[1], FriendID: pair[0], CreatedAt: time.Now()})

		if driver != nil {
			session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
			_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
				return tx.Run(ctx, "MATCH (a:User {id: $u1}), (b:User {id: $u2}) MERGE (a)-[:FRIEND]-(b)", map[string]interface{}{
					"u1": pair[0],
					"u2": pair[1],
				})
			})
			session.Close(ctx)
		}
	}

	// Seed Sample Posts
	postContents := []string{
		"Liberty, when it begins to take root, is a plant of rapid growth.",
		"Real integrity is doing the right thing, knowing that nobody is going to know whether you did it or not.",
		"Honesty is the first chapter in the book of wisdom.",
		"In the end, it is not the years in your life that count. It is the life in your years.",
		"Believe you can and you are halfway there.",
	}

	for i, p := range presidents {
		postID := fmt.Sprintf("00000000-0000-0000-0000-00000000001%d", i+1)
		post := PostEntity{
			ID:        postID,
			UserID:    p.ID,
			Content:   postContents[i],
			Privacy:   "PUBLIC",
			CreatedAt: time.Now().Add(-time.Duration(i*3) * time.Hour),
		}
		postDB.Where(PostEntity{ID: postID}).FirstOrCreate(&post)

		// Seed Story
		storyID := fmt.Sprintf("00000000-0000-0000-0000-00000000002%d", i+1)
		storyDB.Where(StoryEntity{ID: storyID}).FirstOrCreate(&StoryEntity{
			ID:        storyID,
			UserID:    p.ID,
			Caption:   "Day in the life of " + p.GivenName,
			MediaType: "IMAGE",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})

		// Seed Neo4j Post Node
		if driver != nil {
			session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
			_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
				return tx.Run(ctx, "MATCH (u:User {id: $uID}) MERGE (p:Post {id: $pID}) ON CREATE SET p.createdAt = datetime() MERGE (u)-[:POSTED]->(p)", map[string]interface{}{
					"uID": p.ID,
					"pID": postID,
				})
			})
			session.Close(ctx)
		}
	}

	log.Println("Test data generation complete for all PostgreSQL databases and Neo4j Graph!")
}
