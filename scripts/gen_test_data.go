package main

import (
	"context"
	"log"
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

	pgDSN := "host=localhost user=postgres password=postgres dbname=auth_db port=5432 sslmode=disable"
	neo4jURI := "bolt://localhost:7687"
	neo4jUser := "neo4j"
	neo4jPass := "password"

	// 1. Connect to Postgres
	db, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	log.Println("Connected to Postgres successfully.")

	// 2. Connect to Neo4j
	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
	if err != nil {
		log.Fatalf("Failed to connect to Neo4j: %v", err)
	}
	defer driver.Close(context.Background())
	ctx := context.Background()
	log.Println("Connected to Neo4j successfully.")

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
			ID:         "8d29b578-1b32-4acd-a8d0-6a6d88e5a088", // unique test-8 id
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

	hashedPassword := "$2a$10$nY7QXq4yxG7XuWRMHB1aQeqyPD.MERuNHSllKgu8nMRGdjNwOx7YS"

	// 3. Clear existing test accounts & Neo4j nodes
	log.Println("Cleaning existing data...")
	for _, p := range presidents {
		// Postgres
		db.Where("email = ?", p.Email).Delete(&Account{})

		// Neo4j - delete User, their Posts, and Comments
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Detach delete user, their comments, and posts
			query := `
				MATCH (u:User {id: $id})
				OPTIONAL MATCH (u)-[:POSTED]->(p:Post)
				OPTIONAL MATCH (p)<-[:COMMENT_OF]-(c:Comment)
				DETACH DELETE c, p, u
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": p.ID})
		})
		session.Close(ctx)
		if err != nil {
			log.Printf("Warning: Failed to clean Neo4j data for %s: %v", p.Username, err)
		}
	}

	// 4. Create Postgres Accounts
	log.Println("Inserting Postgres accounts...")
	for _, p := range presidents {
		uID, _ := uuid.Parse(p.ID)
		acc := Account{
			ID:         uID,
			Email:      p.Email,
			Password:   hashedPassword,
			Role:       "USER",
			IsVerified: true,
			CreatedAt:  time.Now(),
		}
		if err := db.Create(&acc).Error; err != nil {
			log.Fatalf("Failed to create account %s: %v", p.Email, err)
		}
	}

	// 5. Create Neo4j Users
	log.Println("Inserting Neo4j users...")
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, p := range presidents {
			query := `
				CREATE (u:User {
					id: $id,
					username: $username,
					email: $email,
					givenName: $givenName,
					familyName: $familyName,
					bio: $bio,
					birthdate: $birthdate,
					createdAt: datetime(),
					friendCount: 0,
					nextChangeNameDate: datetime(),
					nextChangeBirthdateDate: datetime(),
					nextChangeUsernameDate: datetime()
				})
			`
			_, err = tx.Run(ctx, query, map[string]interface{}{
				"id":         p.ID,
				"username":   p.Username,
				"email":      p.Email,
				"givenName":  p.GivenName,
				"familyName": p.FamilyName,
				"bio":        p.Bio,
				"birthdate":  p.Birthdate,
			})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
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
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, f := range friendships {
			query := `
				MATCH (u1:User {username: $u1}), (u2:User {username: $u2})
				MERGE (u1)-[:FRIEND]-(u2)
				SET u1.friendCount = coalesce(u1.friendCount, 0) + 1
				SET u2.friendCount = coalesce(u2.friendCount, 0) + 1
			`
			_, err = tx.Run(ctx, query, map[string]interface{}{"u1": f[0], "u2": f[1]})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		log.Printf("Warning: Failed to establish friendships: %v", err)
	}

	// 7. Create Posts
	log.Println("Creating posts...")
	posts := []struct {
		ID      string
		Author  string
		Content string
		Privacy string
	}{
		{
			ID:      uuid.NewString(),
			Author:  "obama",
			Content: "Change will not come if we wait for some other person or some other time. We are the ones we've been waiting for. We are the change that we seek.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "trump",
			Content: "Big news today! We are going to make our country greater and stronger than ever before! MAGA!",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "biden",
			Content: "Folks, today we took another historic step forward in rebuilding our middle class and our infrastructure. Let's keep moving.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "putin",
			Content: "The multipolar world order is becoming a reality, based on sovereignty and international law.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "xijinping",
			Content: "Peace, development, and win-win cooperation remain the themes of our times.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "macron",
			Content: "Ensemble, nous devons relever les défis de notre temps. L'Europe doit être forte et souveraine.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "zelenskyy",
			Content: "Thank you to all our international partners for standing with Ukraine in our fight for freedom and democracy.",
			Privacy: "PUBLIC",
		},
		{
			ID:      uuid.NewString(),
			Author:  "hochiminh",
			Content: "Không có gì quý hơn độc lập, tự do.",
			Privacy: "PUBLIC",
		},
	}

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, p := range posts {
			query := `
				MATCH (u:User {username: $author})
				CREATE (post:Post {
					id: $id,
					content: $content,
					privacy: $privacy,
					files: [],
					likeCount: 0,
					commentCount: 0,
					shareCount: 0,
					createdAt: datetime(),
					updatedAt: null,
					deletedAt: null
				})
				CREATE (u)-[:POSTED]->(post)
			`
			_, err = tx.Run(ctx, query, map[string]interface{}{
				"id":      p.ID,
				"author":  p.Author,
				"content": p.Content,
				"privacy": p.Privacy,
			})
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		log.Fatalf("Failed to create Posts: %v", err)
	}

	// 8. Add Comments & Likes
	log.Println("Adding comments & likes...")
	trumpPostID := ""
	bidenPostID := ""
	zelenskyyPostID := ""
	hochiminhPostID := ""

	for _, p := range posts {
		if p.Author == "trump" {
			trumpPostID = p.ID
		} else if p.Author == "biden" {
			bidenPostID = p.ID
		} else if p.Author == "zelenskyy" {
			zelenskyyPostID = p.ID
		} else if p.Author == "hochiminh" {
			hochiminhPostID = p.ID
		}
	}

	comments := []struct {
		CommentID string
		PostID    string
		Author    string
		Content   string
	}{
		{uuid.NewString(), trumpPostID, "putin", "A strong leader is always respected."},
		{uuid.NewString(), trumpPostID, "biden", "We need unity, not division, Donald."},
		{uuid.NewString(), bidenPostID, "obama", "Proud of the work you're doing, Joe!"},
		{uuid.NewString(), zelenskyyPostID, "biden", "We stand with you, Volodymyr."},
		{uuid.NewString(), zelenskyyPostID, "macron", "La France est à vos côtés."},
		{uuid.NewString(), hochiminhPostID, "obama", "An inspiring quote from a legendary leader."},
	}

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, c := range comments {
			query := `
				MATCH (author:User {username: $author}), (p:Post {id: $postID})
				CREATE (cmt:Comment {
					id: $commentID,
					content: $content,
					file: "",
					likeCount: 0,
					replyCount: 0,
					createdAt: datetime(),
					updatedAt: null
				})
				CREATE (author)-[:COMMENTED]->(cmt)
				CREATE (cmt)-[:COMMENT_OF]->(p)
				SET p.commentCount = coalesce(p.commentCount, 0) + 1
			`
			_, err = tx.Run(ctx, query, map[string]interface{}{
				"commentID": c.CommentID,
				"postID":    c.PostID,
				"author":    c.Author,
				"content":   c.Content,
			})
			if err != nil {
				return nil, err
			}
		}

		// Add some likes
		likes := []struct {
			PostID string
			Liker  string
		}{
			{trumpPostID, "putin"},
			{bidenPostID, "obama"},
			{bidenPostID, "macron"},
			{zelenskyyPostID, "biden"},
			{zelenskyyPostID, "trudeau"},
			{hochiminhPostID, "obama"},
			{hochiminhPostID, "biden"},
		}
		for _, l := range likes {
			query := `
				MATCH (liker:User {username: $liker}), (p:Post {id: $postID})
				MERGE (liker)-[:LIKED]->(p)
				SET p.likeCount = coalesce(p.likeCount, 0) + 1
			`
			_, err = tx.Run(ctx, query, map[string]interface{}{
				"liker":  l.Liker,
				"postID": l.PostID,
			})
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
	})
	if err != nil {
		log.Printf("Warning: Failed to add comments and likes: %v", err)
	}

	log.Println("Data generation complete!")
}
