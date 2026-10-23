package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExecuteWithRetry_SuccessFirstTry(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := executeWithRetry(ctx, 3, 10*time.Millisecond, func() error {
		attempts++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, attempts)
}

func TestExecuteWithRetry_SuccessAfterRetry(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := executeWithRetry(ctx, 3, 5*time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestExecuteWithRetry_ExhaustRetries(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := executeWithRetry(ctx, 3, 5*time.Millisecond, func() error {
		attempts++
		return errors.New("persistent error")
	})

	assert.Error(t, err)
	assert.Equal(t, "persistent error", err.Error())
	assert.Equal(t, 3, attempts)
}

func TestExecuteWithRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	// Cancel context after first failure
	err := executeWithRetry(ctx, 5, 50*time.Millisecond, func() error {
		attempts++
		cancel()
		return errors.New("network failure")
	})

	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, 1, attempts)
}
