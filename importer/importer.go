package importer

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/location"
	"github.com/PlakarKorp/kloset/objects"
)

type TestImporter struct {
	scanDir string
}

func init() {
	importer.Register("test", 0, NewTestImporter)
}

func NewTestImporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (importer.Importer, error) {
	loc, ok := config["location"]
	if !ok {
		return nil, fmt.Errorf("missing location")
	}

	scanDir := strings.TrimPrefix(loc, proto+"://")
	if scanDir == "" {
		return nil, fmt.Errorf("empty path after %s://", proto)
	}

	return &TestImporter{
		scanDir: scanDir,
	}, nil
}

func (f *TestImporter) Root() string          { return f.scanDir }
func (f *TestImporter) Origin() string        { return "localhost" }
func (f *TestImporter) Type() string          { return "test" }
func (f *TestImporter) Flags() location.Flags { return 0 }

func (f *TestImporter) Ping(ctx context.Context) error {
	_, err := os.Stat(f.scanDir)
	return err
}

func (f *TestImporter) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
	defer close(records)

	return filepath.WalkDir(f.scanDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == f.scanDir {
				return err
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		fi := objects.FileInfo{
			Lname:    filepath.Base(path),
			Lsize:    info.Size(),
			Lmode:    info.Mode(),
			LmodTime: info.ModTime(),
			Ldev:     1,
		}

		records <- connectors.NewRecord(path, "", fi, nil, func() (io.ReadCloser, error) {
			return os.Open(path)
		})

		return nil
	})
}

func (f *TestImporter) Close(ctx context.Context) error {
	return nil
}
