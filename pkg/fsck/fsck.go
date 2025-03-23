package fsck

import (
    "fmt"
    "sync"

    "github.com/juicedata/juicefs/pkg/meta"
    m "github.com/juicedata/juicefs/pkg/meta"
    "github.com/juicedata/juicefs/pkg/utils"
)

type Fsck struct {
    ctx    meta.Mctx
    m      meta.Meta
    wg     sync.WaitGroup
    broken map[meta.Ino]struct{} // Tracks directories with incorrect nlinks
}

func NewFsck(ctx meta.Mctx, m meta.Meta) *Fsck {
    return &Fsck{
        ctx:    ctx,
        m:      m,
        broken: make(map[meta.Ino]struct{}),
    }
}

func (f *Fsck) MarkBrokenDir(ino meta.Ino) {
    f.broken[ino] = struct{}{}
    utils.Logger.Infof("Marked directory inode %d as broken due to ignored nlinks", ino)
}

func (f *Fsck) Check() error {
    utils.Logger.Info("Starting filesystem consistency check")

    // Scan all inodes from root
    var cursor uint64
    var entries []*meta.Entry
    for {
        entries, err := f.m.ReadDir(f.ctx, meta.RootIno, cursor, 0)
        if err != nil {
            return fmt.Errorf("readdir root: %s", err)
        }
        if len(entries) == 0 {
            break
        }
        for _, e := range entries {
            f.wg.Add(1)
            go f.checkInode(e.Ino)
            cursor = e.Ino
        }
    }
    f.wg.Wait()

    // Fix broken directories
    f.fixBrokenDirs()

    utils.Logger.Info("Filesystem check completed")
    return nil
}

func (f *Fsck) checkInode(ino meta.Ino) {
    defer f.wg.Done()

    var attr meta.Attr
    if err := f.m.GetAttr(f.ctx, ino, &attr); err != nil {
        utils.Logger.Warnf("GetAttr inode %d: %s", ino, err)
        return
    }

    if attr.Typ == meta.TypeDirectory {
        f.checkDirectory(ino, &attr)
    }
}

func (f *Fsck) checkDirectory(ino meta.Ino, attr *meta.Attr) {
    var cursor uint64
    var entries []*meta.Entry
    expectedNlink := uint32(2) // . and ..

    for {
        entries, err := f.m.ReadDir(f.ctx, ino, cursor, 0)
        if err != nil {
            utils.Logger.Warnf("ReadDir inode %d: %s", ino, err)
            return
        }
        if len(entries) == 0 {
            break
        }
        for _, e := range entries {
            if e.Ino != ino && e.Ino != attr.Parent && e.Attr.Typ == meta.TypeDirectory {
                expectedNlink++ // Count subdirectories
            }
            cursor = e.Ino
        }
    }

    if attr.Nlink != expectedNlink {
        utils.Logger.Warnf("Directory inode %d has incorrect nlinks: expected %d, got %d", ino, expectedNlink, attr.Nlink)
        f.MarkBrokenDir(ino)
    }
}

func (f *Fsck) fixBrokenDirs() {
    for ino := range f.broken {
        var attr meta.Attr
        if err := f.m.GetAttr(f.ctx, ino, &attr); err != nil {
            utils.Logger.Warnf("Failed to get attributes for inode %d: %s", ino, err)
            continue
        }

        // Recalculate correct nlinks
        var cursor uint64
        var entries []*meta.Entry
        correctNlink := uint32(2) // . and ..

        for {
            entries, err := f.m.ReadDir(f.ctx, ino, cursor, 0)
            if err != nil {
                utils.Logger.Warnf("Failed to read directory inode %d: %s", ino, err)
                break
            }
            if len(entries) == 0 {
                break
            }
            for _, e := range entries {
                if e.Ino != ino && e.Ino != attr.Parent && e.Attr.Typ == meta.TypeDirectory {
                    correctNlink++
                }
                cursor = e.Ino
            }
        }

        // Update nlinks if necessary
        if correctNlink != attr.Nlink {
            attr.Nlink = correctNlink
            if err := f.m.SetAttr(f.ctx, ino, meta.SetAttrNlink, 0, &attr); err != nil {
                utils.Logger.Errorf("Failed to fix nlinks for inode %d: %s", ino, err)
            } else {
                utils.Logger.Infof("Fixed nlinks for inode %d: set to %d", ino, correctNlink)
            }
        }
    }
}

func RunFsck(ctx meta.Mctx, metaURL string) error {
    m, err := m.NewRedisMeta(metaURL, &meta.Config{})
    if err != nil {
        return fmt.Errorf("create redis meta: %s", err)
    }
    f := NewFsck(ctx, m)
    return f.Check()
}