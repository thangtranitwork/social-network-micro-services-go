package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"social-network-go/internal/bloom"
)

func TestMemoryBlacklistBloom_PreFilterBehavior(t *testing.T) {
	ctx := context.Background()
	bf := bloom.NewMemoryBloomFilter(bloom.Config{Capacity: 1000, ErrorRate: 0.001})

	// Before MarkReady, MightExist fails open (returns true so Redis is checked).
	require.True(t, bloom.MightExist(ctx, bf, "valid-token"))

	require.NoError(t, bf.Add(ctx, "revoked-token"))
	require.NoError(t, bf.MarkReady(ctx))

	require.False(t, bloom.MightExist(ctx, bf, "valid-token"))
	require.True(t, bloom.MightExist(ctx, bf, "revoked-token"))
}
