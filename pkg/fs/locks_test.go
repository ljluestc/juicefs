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

func (m *mockMeta) Stat(ino uint64) (meta.Entry, error) {
    return meta.Entry{}, nil
}

// Updated meta.Meta implementation
func (m *mockMeta) Access(ctx meta.Context, ino meta.Ino, mask uint8, attr *meta.Attr) syscall.Errno {
    return 0 // Success (no error)
}
func (m *mockMeta) Init(format *meta.Format, force bool) error { return nil }
func (m *mockMeta) Load(all bool) (*meta.Format, error)        { return &meta.Format{}, nil }
func (m *mockMeta) Lookup(ctx meta.Context, parent uint64, name string) (meta.Ino, uint32, error) {
    return 0, 0, nil
}
func (m *mockMeta) GetAttr(ctx meta.Context, ino uint64) (meta.Entry, error) {
    return meta.Entry{}, nil
}
func (m *mockMeta) Open(ctx meta.Context, ino uint64, flags uint32) (uint64, uint32, error) {
    return 0, 0, nil
}
func (m *mockMeta) Close(ctx meta.Context, ino uint64, fh uint64) error { return nil }
func (m *mockMeta) Check(ctx meta.Context, path string, repair, deep, reclaim bool) error {
    return nil
}
func (m *mockMeta) CheckSetAttr(ctx meta.Context, ino meta.Ino, set uint16, attr meta.Attr) syscall.Errno {
    return 0 // Success (no error); Changed *meta.Attr to meta.Attr
}

// TestLockDeadlockDetection simulates a deadlock and verifies EDEADLK.
func TestLockDeadlockDetection(t *testing.T) {
    f, err := os.CreateTemp("", "juicefs-lock-test")
    assert.NoError(t, err)
    defer os.Remove(f.Name())
    defer f.Close()

    lm1, err := NewLockManager(&mockMeta{}, f.Name())
    assert.NoError(t, err)
    defer lm1.Close()

    lm2, err := NewLockManager(&mockMeta{}, f.Name())
    assert.NoError(t, err)
    defer lm2.Close()

    var wg sync.WaitGroup
    wg.Add(2)
    deadlockDetected := false

    go func() {
        defer wg.Done()
        err := lm1.Lock(100, 1, unix.F_WRLCK, true)
        assert.NoError(t, err)

        err = lm1.Lock(200, 1, unix.F_WRLCK, true)
        if err == syscall.EDEADLK {
            deadlockDetected = true
        }
        assert.Equal(t, syscall.EDEADLK, err, "Expected deadlock detection")
    }()

    go func() {
        defer wg.Done()
        err := lm2.Lock(200, 1, unix.F_WRLCK, true)
        assert.NoError(t, err)

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

    err = lm.Lock(0, 1, unix.F_WRLCK, false)
    assert.NoError(t, err)

    err = lm.Lock(0, 1, unix.F_WRLCK, false)
    assert.Equal(t, syscall.EAGAIN, err, "Expected lock unavailable")

    err = lm.Unlock(0, 1)
    assert.NoError(t, err)
}