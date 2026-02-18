package connector

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PlakarKorp/kloset/connectors"
	"github.com/PlakarKorp/kloset/connectors/exporter"
	"github.com/PlakarKorp/kloset/connectors/importer"
	"github.com/PlakarKorp/kloset/location"
	"github.com/PlakarKorp/kloset/objects"
)

type testConnector struct{}

const FILE = "/home/tracepanic/Documents/notes.md"

func init() {
	importer.Register("test", location.FLAG_LOCALFS, NewImporter)
	exporter.Register("test", location.FLAG_LOCALFS, NewExporter)
}

func NewImporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (importer.Importer, error) {
	return &testConnector{}, nil
}

func NewExporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (exporter.Exporter, error) {
	return &testConnector{}, nil
}

func (f *testConnector) Root() string   { return "/home/tracepanic/Documents" }
func (f *testConnector) Origin() string { return "localhost" }
func (f *testConnector) Type() string   { return "test" }

func (f *testConnector) Flags() location.Flags {
	return location.FLAG_LOCALFS
}

func (f *testConnector) Ping(ctx context.Context) error {
	return nil
}

func (f *testConnector) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
	defer close(records)

	info, err := os.Stat(FILE)
	if err != nil {
		return err
	}

	fi := objects.FileInfo{
		Lname:    filepath.Base(FILE),
		Lsize:    info.Size(),
		Lmode:    info.Mode(),
		LmodTime: info.ModTime(),
		Ldev:     1,
	}

	records <- connectors.NewRecord(FILE, "", fi, nil, func() (io.ReadCloser, error) {
		return os.Open(FILE)
	})

	return nil
}

func (f *testConnector) Export(ctx context.Context, records <-chan *connectors.Record, results chan<- *connectors.Result) error {
	defer close(results)

	for record := range records {
		fmt.Fprintf(os.Stderr, "--- %s ---\n", record.Pathname)

		if record.Reader != nil {
			if _, err := io.Copy(os.Stderr, record.Reader); err != nil {
				results <- record.Error(err)
				continue
			}
			fmt.Fprintln(os.Stderr)
		}

		results <- record.Ok()
	}

	return nil
}

func (f *testConnector) Close(ctx context.Context) error {
	return nil
}
