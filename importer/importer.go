package importer

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

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

func NewTestImporter(ctx context.Context, opts *connectors.Options, name string, config map[string]string) (importer.Importer, error) {
	loc, ok := config["location"]
	if !ok {
		return nil, fmt.Errorf("missing location")
	}

	prefix := name + "://"

	if len(loc) < len(prefix) || loc[:len(prefix)] != prefix {
		return nil, fmt.Errorf("location must start with %s", prefix)
	}

	rawPath := loc[len(prefix):]
	if rawPath == "" {
		return nil, fmt.Errorf("empty path after %s", prefix)
	}

	rawPath = os.ExpandEnv(rawPath)
	if rawPath[:1] == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			rawPath = filepath.Join(home, rawPath[1:])
		}
	}

	cleanPath := filepath.Clean(rawPath)

	if _, err := os.Stat(cleanPath); err != nil {
		return nil, fmt.Errorf("cannot access location: %w", err)
	}

	return &TestImporter{
		scanDir: cleanPath,
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

	if err := f.Ping(ctx); err != nil {
		return err
	}

	return filepath.WalkDir(f.scanDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
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
