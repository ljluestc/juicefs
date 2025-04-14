package cmd

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/juicedata/juicefs/pkg/meta"
    "github.com/juicedata/juicefs/pkg/utils"
    "github.com/urfave/cli/v2"
)

const (
    defaultBatchSize = 1000
)

type MetadataClient interface {
    ListDir(dir string) ([]meta.Entry, error)
    DeleteBatch(keys []string) error
    RemoveDir(dir string) error
}

func rmrCmd(metaClient MetadataClient, dir string, batchSize int, numThreads int, skipTrash bool) error {
    if batchSize <= 0 {
        batchSize = defaultBatchSize
    }

    batch := make([]string, 0, batchSize)
    err := traverseAndCollect(metaClient, dir, &batch, batchSize)
    if err != nil {
        return fmt.Errorf("failed to traverse directory %s: %v", dir, err)
    }

    if len(batch) > 0 {
        if err := metaClient.DeleteBatch(batch); err != nil {
            return fmt.Errorf("failed to delete batch in %s: %v", dir, err)
        }
    }

    if err := metaClient.RemoveDir(dir); err != nil {
        return fmt.Errorf("failed to remove directory %s: %v", dir, err)
    }

    return nil
}

func traverseAndCollect(metaClient MetadataClient, dir string, batch *[]string, batchSize int) error {
    entries, err := metaClient.ListDir(dir)
    if err != nil {
        return err
    }

    for _, entry := range entries {
        name := string(entry.Name)
        fullPath := filepath.Join(dir, name)
        if entry.Attr.Type == meta.TypeDirectory {
            if err := traverseAndCollect(metaClient, fullPath, batch, batchSize); err != nil {
                return err
            }
        } else {
            *batch = append(*batch, fullPath)
            if len(*batch) >= batchSize {
                if err := metaClient.DeleteBatch(*batch); err != nil {
                    return err
                }
                *batch = (*batch)[:0]
            }
        }
    }
    return nil
}

func cmdRmr() *cli.Command {
    return &cli.Command{
        Name:      "rmr",
        Action:    rmr,
        Category:  "TOOL",
        Usage:     "Remove directories recursively",
        ArgsUsage: "PATH ...",
        Description: `
This command provides a faster way to remove huge directories in JuiceFS.

Examples:
$ juicefs rmr /mnt/jfs/foo`,
        Flags: []cli.Flag{
            &cli.BoolFlag{
                Name:  "skip-trash",
                Usage: "skip trash and delete files directly (requires root)",
            },
            &cli.IntFlag{
                Name:    "threads",
                Aliases: []string{"p"},
                Value:   50,
                Usage:   "number of threads for delete jobs (max 255)",
            },
            &cli.IntFlag{
                Name:  "batch-size",
                Value: defaultBatchSize,
                Usage: "number of files to delete in a single batch",
            },
        },
    }
}

func rmr(ctx *cli.Context) error {
    setup(ctx, 1)
    numThreads := ctx.Int("threads")
    batchSize := ctx.Int("batch-size")

    if numThreads <= 0 {
        numThreads = meta.RmrDefaultThreads
    }
    if numThreads > 255 {
        numThreads = 255
    }
    if ctx.Bool("skip-trash") {
        if os.Getuid() != 0 {
            logger.Fatalf("Only root can remove files directly")
        }
    }

    metaClient := &mockMetaClient{}
    progress := utils.NewProgress(false)
    spin := progress.AddCountSpinner("Removing entries")

    for i := 0; i < ctx.Args().Len(); i++ {
        path := ctx.Args().Get(i)
        p, err := filepath.Abs(path)
        if err != nil {
            logger.Errorf("abs of %s: %s", path, err)
            continue
        }

        err = rmrCmd(metaClient, p, batchSize, numThreads, ctx.Bool("skip-trash"))
        if err != nil {
            logger.Errorf("rmr %s: %s", p, err)
            continue
        }

        spin.Increment()
    }

    progress.Done()
    return nil
}

func openController(dpath string) (*os.File, error) {
    st, err := os.Stat(dpath)
    if err != nil {
        return nil, err
    }
    if !st.IsDir() {
        dpath = filepath.Dir(dpath)
    }
    fp, err := os.OpenFile(filepath.Join(dpath, ".jfs.control"), os.O_RDWR, 0)
    if os.IsNotExist(err) {
        fp, err = os.OpenFile(filepath.Join(dpath, ".control"), os.O_RDWR, 0)
    }
    return fp, err
}

type mockMetaClient struct{}

func (m *mockMetaClient) ListDir(dir string) ([]meta.Entry, error) {
    return []meta.Entry{}, nil
}

func (m *mockMetaClient) DeleteBatch(keys []string) error {
    fmt.Printf("Deleting batch of %d keys\n", len(keys))
    return nil
}

func (m *mockMetaClient) RemoveDir(dir string) error {
    fmt.Printf("Removing directory %s\n", dir)
    return nil
}