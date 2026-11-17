package bloom

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryBloomFilter_AccuracyAndFalsePositiveRate(t *testing.T) {
	ctx := context.Background()
	f := NewMemoryBloomFilter(Config{
		Capacity:  10000,
		ErrorRate: 0.01,
	})

	// Before MarkReady, MightExist must fail-open (return true)
	require.True(t, MightExist(ctx, f, "missing@example.com"))

	items := make([]string, 5000)
	for i := range items {
		items[i] = fmt.Sprintf("user-%d@example.com", i)
	}
	require.NoError(t, f.AddBatch(ctx, items))
	require.NoError(t, f.MarkReady(ctx))

	// Zero false negatives
	for _, it := range items {
		exists, err := f.Exists(ctx, it)
		require.NoError(t, err)
		require.True(t, exists, "expected %s to exist", it)
		require.True(t, MightExist(ctx, f, it))
	}

	// False positive rate on unseen items should stay well below 2% (target 1%)
	probes := 5000
	probeItems := make([]string, probes)
	for i := range probeItems {
		probeItems[i] = fmt.Sprintf("absent-%d@example.com", i)
	}
	results, err := f.ExistsBatch(ctx, probeItems)
	require.NoError(t, err)
	require.Len(t, results, probes)

	fp := 0
	for _, hit := range results {
		if hit {
			fp++
		}
	}
	fpRate := float64(fp) / float64(probes)
	require.Less(t, fpRate, 0.02, "false positive rate too high: %v", fpRate)
}

func TestMemoryBloomFilter_Concurrency(t *testing.T) {
	ctx := context.Background()
	f := NewMemoryBloomFilter(Config{Capacity: 5000, ErrorRate: 0.01})
	require.NoError(t, f.MarkReady(ctx))

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := fmt.Sprintf("w%d-%d", worker, i)
				_ = f.Add(ctx, key)
				_, _ = f.Exists(ctx, key)
			}
		}(g)
	}
	wg.Wait()
}

type failingFilter struct {
	ready    bool
	readyErr error
	exists   bool
	existErr error
}

func (f failingFilter) Reserve(context.Context) error                { return nil }
func (f failingFilter) Add(context.Context, string) error            { return nil }
func (f failingFilter) AddBatch(context.Context, []string) error     { return nil }
func (f failingFilter) Exists(context.Context, string) (bool, error) { return f.exists, f.existErr }
func (f failingFilter) ExistsBatch(context.Context, []string) ([]bool, error) {
	return nil, nil
}
func (f failingFilter) MarkReady(context.Context) error { return nil }
func (f failingFilter) IsReady(context.Context) (bool, error) {
	return f.ready, f.readyErr
}

func TestMightExist_FailOpen(t *testing.T) {
	ctx := context.Background()
	require.True(t, MightExist(ctx, nil, "abc"))
	require.True(t, MightExist(ctx, failingFilter{ready: false, exists: false}, "abc"))
	require.True(t, MightExist(ctx, failingFilter{readyErr: errors.New("redis down")}, "abc"))
	require.True(t, MightExist(ctx, failingFilter{ready: true, existErr: errors.New("timeout")}, "abc"))
	require.False(t, MightExist(ctx, failingFilter{ready: true, exists: false}, "abc"))
	require.True(t, MightExist(ctx, failingFilter{ready: true, exists: true}, "abc"))
}
