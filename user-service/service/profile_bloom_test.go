package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"social-network-go/internal/bloom"
	"social-network-go/user-service/model"
	"social-network-go/user-service/repository"
)

type spyUserRepo struct {
	repository.UserRepository
	getProfileCalls int
}

func (s *spyUserRepo) GetUserProfile(_ context.Context, _ string, _ string) (*model.User, error) {
	s.getProfileCalls++
	return nil, ErrUserNotFound
}

func TestGetUserProfile_BloomShortCircuit(t *testing.T) {
	ctx := context.Background()
	idBloom := bloom.NewMemoryBloomFilter(bloom.Config{Capacity: 1000, ErrorRate: 0.01})
	usernameBloom := bloom.NewMemoryBloomFilter(bloom.Config{Capacity: 1000, ErrorRate: 0.01})

	existingID := uuid.NewString()
	require.NoError(t, idBloom.Add(ctx, existingID))
	require.NoError(t, idBloom.MarkReady(ctx))

	require.NoError(t, usernameBloom.Add(ctx, "alice"))
	require.NoError(t, usernameBloom.MarkReady(ctx))

	repo := &spyUserRepo{}
	svc := &UserService{
		UserRepo:      repo,
		UserIDBloom:   idBloom,
		UsernameBloom: usernameBloom,
	}

	// Missing UUID -> short-circuit
	_, err := svc.GetUserProfile(ctx, uuid.NewString(), "viewer-1")
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Equal(t, 0, repo.getProfileCalls)

	// Missing username -> short-circuit
	_, err = svc.GetUserProfile(ctx, "unknown_user", "viewer-1")
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Equal(t, 0, repo.getProfileCalls)

	// Existing username (case-insensitive) -> hits Repo
	_, err = svc.GetUserProfile(ctx, "Alice", "viewer-1")
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Equal(t, 1, repo.getProfileCalls)

	// Existing UUID -> hits Repo
	_, err = svc.GetUserProfile(ctx, existingID, "viewer-1")
	require.ErrorIs(t, err, ErrUserNotFound)
	require.Equal(t, 2, repo.getProfileCalls)
}
