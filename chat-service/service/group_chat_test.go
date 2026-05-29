package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/chat-service/config"
	"social-network-go/chat-service/db"
)

func TestGroupChat(t *testing.T) {
	// 1. Load Configurations
	cfg := config.LoadConfig()
	cfg.Neo4jURI = "neo4j://localhost:7687"
	cfg.Neo4jUser = "neo4j"
	cfg.Neo4jPass = "password"
	cfg.MongoURI = "mongodb://localhost:27017/chat_db"

	// Initialize DBs
	db.InitDB(cfg)
	db.InitNeo4j(cfg)

	if db.Neo4jDriver == nil {
		t.Skip("Neo4j driver is nil, skipping integration test")
	}

	ctx := context.Background()

	// 2. Setup mock user nodes in Neo4j
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	setupQuery := `
		MERGE (u1:User {id: "user_admin", username: "admin", givenName: "Admin", familyName: "User"})
		MERGE (u2:User {id: "user_member1", username: "member1", givenName: "Member", familyName: "One"})
		MERGE (u3:User {id: "user_member2", username: "member2", givenName: "Member", familyName: "Two"})
		RETURN u1.id
	`
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, setupQuery, nil)
	})
	if err != nil {
		t.Fatalf("Failed to setup mock user nodes: %v", err)
	}

	svc := NewChatService(cfg)

	// 3. Create Group Chat
	t.Run("CreateGroupChat", func(t *testing.T) {
		chatID, err := svc.CreateGroupChat(ctx, "user_admin", "Test Group", []string{"user_member1"})
		if err != nil {
			t.Fatalf("Failed to create group chat: %v", err)
		}
		if chatID == "" {
			t.Fatal("Expected non-empty chatID")
		}

		// Verify membership
		if !svc.IsMemberOfChat("user_admin", chatID) {
			t.Error("Admin should be a member of the group chat")
		}
		if !svc.IsMemberOfChat("user_member1", chatID) {
			t.Error("Member1 should be a member of the group chat")
		}
		if svc.IsMemberOfChat("user_member2", chatID) {
			t.Error("Member2 should NOT be a member of the group chat yet")
		}

		// Get Chat List and verify
		rooms := svc.GetChatList("user_admin")
		found := false
		for _, room := range rooms {
			if room.ChatID == chatID {
				found = true
				if !room.IsGroup {
					t.Error("Expected IsGroup to be true")
				}
				if room.Name != "Test Group" {
					t.Errorf("Expected group name 'Test Group', got '%s'", room.Name)
				}
				if room.AdminID != "user_admin" {
					t.Errorf("Expected admin ID 'user_admin', got '%s'", room.AdminID)
				}
				if room.Target != nil {
					t.Error("Expected Target to be nil for group chat")
				}
			}
		}
		if !found {
			t.Error("Expected to find the newly created group chat in GetChatList")
		}

		// 4. Add Members to Group
		t.Run("AddMembersToGroup", func(t *testing.T) {
			err := svc.AddMembersToGroup(ctx, "user_admin", chatID, []string{"user_member2"})
			if err != nil {
				t.Fatalf("Failed to add member to group: %v", err)
			}
			if !svc.IsMemberOfChat("user_member2", chatID) {
				t.Error("Member2 should now be a member of the group chat")
			}
		})

		// 5. Update Group Chat
		t.Run("UpdateGroupChat", func(t *testing.T) {
			err := svc.UpdateGroupChat(ctx, "user_admin", chatID, "Updated Group Name", "new_avatar_id")
			if err != nil {
				t.Fatalf("Failed to update group chat: %v", err)
			}
			rooms := svc.GetChatList("user_admin")
			found := false
			for _, room := range rooms {
				if room.ChatID == chatID {
					found = true
					if room.Name != "Updated Group Name" {
						t.Errorf("Expected group name 'Updated Group Name', got '%s'", room.Name)
					}
					if room.Avatar != fmt.Sprintf("%s/new_avatar_id", cfg.FileServiceURL) {
						t.Errorf("Expected group avatar to be enriched, got '%s'", room.Avatar)
					}
				}
			}
			if !found {
				t.Error("Expected to find the updated group chat in GetChatList")
			}
		})

		// 6. Remove Member From Group
		t.Run("RemoveMemberFromGroup", func(t *testing.T) {
			err := svc.RemoveMemberFromGroup(ctx, "user_admin", chatID, "user_member1")
			if err != nil {
				t.Fatalf("Failed to remove member from group: %v", err)
			}
			if svc.IsMemberOfChat("user_member1", chatID) {
				t.Error("Member1 should NOT be a member of the group chat anymore")
			}
		})

		// 7. Cleanup
		cleanupQuery := `
			MATCH (c:Chat {id: $chatId})
			DETACH DELETE c
		`
		_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			return tx.Run(ctx, cleanupQuery, map[string]interface{}{"chatId": chatID})
		})
		if err != nil {
			t.Logf("Failed to cleanup test group chat: %v", err)
		}
	})

	// Cleanup test users
	cleanupUsersQuery := `
		MATCH (u:User) WHERE u.id IN ["user_admin", "user_member1", "user_member2"]
		DETACH DELETE u
	`
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, cleanupUsersQuery, nil)
	})
}
