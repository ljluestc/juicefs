package fs

import (
	"os"
	"sync"
	"syscall"
	"testing"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

type mockMeta struct{}

func (m *mockMeta) Stat(ino uint64) (meta.InodeInfo, error) { return meta.InodeInfo{}, nil }

// TestLockDeadlockDetection simulates a deadlock and verifies EDEADLK.
func TestLockDeadlockDetection(t *testing.T) {
	// Create a temporary file
	f, err := os.CreateTemp("", "juicefs-lock-test")
	assert.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	// Initialize two LockManagers (simulating two processes)
	lm1, err := NewLockManager(&mockMeta{}, f.Name())
	assert.NoError(t, err)
	defer lm1.Close()

	lm2, err := NewLockManager(&mockMeta{}, f.Name())
	assert.NoError(t, err)
	defer lm2.Close()

	// Simulate deadlock: Process 1 locks byte 100, Process 2 locks byte 200
	var wg sync.WaitGroup
	wg.Add(2)
	deadlockDetected := false

	// Process 1
	go func() {
		defer wg.Done()
		// Lock byte 100
		err := lm1.Lock(100, 1, unix.F_WRLCK, true)
		assert.NoError(t, err)

		// Try to lock byte 200 (should block, then deadlock)
		err = lm1.Lock(200, 1, unix.F_WRLCK, true)
		if err == syscall.EDEADLK {
			deadlockDetected = true
		}
		assert.Equal(t, syscall.EDEADLK, err, "Expected deadlock detection")
	}()

	// Process 2
	go func() {
		defer wg.Done()
		// Lock byte 200
		err := lm2.Lock(200, 1, unix.F_WRLCK, true)
		assert.NoError(t, err)

		// Try to lock byte 100 (triggers deadlock)
		err = lm2.Lock(100, 1, unix.F_WRLCK, true)
		if err == syscall.EDEADLK {
			deadlockDetected = true
		}
		assert.Equal(t, syscall.EDEADLK, err, "Expected deadlock detection")
	}()

	wg.Wait()
	assert.True(t, deadlockDetected, "Deadlock should be detected in at least one process")
}

// TestLockNonBlocking verifies non-blocking behavior.
func TestLockNonBlocking(t *testing.T) {
	f, err := os.CreateTemp("", "juicefs-lock-test")
	assert.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	lm, err := NewLockManager(&mockMeta{}, f.Name())
	assert.NoError(t, err)
	defer lm.Close()

	// Acquire lock
	err = lm.Lock(0, 1, unix.F_WRLCK, false)
	assert.NoError(t, err)

	// Try to acquire same lock (non-blocking)
	err = lm.Lock(0, 1, unix.F_WRLCK, false)
	assert.Equal(t, syscall.EAGAIN, err, "Expected lock unavailable")

	err = lm.Unlock(0, 1)
	assert.NoError(t, err)
}
