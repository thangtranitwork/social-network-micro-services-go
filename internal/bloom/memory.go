package bloom

import (
	"context"
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"
)

// MemoryBloomFilter is a thread-safe in-memory Bloom filter backed by a uint64 bitset.
// Useful for local pre-filtering (e.g., token blacklist) and deterministic unit testing.
type MemoryBloomFilter struct {
	mu    sync.RWMutex
	bits  []uint64
	m     uint64
	k     uint64
	ready atomic.Bool
}

// NewMemoryBloomFilter creates an in-memory Bloom filter sized for cfg.Capacity and cfg.ErrorRate.
func NewMemoryBloomFilter(cfg Config) *MemoryBloomFilter {
	cfg = cfg.withDefaults()
	m, k := optimalParams(cfg.Capacity, cfg.ErrorRate)
	words := (m + 63) / 64
	return &MemoryBloomFilter{
		bits: make([]uint64, words),
		m:    m,
		k:    k,
	}
}

func optimalParams(n uint64, p float64) (uint64, uint64) {
	ln2 := math.Ln2
	mFloat := -float64(n) * math.Log(p) / (ln2 * ln2)
	m := uint64(math.Ceil(mFloat))
	if m < 64 {
		m = 64
	}
	kFloat := (float64(m) / float64(n)) * ln2
	k := uint64(math.Round(kFloat))
	if k < 1 {
		k = 1
	}
	return m, k
}

func (f *MemoryBloomFilter) Reserve(_ context.Context) error {
	return nil
}

func (f *MemoryBloomFilter) Add(_ context.Context, item string) error {
	if item == "" {
		return nil
	}
	h1, h2 := hashItem(item)
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := uint64(0); i < f.k; i++ {
		idx := (h1 + i*h2) % f.m
		f.bits[idx/64] |= 1 << (idx % 64)
	}
	return nil
}

func (f *MemoryBloomFilter) AddBatch(ctx context.Context, items []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range items {
		if item == "" {
			continue
		}
		h1, h2 := hashItem(item)
		for i := uint64(0); i < f.k; i++ {
			idx := (h1 + i*h2) % f.m
			f.bits[idx/64] |= 1 << (idx % 64)
		}
	}
	return nil
}

func (f *MemoryBloomFilter) Exists(_ context.Context, item string) (bool, error) {
	if item == "" {
		return false, nil
	}
	h1, h2 := hashItem(item)
	f.mu.RLock()
	defer f.mu.RUnlock()
	for i := uint64(0); i < f.k; i++ {
		idx := (h1 + i*h2) % f.m
		if (f.bits[idx/64] & (1 << (idx % 64))) == 0 {
			return false, nil
		}
	}
	return true, nil
}

func (f *MemoryBloomFilter) ExistsBatch(_ context.Context, items []string) ([]bool, error) {
	out := make([]bool, len(items))
	f.mu.RLock()
	defer f.mu.RUnlock()
	for idxItem, item := range items {
		if item == "" {
			continue
		}
		h1, h2 := hashItem(item)
		found := true
		for i := uint64(0); i < f.k; i++ {
			idx := (h1 + i*h2) % f.m
			if (f.bits[idx/64] & (1 << (idx % 64))) == 0 {
				found = false
				break
			}
		}
		out[idxItem] = found
	}
	return out, nil
}

func (f *MemoryBloomFilter) MarkReady(_ context.Context) error {
	f.ready.Store(true)
	return nil
}

func (f *MemoryBloomFilter) IsReady(_ context.Context) (bool, error) {
	return f.ready.Load(), nil
}

// Reset clears the bitset and ready state (useful for periodic blacklist rotation).
func (f *MemoryBloomFilter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.bits)
	f.ready.Store(false)
}

func hashItem(item string) (uint64, uint64) {
	h1 := fnv.New64a()
	_, _ = h1.Write([]byte(item))
	sum1 := h1.Sum64()

	h2 := fnv.New64()
	_, _ = h2.Write([]byte(item))
	_, _ = h2.Write([]byte{0x9e, 0x37, 0x79, 0xb9})
	sum2 := h2.Sum64()
	if sum2 == 0 {
		sum2 = 1
	}
	return sum1, sum2
}
