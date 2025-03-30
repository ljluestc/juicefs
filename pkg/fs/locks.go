package fs

import (
    "os"
    "syscall"
    "github.com/juicedata/juicefs/pkg/meta"
    "golang.org/x/sys/unix"
)

type LockManager struct {
    m      meta.Meta
    file   *os.File
    fd     int
    closed bool
}

func NewLockManager(m meta.Meta, path string) (*LockManager, error) {
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return nil, err
    }
    return &LockManager{
        m:    m,
        file: f,
        fd:   int(f.Fd()),
    }, nil
}

func (lm *LockManager) Lock(start, len int64, lockType int16, block bool) error {
    if lm.closed {
        return syscall.EBADF
    }
    flock := unix.Flock_t{
        Type:   lockType,
        Whence: int16(os.SEEK_SET),
        Start:  start,
        Len:    len,
    }
    op := unix.F_SETLK
    if block {
        op = unix.F_SETLKW
    }
    return unix.FcntlFlock(uintptr(lm.fd), op, &flock)
}

func (lm *LockManager) Unlock(start, len int64) error {
    if lm.closed {
        return syscall.EBADF
    }
    flock := unix.Flock_t{
        Type:   unix.F_UNLCK,
        Whence: int16(os.SEEK_SET),
        Start:  start,
        Len:    len,
    }
    return unix.FcntlFlock(uintptr(lm.fd), unix.F_SETLK, &flock)
}

func (lm *LockManager) Close() error {
    if lm.closed {
        return nil
    }
    lm.closed = true
    return lm.file.Close()
}