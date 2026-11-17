package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"social-network-go/internal/bloom"
)

const batchSize = 1000

func main() {
	redisAddr := flag.String("redis-addr", envOrDefault("REDIS_ADDR", "localhost:16379"), "Redis address")
	redisPass := flag.String("redis-pass", os.Getenv("REDIS_PASSWORD"), "Redis password")
	authDSN := flag.String("auth-dsn", envOrDefault("AUTH_POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=auth_db port=15432 sslmode=disable"), "Auth PostgreSQL DSN")
	userDSN := flag.String("user-dsn", envOrDefault("USER_POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=user_db port=15432 sslmode=disable"), "User PostgreSQL DSN")
	postDSN := flag.String("post-dsn", envOrDefault("POST_POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=post_db port=15432 sslmode=disable"), "Post PostgreSQL DSN")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{
		Addr:     *redisAddr,
		Password: *redisPass,
		DB:       0,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping failed (%s): %v", *redisAddr, err)
	}

	if *authDSN != "" {
		if err := warmupEmails(ctx, rdb, *authDSN); err != nil {
			log.Printf("WARN: warmup accounts.email skipped/failed: %v", err)
		}
	}
	if *userDSN != "" {
		if err := warmupUsers(ctx, rdb, *userDSN); err != nil {
			log.Printf("WARN: warmup users skipped/failed: %v", err)
		}
	}
	if *postDSN != "" {
		if err := warmupPosts(ctx, rdb, *postDSN); err != nil {
			log.Printf("WARN: warmup posts skipped/failed: %v", err)
		}
	}

	log.Println("Bloom filter warmup completed.")
}

func warmupEmails(ctx context.Context, rdb *redis.Client, dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	f := bloom.NewRedisBloomFilter(rdb, bloom.Config{Key: "bf:accounts:email", ErrorRate: 0.001, Capacity: 100000})
	if err := f.Reserve(ctx); err != nil {
		return err
	}

	type row struct{ Email string }
	total := 0
	offset := 0
	for {
		var rows []row
		if err := db.WithContext(ctx).Table("accounts").Select("email").Limit(batchSize).Offset(offset).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		items := make([]string, 0, len(rows))
		for _, r := range rows {
			if norm := bloom.NormalizeEmail(r.Email); norm != "" {
				items = append(items, norm)
			}
		}
		if err := f.AddBatch(ctx, items); err != nil {
			return err
		}
		total += len(items)
		offset += len(rows)
	}
	if err := f.MarkReady(ctx); err != nil {
		return err
	}
	log.Printf("Warmed up bf:accounts:email (%d items)", total)
	return nil
}

func warmupUsers(ctx context.Context, rdb *redis.Client, dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	idFilter := bloom.NewRedisBloomFilter(rdb, bloom.Config{Key: "bf:users:id", ErrorRate: 0.001, Capacity: 100000})
	usernameFilter := bloom.NewRedisBloomFilter(rdb, bloom.Config{Key: "bf:users:username", ErrorRate: 0.001, Capacity: 100000})
	if err := idFilter.Reserve(ctx); err != nil {
		return err
	}
	if err := usernameFilter.Reserve(ctx); err != nil {
		return err
	}

	type row struct {
		ID       string
		Username string
	}
	totalIDs, totalUsernames := 0, 0
	offset := 0
	for {
		var rows []row
		if err := db.WithContext(ctx).Table("users").Select("id, username").Where("deleted_at IS NULL").Limit(batchSize).Offset(offset).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]string, 0, len(rows))
		usernames := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.ID != "" {
				ids = append(ids, r.ID)
			}
			if norm := bloom.NormalizeUsername(r.Username); norm != "" {
				usernames = append(usernames, norm)
			}
		}
		if err := idFilter.AddBatch(ctx, ids); err != nil {
			return err
		}
		if err := usernameFilter.AddBatch(ctx, usernames); err != nil {
			return err
		}
		totalIDs += len(ids)
		totalUsernames += len(usernames)
		offset += len(rows)
	}
	if err := idFilter.MarkReady(ctx); err != nil {
		return err
	}
	if err := usernameFilter.MarkReady(ctx); err != nil {
		return err
	}
	log.Printf("Warmed up bf:users:id (%d items) and bf:users:username (%d items)", totalIDs, totalUsernames)
	return nil
}

func warmupPosts(ctx context.Context, rdb *redis.Client, dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	f := bloom.NewRedisBloomFilter(rdb, bloom.Config{Key: "bf:posts:id", ErrorRate: 0.001, Capacity: 500000})
	if err := f.Reserve(ctx); err != nil {
		return err
	}

	type row struct{ ID string }
	total := 0
	offset := 0
	for {
		var rows []row
		if err := db.WithContext(ctx).Table("posts").Select("id").Where("deleted_at IS NULL").Limit(batchSize).Offset(offset).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		items := make([]string, 0, len(rows))
		for _, r := range rows {
			if r.ID != "" {
				items = append(items, r.ID)
			}
		}
		if err := f.AddBatch(ctx, items); err != nil {
			return err
		}
		total += len(items)
		offset += len(rows)
	}
	if err := f.MarkReady(ctx); err != nil {
		return err
	}
	log.Printf("Warmed up bf:posts:id (%d items)", total)
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
