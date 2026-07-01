package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Account struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email      string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Password   string    `gorm:"type:varchar(255);not null"`
	Role       string    `gorm:"type:varchar(50);default:'USER';not null"`
	IsVerified bool      `gorm:"type:boolean;default:false;not null"`
	CreatedAt  time.Time `gorm:"not null"`
}

func (Account) TableName() string {
	return "accounts"
}

type President struct {
	ID         string
	Email      string
	Username   string
	GivenName  string
	FamilyName string
	Bio        string
	Birthdate  string
}

func main() {
	// Load environment variables
	_ = godotenv.Load("auth-service/.env")
	_ = godotenv.Load("user-service/.env")

	pgDSN := os.Getenv("POSTGRES_DSN")
	if pgDSN == "" {
		pgDSN = "host=localhost user=postgres password=postgres dbname=auth_db port=5432 sslmode=disable"
	}
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "bolt://localhost:7687"
	}
	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		neo4jPass = "password"
	}

	// Guard against accidental execution in production / unauthorized environments
	allowDestructive := os.Getenv("ALLOW_DESTRUCTIVE_SEED") == "true"
	hasConfirmFlag := false
	for _, arg := range os.Args {
		if arg == "--confirm-reset-test-data" {
			hasConfirmFlag = true
			break
		}
	}

	if !allowDestructive && !hasConfirmFlag {
		// Check if interactive terminal
		fileInfo, err := os.Stdin.Stat()
		isTerminal := err == nil && (fileInfo.Mode()&os.ModeCharDevice) != 0

		if !isTerminal {
			log.Fatalf("FATAL: Non-interactive terminal detected. To execute this destructive seed script, you must either:\n"+
				"  1. Pass the CLI flag '--confirm-reset-test-data'\n"+
				"  2. Set the environment variable ALLOW_DESTRUCTIVE_SEED=true")
		}

		dbName := getDbName(pgDSN)
		fmt.Printf("\n"+
			"==============================================================\n"+
			"⚠️  WARNING: DESTRUCTIVE TEST DATA SEEDING SCRIPT\n"+
			"==============================================================\n"+
			"Target Postgres DSN: %s\n"+
			"Target Neo4j URI:    %s\n"+
			"--------------------------------------------------------------\n"+
			"This operation will:\n"+
			"  1. TRUNCATE/DELETE accounts matching test users and 'admin@admin.com'.\n"+
			"  2. DETACH DELETE Neo4j nodes (User, Post, Comment, etc.).\n"+
			"This will PERMANENTLY modify and overwrite database contents.\n"+
			"==============================================================\n"+
			"Are you sure you want to proceed?\n"+
			"Type the target database name '%s' to confirm: ", pgDSN, neo4jURI, dbName)

		var input string
		_, err = fmt.Scanln(&input)
		if err != nil || strings.TrimSpace(input) != dbName {
			log.Fatalf("FATAL: Seeding aborted. Confirmation input did not match '%s'.", dbName)
		}
		log.Println("Confirmation success. Proceeding with seeding...")
	}

	// 1. Connect to Postgres
	db, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	log.Println("Connected to Postgres successfully.")

	// Check if already seeded
	var count int64
	_ = db.AutoMigrate(&Account{})
	db.Model(&Account{}).Count(&count)
	if count > 0 && os.Getenv("FORCE_SEED") != "true" {
		log.Println("Database already contains accounts. Skipping seeding to prevent overwriting existing data. Set FORCE_SEED=true to force.")
		return
	}

	// 2. Connect to Neo4j
	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
	if err != nil {
		log.Fatalf("Failed to connect to Neo4j: %v", err)
	}
	defer driver.Close(context.Background())
	ctx := context.Background()
	log.Println("Connected to Neo4j successfully.")

	// Define core 10 users to match original preset IDs
	presidents := []President{
		{
			ID:         "54195e99-d47c-4a20-af50-2579c2912baf",
			Email:      "test-1@test.com",
			Username:   "obama",
			GivenName:  "Barack",
			FamilyName: "Obama",
			Bio:        "44th President of the United States. 'Yes we can!'",
			Birthdate:  "1961-08-04",
		},
		{
			ID:         "feba5708-7628-4078-8308-ab8d9095f240",
			Email:      "test-2@test.com",
			Username:   "trump",
			GivenName:  "Donald",
			FamilyName: "Trump",
			Bio:        "45th and 47th President of the United States. 'Make America Great Again!'",
			Birthdate:  "1946-06-14",
		},
		{
			ID:         "1b45fe05-8663-40e3-8fb3-3e9a0fac9e2e",
			Email:      "test-3@test.com",
			Username:   "biden",
			GivenName:  "Joe",
			FamilyName: "Biden",
			Bio:        "46th President of the United States. 'Build Back Better.'",
			Birthdate:  "1942-11-20",
		},
		{
			ID:         "c31a8d7e-2b8d-43b5-bd84-c73a91105c6a",
			Email:      "test-4@test.com",
			Username:   "putin",
			GivenName:  "Vladimir",
			FamilyName: "Putin",
			Bio:        "President of the Russian Federation.",
			Birthdate:  "1952-10-07",
		},
		{
			ID:         "cc808ce9-bb83-4299-ad41-1763947c1dc4",
			Email:      "test-5@test.com",
			Username:   "xijinping",
			GivenName:  "Jinping",
			FamilyName: "Xi",
			Bio:        "President of the People's Republic of China.",
			Birthdate:  "1953-06-15",
		},
		{
			ID:         "4db62232-aa37-4201-9874-34635c5e41f7",
			Email:      "test-6@test.com",
			Username:   "macron",
			GivenName:  "Emmanuel",
			FamilyName: "Macron",
			Bio:        "President of the French Republic.",
			Birthdate:  "1977-12-21",
		},
		{
			ID:         "759c8863-e11d-452c-9c6f-6fdce4cbf1c2",
			Email:      "test-7@test.com",
			Username:   "zelenskyy",
			GivenName:  "Volodymyr",
			FamilyName: "Zelenskyy",
			Bio:        "President of Ukraine.",
			Birthdate:  "1978-01-25",
		},
		{
			ID:         "8d29b578-1b32-4acd-a8d0-6a6d88e5a088",
			Email:      "test-8@test.com",
			Username:   "trudeau",
			GivenName:  "Justin",
			FamilyName: "Trudeau",
			Bio:        "Prime Minister of Canada.",
			Birthdate:  "1971-12-25",
		},
		{
			ID:         "97c6f23c-1003-43c1-9236-b1dd2a3592a1",
			Email:      "test-9@test.com",
			Username:   "sunak",
			GivenName:  "Rishi",
			FamilyName: "Sunak",
			Bio:        "Former Prime Minister of the United Kingdom.",
			Birthdate:  "1980-05-12",
		},
		{
			ID:         "217cf36c-6a4b-4102-88b7-b83508b78220",
			Email:      "test-10@test.com",
			Username:   "hochiminh",
			GivenName:  "Chi Minh",
			FamilyName: "Ho",
			Bio:        "First President of the Democratic Republic of Vietnam.",
			Birthdate:  "1890-05-19",
		},
	}

	// Add predefined famous personalities to enrich test data
	famousUsers := []struct {
		Username   string
		GivenName  string
		FamilyName string
		Bio        string
		Birthdate  string
	}{
		{"billgates", "Bill", "Gates", "Co-founder of Microsoft. Philanthropist.", "1955-10-28"},
		{"stevejobs", "Steve", "Jobs", "Co-founder of Apple. Stay hungry, stay foolish.", "1955-02-24"},
		{"elonmusk", "Elon", "Musk", "CEO of Tesla & SpaceX. Mars & Memes.", "1971-06-28"},
		{"zuck", "Mark", "Zuckerberg", "CEO of Meta. Connecting the world.", "1984-05-14"},
		{"bezos", "Jeff", "Bezos", "Founder of Amazon. To the moon and back.", "1964-01-12"},
		{"torvalds", "Linus", "Torvalds", "Creator of Linux and Git. Talk is cheap. Show me the code.", "1969-12-28"},
		{"altman", "Sam", "Altman", "CEO of OpenAI. Building safe AGI.", "1985-04-22"},
		{"pichai", "Sundar", "Pichai", "CEO of Google and Alphabet.", "1972-06-10"},

		{"einstein", "Albert", "Einstein", "Theoretical physicist. E=mc².", "1879-03-14"},
		{"newton", "Isaac", "Newton", "Mathematician and physicist. Gravity works.", "1643-01-04"},
		{"turing", "Alan", "Turing", "Father of modern computer science.", "1912-06-23"},
		{"curie", "Marie", "Curie", "Physicist and chemist. Pioneer in radioactivity.", "1867-11-07"},
		{"tesla", "Nikola", "Tesla", "Inventor and electrical engineer. Alternating current.", "1856-07-10"},
		{"socrates", "Socrates", "Socrates", "An unexamined life is not worth living.", "0470-01-01"},
		{"plato", "Plato", "Plato", "Philosopher and founder of the Academy.", "0427-01-01"},
		{"aristotle", "Aristotle", "Aristotle", "Philosopher and polymath. Student of Plato.", "0384-01-01"},
		{"davinci", "Leonardo", "da Vinci", "Renaissance polymath, painter of Mona Lisa.", "1452-04-15"},

		{"sherlock", "Sherlock", "Holmes", "Consulting detective. Elementary, my dear Watson.", "1854-01-06"},
		{"harrypotter", "Harry", "Potter", "The Boy Who Lived. Gryffindor.", "1980-07-31"},
		{"batman", "Bruce", "Wayne", "I am Batman. Gotham's protector.", "1939-05-27"},
		{"ironman", "Tony", "Stark", "Genius, billionaire, playboy, philanthropist.", "1970-05-29"},
		{"spiderman", "Peter", "Parker", "Your friendly neighborhood Spider-Man.", "2001-08-10"},
		{"luffy", "Monkey D.", "Luffy", "I'm gonna be the King of the Pirates!", "1997-05-05"},
		{"goku", "Son", "Goku", "Saiyan raised on Earth. Let's fight!", "1984-04-16"},
	}

	for _, u := range famousUsers {
		deterministicID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(u.Username)).String()
		presidents = append(presidents, President{
			ID:         deterministicID,
			Email:      fmt.Sprintf("%s@example.com", u.Username),
			Username:   u.Username,
			GivenName:  u.GivenName,
			FamilyName: u.FamilyName,
			Bio:        u.Bio,
			Birthdate:  u.Birthdate,
		})
	}

	// Generate additional 90 generic test users programmatically
	for i := 1; i <= 90; i++ {
		username := fmt.Sprintf("user_%d", i)
		email := fmt.Sprintf("test-%d@test.com", i+10) // offset to prevent overlapping test-1..10
		deterministicID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(username)).String()
		presidents = append(presidents, President{
			ID:         deterministicID,
			Email:      email,
			Username:   username,
			GivenName:  "Test",
			FamilyName: fmt.Sprintf("User %d", i),
			Bio:        fmt.Sprintf("Hello! I am test user #%d, here to help scale the graph database and test feed performance.", i),
			Birthdate:  fmt.Sprintf("199%d-0%d-15", i%10, (i%9)+1),
		})
	}

	hashedPassword := "$2a$10$nY7QXq4yxG7XuWRMHB1aQeqyPD.MERuNHSllKgu8nMRGdjNwOx7YS"

	// 3. Clear existing test accounts & Neo4j nodes in bulk
	log.Println("Cleaning existing data...")

	// Postgres Bulk Delete
	emails := make([]string, len(presidents))
	for i, p := range presidents {
		emails[i] = p.Email
	}
	emails = append(emails, "admin@admin.com")
	if err := db.Where("email IN ?", emails).Delete(&Account{}).Error; err != nil {
		log.Printf("Warning: Failed to clean Postgres accounts: %v", err)
	}

	// Neo4j Bulk Delete (Users, their Posts, and Comments on those posts / comments they posted)
	ids := make([]string, len(presidents))
	for i, p := range presidents {
		ids[i] = p.ID
	}
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User)
			WHERE u.id IN $ids
			OPTIONAL MATCH (u)-[:POSTED]->(p:Post)
			OPTIONAL MATCH (p)<-[:COMMENT_OF]-(c1:Comment)
			OPTIONAL MATCH (u)-[:COMMENTED]->(c2:Comment)
			DETACH DELETE c1, c2, p, u
		`
		return tx.Run(ctx, query, map[string]interface{}{"ids": ids})
	})
	session.Close(ctx)
	if err != nil {
		log.Printf("Warning: Failed to clean Neo4j data: %v", err)
	}

	// 4. Create Postgres Accounts in batches
	log.Println("Inserting Postgres accounts...")
	accounts := make([]Account, len(presidents))
	for i, p := range presidents {
		uID, _ := uuid.Parse(p.ID)
		accounts[i] = Account{
			ID:         uID,
			Email:      p.Email,
			Password:   hashedPassword,
			Role:       "USER",
			IsVerified: true,
			CreatedAt:  time.Now(),
		}
	}

	// Add Admin Account
	adminID, _ := uuid.Parse("04ba77c9-e602-4fca-9325-338cd40a750e")
	accounts = append(accounts, Account{
		ID:         adminID,
		Email:      "admin@admin.com",
		Password:   "$2a$10$QcfP7.Nbs1z6QbJ49msqvOIozQ2415XDroj/9kDtnZsyQLPMu0NA2", // BCrypt for 123456Aa@
		Role:       "ADMIN",
		IsVerified: true,
		CreatedAt:  time.Now(),
	})

	if err := db.CreateInBatches(accounts, 50).Error; err != nil {
		log.Fatalf("Failed to batch create accounts: %v", err)
	}

	// 5. Create Neo4j Users in bulk
	log.Println("Inserting Neo4j users...")
	session = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		userList := make([]map[string]interface{}, len(presidents))
		for i, p := range presidents {
			userList[i] = map[string]interface{}{
				"id":         p.ID,
				"username":   p.Username,
				"email":      p.Email,
				"givenName":  p.GivenName,
				"familyName": p.FamilyName,
				"bio":        p.Bio,
				"birthdate":  p.Birthdate,
			}
		}

		query := `
			UNWIND $users AS user
			CREATE (u:User {
				id: user.id,
				username: user.username,
				email: user.email,
				givenName: user.givenName,
				familyName: user.familyName,
				bio: user.bio,
				birthdate: user.birthdate,
				createdAt: datetime(),
				friendCount: 0,
				nextChangeNameDate: datetime(),
				nextChangeBirthdateDate: datetime(),
				nextChangeUsernameDate: datetime()
			})
		`
		_, err = tx.Run(ctx, query, map[string]interface{}{"users": userList})
		return nil, err
	})
	session.Close(ctx)
	if err != nil {
		log.Fatalf("Failed to create Neo4j User nodes: %v", err)
	}

	// 6. Establish Friendships
	log.Println("Establishing friendships...")
	friendships := [][2]string{
		{"obama", "biden"},
		{"obama", "trudeau"},
		{"obama", "macron"},
		{"trump", "putin"},
		{"putin", "xijinping"},
		{"zelenskyy", "biden"},
		{"zelenskyy", "macron"},
		{"zelenskyy", "sunak"},
		{"zelenskyy", "trudeau"},
	}

	// Build a map of friendships to avoid duplicates
	friendPairs := make(map[string]bool)
	for _, f := range friendships {
		u1, u2 := f[0], f[1]
		if u1 > u2 {
			u1, u2 = u2, u1
		}
		friendPairs[u1+"-"+u2] = true
	}

	// Generate additional random friendships deterministically
	friendRand := rand.New(rand.NewSource(42))
	numRandomFriendships := 250
	for i := 0; i < numRandomFriendships; i++ {
		idx1 := friendRand.Intn(len(presidents))
		idx2 := friendRand.Intn(len(presidents))
		if idx1 == idx2 {
			continue
		}
		u1 := presidents[idx1].Username
		u2 := presidents[idx2].Username
		if u1 > u2 {
			u1, u2 = u2, u1
		}
		pairKey := u1 + "-" + u2
		friendPairs[pairKey] = true
	}

	// Convert friendship map to a list of maps for Cypher
	friendshipList := make([]map[string]interface{}, 0, len(friendPairs))
	for pairKey := range friendPairs {
		parts := strings.Split(pairKey, "-")
		friendshipList = append(friendshipList, map[string]interface{}{
			"u1":   parts[0],
			"u2":   parts[1],
			"uuid": uuid.NewSHA1(uuid.NameSpaceDNS, []byte(pairKey)).String(),
		})
	}

	session = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			UNWIND $friendships AS f
			MATCH (u1:User {username: f.u1}), (u2:User {username: f.u2})
			MERGE (u1)-[r:FRIEND]-(u2)
			ON CREATE SET r.uuid = f.uuid, r.createdAt = datetime()
			SET u1.friendCount = coalesce(u1.friendCount, 0) + 1
			SET u2.friendCount = coalesce(u2.friendCount, 0) + 1
		`
		_, err = tx.Run(ctx, query, map[string]interface{}{"friendships": friendshipList})
		return nil, err
	})
	session.Close(ctx)
	if err != nil {
		log.Printf("Warning: Failed to establish friendships: %v", err)
	}

	// 7. Create Posts
	log.Println("Creating posts...")

	type Post struct {
		ID        string
		Author    string
		Content   string
		Privacy   string
		CreatedAt string
	}

	corePosts := []struct {
		ID      string
		Author  string
		Content string
		Privacy string
	}{
		{
			ID:      "8f8fb4a8-6f6a-4d2c-85a2-c1c5b8b9f0b1",
			Author:  "obama",
			Content: "Change will not come if we wait for some other person or some other time. We are the ones we've been waiting for. We are the change that we seek.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "a3d34b3e-5284-48f8-8a88-2947a111b151",
			Author:  "trump",
			Content: "Big news today! We are going to make our country greater and stronger than ever before! MAGA!",
			Privacy: "PUBLIC",
		},
		{
			ID:      "92a4de8e-1729-43c2-bf77-18f9df0123ef",
			Author:  "biden",
			Content: "Folks, today we took another historic step forward in rebuilding our middle class and our infrastructure. Let's keep moving.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "d19fba80-c11a-4d92-805d-ea753b811234",
			Author:  "putin",
			Content: "The multipolar world order is becoming a reality, based on sovereignty and international law.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "b8912ef0-cc99-4c12-9851-bc7e201b125f",
			Author:  "xijinping",
			Content: "Peace, development, and win-win cooperation remain the themes of our times.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "e8bca012-5b91-4202-a1b2-1a4db4a8e0f1",
			Author:  "macron",
			Content: "Ensemble, nous devons relever les défis de notre temps. L'Europe doit être forte et souveraine.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "c789dfb8-c1ab-432d-8de1-c11fdfab5678",
			Author:  "zelenskyy",
			Content: "Thank you to all our international partners for standing with Ukraine in our fight for freedom and democracy.",
			Privacy: "PUBLIC",
		},
		{
			ID:      "f8e12a4b-9d8c-4a3b-a2e1-c8b9d0e1f2a3",
			Author:  "hochiminh",
			Content: "Không có gì quý hơn độc lập, tự do.",
			Privacy: "PUBLIC",
		},
	}

	var allPosts []Post

	// Add core posts
	for _, p := range corePosts {
		allPosts = append(allPosts, Post{
			ID:        p.ID,
			Author:    p.Author,
			Content:   p.Content,
			Privacy:   p.Privacy,
			CreatedAt: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		})
	}

	// Templates for generating additional posts
	postTemplates := []string{
		"Just finished setting up a new Kubernetes cluster. The orchestration is beautiful!",
		"If you cannot explain it simply, you do not understand it well enough.",
		"Code is like humor. When you have to explain it, it’s bad.",
		"What are your favorite features in Go 1.21? The new built-in min and max functions are handy.",
		"The best way to predict the future is to invent it.",
		"Simplicity is the soul of efficiency.",
		"Coffee: because coding without caffeine is just typing.",
		"Does anyone else love writing raw SQL queries more than using heavy ORMs?",
		"Graph databases are so underrated. Neo4j makes relationship queries so simple.",
		"Remember that a clean codebase is a happy codebase.",
		"First, solve the problem. Then, write the code.",
		"The only way to do great work is to love what you do.",
		"Hello world! This is my first post on this amazing new social network.",
		"Beautiful day to write some code and build some cool features.",
		"Refactoring legacy code is like cleaning a messy room. Exhausting but so satisfying.",
		"Currently learning WebRTC for video call signaling. Super interesting technology!",
		"Who is up for a hackathon this weekend? Let's build something awesome.",
		"Optimizing database indexes today. Cut query execution time by 80%!",
		"Don't worry if it doesn't work right. If everything did, you'd be out of a job.",
		"Make it work, make it right, make it fast.",
	}

	// Generate posts deterministically
	postRand := rand.New(rand.NewSource(99))
	for i, p := range presidents {
		// Determine number of posts for this user: 0, 1, or 2
		numPosts := postRand.Intn(3)
		for j := 0; j < numPosts; j++ {
			templateIdx := postRand.Intn(len(postTemplates))
			content := postTemplates[templateIdx]

			// Slightly customize content to make it unique per user
			content = fmt.Sprintf("%s #%d", content, i*10+j)

			postID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s-post-%d", p.Username, j))).String()

			privacy := "PUBLIC"
			if postRand.Float64() < 0.2 {
				privacy = "FRIEND"
			} else if postRand.Float64() < 0.05 {
				privacy = "PRIVATE"
			}

			// Spread posts over the last 5 days
			hoursAgo := (i*7 + j*13) % 120
			postTime := time.Now().Add(-time.Duration(hoursAgo) * time.Hour)

			allPosts = append(allPosts, Post{
				ID:        postID,
				Author:    p.Username,
				Content:   content,
				Privacy:   privacy,
				CreatedAt: postTime.Format(time.RFC3339),
			})
		}
	}

	session = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		postList := make([]map[string]interface{}, len(allPosts))
		for i, p := range allPosts {
			postList[i] = map[string]interface{}{
				"id":        p.ID,
				"author":    p.Author,
				"content":   p.Content,
				"privacy":   p.Privacy,
				"createdAt": p.CreatedAt,
			}
		}

		query := `
			UNWIND $posts AS p
			MATCH (u:User {username: p.author})
			CREATE (post:Post {
				id: p.id,
				content: p.content,
				privacy: p.privacy,
				files: [],
				likeCount: 0,
				commentCount: 0,
				shareCount: 0,
				createdAt: datetime(p.createdAt),
				updatedAt: null,
				deletedAt: null
			})
			CREATE (u)-[:POSTED]->(post)
		`
		_, err = tx.Run(ctx, query, map[string]interface{}{"posts": postList})
		return nil, err
	})
	session.Close(ctx)
	if err != nil {
		log.Fatalf("Failed to create Posts: %v", err)
	}

	// 8. Add Comments & Likes
	log.Println("Adding comments & likes...")

	trumpPostID := "a3d34b3e-5284-48f8-8a88-2947a111b151"
	bidenPostID := "92a4de8e-1729-43c2-bf77-18f9df0123ef"
	zelenskyyPostID := "c789dfb8-c1ab-432d-8de1-c11fdfab5678"
	hochiminhPostID := "f8e12a4b-9d8c-4a3b-a2e1-c8b9d0e1f2a3"

	type Comment struct {
		CommentID string
		PostID    string
		Author    string
		Content   string
		CreatedAt string
	}

	type Like struct {
		PostID string
		Liker  string
	}

	// Core comments
	allComments := []Comment{
		{uuid.NewString(), trumpPostID, "putin", "A strong leader is always respected.", time.Now().Add(-50 * time.Minute).Format(time.RFC3339)},
		{uuid.NewString(), trumpPostID, "biden", "We need unity, not division, Donald.", time.Now().Add(-45 * time.Minute).Format(time.RFC3339)},
		{uuid.NewString(), bidenPostID, "obama", "Proud of the work you're doing, Joe!", time.Now().Add(-40 * time.Minute).Format(time.RFC3339)},
		{uuid.NewString(), zelenskyyPostID, "biden", "We stand with you, Volodymyr.", time.Now().Add(-35 * time.Minute).Format(time.RFC3339)},
		{uuid.NewString(), zelenskyyPostID, "macron", "La France est à vos côtés.", time.Now().Add(-30 * time.Minute).Format(time.RFC3339)},
		{uuid.NewString(), hochiminhPostID, "obama", "An inspiring quote from a legendary leader.", time.Now().Add(-25 * time.Minute).Format(time.RFC3339)},
	}

	// Core likes
	allLikes := []Like{
		{trumpPostID, "putin"},
		{bidenPostID, "obama"},
		{bidenPostID, "macron"},
		{zelenskyyPostID, "biden"},
		{zelenskyyPostID, "trudeau"},
		{hochiminhPostID, "obama"},
		{hochiminhPostID, "biden"},
	}

	commentTemplates := []string{
		"Interesting perspective!",
		"Fully agree with this.",
		"Could you explain this in more detail?",
		"Wow, this is neat!",
		"Thanks for sharing, very helpful.",
		"I was just thinking about this yesterday.",
		"Is this public yet?",
		"Amazing work!",
		"Nice one!",
		"Indeed, clean code is key.",
		"Let's catch up and discuss this.",
		"Exactly my thoughts.",
	}

	likeMap := make(map[string]bool)
	for _, l := range allLikes {
		likeMap[l.PostID+"-"+l.Liker] = true
	}

	// Generate additional likes & comments deterministically
	commentRand := rand.New(rand.NewSource(100))
	for _, p := range allPosts {
		// 1. Likes
		numLikes := commentRand.Intn(10) // 0 to 9 additional likes
		for k := 0; k < numLikes; k++ {
			likerIdx := commentRand.Intn(len(presidents))
			liker := presidents[likerIdx].Username
			if liker == p.Author {
				continue // don't self-like
			}
			key := p.ID + "-" + liker
			if !likeMap[key] {
				likeMap[key] = true
				allLikes = append(allLikes, Like{
					PostID: p.ID,
					Liker:  liker,
				})
			}
		}

		// 2. Comments
		numComments := commentRand.Intn(4) // 0 to 3 additional comments
		for k := 0; k < numComments; k++ {
			commenterIdx := commentRand.Intn(len(presidents))
			commenter := presidents[commenterIdx].Username
			templateIdx := commentRand.Intn(len(commentTemplates))
			content := commentTemplates[templateIdx]

			commentID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(fmt.Sprintf("%s-comment-%s-%d", p.ID, commenter, k))).String()

			// Slightly backdate comments relative to now
			commentTime := time.Now().Add(-time.Duration(commentRand.Intn(120)) * time.Minute)

			allComments = append(allComments, Comment{
				CommentID: commentID,
				PostID:    p.ID,
				Author:    commenter,
				Content:   content,
				CreatedAt: commentTime.Format(time.RFC3339),
			})
		}
	}

	// Write Comments in bulk
	session = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		commentList := make([]map[string]interface{}, len(allComments))
		for i, c := range allComments {
			commentList[i] = map[string]interface{}{
				"commentID": c.CommentID,
				"postID":    c.PostID,
				"author":    c.Author,
				"content":   c.Content,
				"createdAt": c.CreatedAt,
			}
		}

		query := `
			UNWIND $comments AS c
			MATCH (author:User {username: c.author}), (p:Post {id: c.postID})
			CREATE (cmt:Comment {
				id: c.commentID,
				content: c.content,
				file: "",
				likeCount: 0,
				replyCount: 0,
				createdAt: datetime(c.createdAt),
				updatedAt: null
			})
			CREATE (author)-[:COMMENTED]->(cmt)
			CREATE (cmt)-[:COMMENT_OF]->(p)
			SET p.commentCount = coalesce(p.commentCount, 0) + 1
		`
		_, err = tx.Run(ctx, query, map[string]interface{}{"comments": commentList})
		return nil, err
	})
	session.Close(ctx)
	if err != nil {
		log.Printf("Warning: Failed to add comments in bulk: %v", err)
	}

	// Write Likes in bulk
	session = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		likeList := make([]map[string]interface{}, len(allLikes))
		for i, l := range allLikes {
			likeList[i] = map[string]interface{}{
				"postID": l.PostID,
				"liker":  l.Liker,
			}
		}

		query := `
			UNWIND $likes AS l
			MATCH (liker:User {username: l.liker}), (p:Post {id: l.postID})
			MERGE (liker)-[:LIKED]->(p)
			SET p.likeCount = coalesce(p.likeCount, 0) + 1
		`
		_, err = tx.Run(ctx, query, map[string]interface{}{"likes": likeList})
		return nil, err
	})
	session.Close(ctx)
	if err != nil {
		log.Printf("Warning: Failed to add likes in bulk: %v", err)
	}

	log.Println("Data generation complete!")
}

func getDbName(dsn string) string {
	parts := strings.Fields(dsn)
	for _, p := range parts {
		if strings.HasPrefix(p, "dbname=") {
			return strings.TrimPrefix(p, "dbname=")
		}
	}
	return "auth_db"
}
