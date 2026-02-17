package connector

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/exporter"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/location"
	"github.com/PlakarKorp/kloset/objects"
)

type testConnector struct {
	scanDir string
}

func init() {
	importer.Register("test", location.FLAG_LOCALFS, NewImporter)
	exporter.Register("test", location.FLAG_LOCALFS, NewExporter)
}

func NewImporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (importer.Importer, error) {
	return newConnector(proto, config)
}

func NewExporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (exporter.Exporter, error) {
	return newConnector(proto, config)
}

func newConnector(proto string, config map[string]string) (*testConnector, error) {
	loc, ok := config["location"]
	if !ok {
		return nil, fmt.Errorf("missing location")
	}

	scanDir := strings.TrimPrefix(loc, proto+"://")
	if scanDir == "" {
		return nil, fmt.Errorf("empty path after %s://", proto)
	}

	return &testConnector{
		scanDir: scanDir,
	}, nil
}

func (f *testConnector) Root() string   { return f.scanDir }
func (f *testConnector) Origin() string { return "localhost" }
func (f *testConnector) Type() string   { return "test" }

func (f *testConnector) Flags() location.Flags {
	return location.FLAG_LOCALFS
}

func (f *testConnector) Ping(ctx context.Context) error {
	_, err := os.Stat(f.scanDir)
	return err
}

func (f *testConnector) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
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

func (f *testConnector) Export(ctx context.Context, records <-chan *connectors.Record, results chan<- *connectors.Result) error {
	defer close(results)

	for record := range records {
		pathname := strings.TrimPrefix(record.Pathname, "/")
		path := filepath.Join(f.scanDir, pathname)

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			results <- record.Error(err)
			continue
		}

		fp, err := os.Create(path)
		if err != nil {
			results <- record.Error(err)
			continue
		}

		if record.Reader != nil {
			_, err = io.Copy(fp, record.Reader)
			record.Close()
		}
		fp.Close()

		if err != nil {
			results <- record.Error(err)
		} else {
			results <- record.Ok()
		}
	}

	return nil
}

func (f *testConnector) Close(ctx context.Context) error {
	return nil
}
