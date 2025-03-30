package fs

import (
	"os"
	"syscall"

	"github.com/juicedata/juicefs/pkg/meta"
	"golang.org/x/sys/unix"
)

// LockManager manages POSIX locks for JuiceFS files.
type LockManager struct {
	m    meta.Meta // Metadata interface
	file *os.File  // File descriptor for locking
}

// NewLockManager initializes a LockManager for a given file path.
func NewLockManager(m meta.Meta, path string) (*LockManager, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return &LockManager{m: m, file: f}, nil
}

// Lock acquires a POSIX lock (shared or exclusive) with optional blocking.
func (lm *LockManager) Lock(start, len int64, lockType int16, block bool) error {
	flock := syscall.Flock_t{
		Type:   lockType, // F_RDLCK (shared) or F_WRLCK (exclusive)
		Whence: int16(os.SEEK_SET),
		Start:  start,
		Len:    len,
		Pid:    0, // Current process
	}

	// Use F_SETLK for non-blocking, F_SETLKW for blocking (with deadlock detection)
	cmd := unix.F_SETLK
	if block {
		cmd = unix.F_SETLKW
	}

	err := unix.FcntlFlock(lm.file.Fd(), cmd, &flock)
	if err != nil {
		switch err {
		case syscall.EDEADLK:
			// Deadlock detected by kernel, return as-is
			return syscall.EDEADLK
		case syscall.EAGAIN, syscall.EACCES:
			// Lock unavailable (non-blocking case)
			return syscall.EAGAIN
		default:
			// Other errors (e.g., EBADF, EINVAL)
			return err
		}
	}
	return nil
}

// Unlock releases a POSIX lock.
func (lm *LockManager) Unlock(start, len int64) error {
	flock := syscall.Flock_t{
		Type:   unix.F_UNLCK,
		Whence: int16(os.SEEK_SET),
		Start:  start,
		Len:    len,
		Pid:    0,
	}
	return unix.FcntlFlock(lm.file.Fd(), unix.F_SETLK, &flock)
}

// Close releases the file descriptor.
func (lm *LockManager) Close() error {
	return lm.file.Close()
}
