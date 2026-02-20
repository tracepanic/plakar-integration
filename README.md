# Plakar Plugin Development Guide

This repository is a reference implementation for building plakar plugins (integrations). It demonstrates how to create custom connectors that extend plakar's backup and restore capabilities.

## Overview

Plakar plugins are standalone executables that communicate with plakar over gRPC through stdin/stdout. A plugin can provide up to three types of connectors:

- **Importer** - Defines how data is read from a source (used during `plakar backup`)
- **Exporter** - Defines how data is written to a destination (used during `plakar restore`)
- **Storage** - Defines a custom storage backend for the repository itself

An integration does not have to provide all three connector types. You only implement what your plugin needs. For example, a plugin that only imports data from an API would only need an importer. To remove a connector type from your plugin, simply:

1. Remove its entry from `manifest.yaml`
2. Remove the corresponding command directory (e.g., `importer/`, `exporter/`, or `storage/`)
3. Remove its registration and interface implementations from the `connectors.go`

## Project Structure

```
.
├── connector.go        # Shared connector logic and interface implementations
├── importer/
│   └── main.go         # Importer entrypoint
├── exporter/
│   └── main.go         # Exporter entrypoint
├── manifest.yaml       # Plugin manifest describing the connectors
├── Makefile            # Build and packaging targets
├── go.mod
└── go.sum
```

## Dependencies

The two key dependencies are:

- `github.com/PlakarKorp/kloset` - Core plakar types and interfaces (`connectors`, `objects`, `location`, etc.)
- `github.com/PlakarKorp/go-kloset-sdk` - SDK providing the gRPC entrypoints that handle communication with plakar

## How It Works

### The Connector

The connector is where you implement your logic. It must satisfy the relevant interfaces depending on which connector types you provide.

**Importer interface:**

```go
type Importer interface {
  Origin() string
  Type() string
  Root() string
  Flags() location.Flags
  Ping(context.Context) error
  Import(context.Context, chan<- *connectors.Record, <-chan *connectors.Result) error
  Close(context.Context) error
}
```

**Exporter interface:**

```go
type Exporter interface {
  Origin() string
  Type() string
  Root() string
  Flags() location.Flags
  Ping(context.Context) error
  Export(context.Context, <-chan *connectors.Record, chan<- *connectors.Result) error
  Close(context.Context) error
}
```

**Storage interface:**

```go
type Store interface {
  Create(context.Context, []byte) error
  Open(context.Context) ([]byte, error)
  Ping(context.Context) error

  Origin() string
  Type() string
  Root() string
  Flags() location.Flags

  Mode(context.Context) (Mode, error)
  Size(context.Context) (int64, error)
  List(context.Context, StorageResource) ([]objects.MAC, error)
  Put(context.Context, StorageResource, objects.MAC, io.Reader) (int64, error)
  Get(context.Context, StorageResource, objects.MAC, *Range) (io.ReadCloser, error)
  Delete(context.Context, StorageResource, objects.MAC) error

  Close(ctx context.Context) error
}
```

A single struct can implement both the importer and exporter interfaces if your plugin provides both.

### Constructor Functions

Each connector type needs a constructor function that plakar calls to create an instance:

```go
func NewImporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (importer.Importer, error)
func NewExporter(ctx context.Context, opts *connectors.Options, proto string, config map[string]string) (exporter.Exporter, error)
```

The storage constructor has a different signature — it does not receive `*connectors.Options`:

```go
func NewStore(ctx context.Context, proto string, config map[string]string) (storage.Store, error)
```

The `config` map contains the parsed location and other settings. The `config["location"]` value holds the full URI (e.g., `test:///some/path`).

### Registration

In your connector file, register your connector types in an `init()` function:

```go
func init() {
  importer.Register("test", location.FLAG_LOCALFS, NewImporter)
  exporter.Register("test", location.FLAG_LOCALFS, NewExporter)
}
```

The first argument is the protocol name (used as the `protocol://` prefix in plakar commands). The second argument is the location flag (see [Flags](#flags) below).

### Flags

Flags describe the behavior and capabilities of your connector. They are set both in code (during registration and via the `Flags()` method) and in the manifest. The available flags are:

| Flag | Manifest value | Applies to | Description |
|------|---------------|------------|-------------|
| `location.FLAG_LOCALFS` | `localfs` | All | The connector deals with files or directories on the local filesystem. When set, plakar will resolve relative paths against the current working directory. |
| `location.FLAG_FILE` | `file` | Storage | The storage backend stores the kloset in a single file. |
| `location.FLAG_STREAM` | `stream` | Importer | The importer can only call `Import()` once (e.g., reading from a stream that cannot be replayed). |
| `location.FLAG_NEEDACK` | `needack` | Importer | The importer cares about acknowledgments — it will read from the `results` channel during `Import()`. |

Flags can be combined with bitwise OR. For example, a streaming importer on the local filesystem:

```go
func (f *myConnector) Flags() location.Flags {
  return location.FLAG_LOCALFS | location.FLAG_STREAM
}
```

For remote or API-based connectors that don't deal with local paths, you can use `0` (no flags):

```go
func (f *myConnector) Flags() location.Flags {
  return 0
}
```

### Entrypoints

Each connector type gets its own `main.go` in a separate directory. These are minimal — they just call the SDK entrypoint with your constructor:

**importer/main.go:**

```go
package main

import (
  "os"
  sdk "github.com/PlakarKorp/go-kloset-sdk"
  connector "github.com/tracepanic/plakar-integration"
)

func main() {
  sdk.EntrypointImporter(os.Args, connector.NewImporter)
}
```

**exporter/main.go:**

```go
package main

import (
  "os"
  sdk "github.com/PlakarKorp/go-kloset-sdk"
  connector "github.com/tracepanic/plakar-integration"
)

func main() {
  sdk.EntrypointExporter(os.Args, connector.NewExporter)
}
```

### Import Method

The `Import` method sends records into the `records` channel. Each record represents a file with its metadata and a function to open its content:

```go
func (f *testConnector) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
  defer close(records)

  info, err := os.Stat(path)
  if err != nil {
      return err
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
}
```

You **must** `close(records)` when done to signal that all records have been sent.

### Export Method

The `Export` method receives records from the `records` channel and processes them. You **must** `close(results)` when done and send a result for each record via `record.Ok()` or `record.Error(err)`:

```go
func (f *testConnector) Export(ctx context.Context, records <-chan *connectors.Record, results chan<- *connectors.Result) error {
  defer close(results)

  for record := range records {
    // Process the record...

    if record.Reader != nil {
        // Read the content from record.Reader
    }

    results <- record.Ok()
  }

  return nil
}
```

Note that `record.Ok()` and `record.Error()` implicitly close the reader, so you do not need to call `record.Close()` yourself.

## Manifest

The `manifest.yaml` file describes your plugin and its connectors:

```yaml
name: test-integration
description: Import files from local filesystem
connectors:
  - type: importer
    executable: ./test-importer
    homepage: https://github.com/tracepanic/plakar-integration
    license: ISC
    protocols: [test]
    flags: [localfs]
  - type: exporter
    executable: ./test-exporter
    homepage: https://github.com/tracepanic/plakar-integration
    license: ISC
    protocols: [test]
    flags: [localfs]
```

Each connector entry specifies:

- `type` - One of `importer`, `exporter`, or `storage`
- `executable` - Path to the built binary (relative to the package)
- `protocols` - The protocol prefix(es) this connector handles
- `flags` - Location flags (e.g., `localfs`)

## Building

```sh
go build -v -o test-importer ./importer
go build -v -o test-exporter ./exporter
```

This compiles the importer and exporter into separate binaries (`test-importer` and `test-exporter`).

## Packaging

```sh
plakar pkg create manifest.yaml v1.1.0-beta.4
```

This creates a `.ptar` package file using the `plakar pkg create` command that is built into Plakar. The first argument is the manifest file and the second is the Plakar version.

## Installing

```sh
plakar pkg add <package-file>.ptar
```

To verify the package was installed:

```sh
plakar pkg show
# s3@v1.1.0-beta.2
# test-integration@v1.1.0-beta.4
```

## Usage

Once installed, use your protocol name in plakar commands. The protocol URI format depends on the type of integration:

- **Local filesystem** - `protocol:///path/to/directory` (e.g., `test:///home/user/Documents`)
- **Remote/API-based** - `protocol://host-or-endpoint` (e.g., `s3://us-east-1.amazonaws.com/bucket`)
- **No location needed** - `protocol://` (e.g., `notion://`) when the integration handles everything internally

This example integration uses `test://` with hardcoded paths, so the location after `test://` is ignored — the importer always reads from `/home/tracepanic/Documents/notes.md` as the hard coded file location.

## Examples: Different Integration Patterns

The connector in this repository hardcodes a single file path. Real integrations typically fall into one of three patterns: walking a local directory, talking to a remote API, or providing a storage backend. Below are minimal examples of each.

### Walking a Local Directory

Instead of hardcoding paths, parse the location from config and walk the directory:

```go
// Parse path from config and walk it
scanDir := strings.TrimPrefix(config["location"], proto+"://")

func (f *fsConnector) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
  defer close(records)

  return filepath.WalkDir(f.scanDir, func(path string, d fs.DirEntry, err error) error {
    // ... stat each file, build FileInfo, send record
    records <- connectors.NewRecord(path, "", fi, nil, func() (io.ReadCloser, error) {
        return os.Open(path)
    })
    return nil
  })
}

func (f *fsConnector) Export(ctx context.Context, records <-chan *connectors.Record, results chan<- *connectors.Result) error {
  defer close(results)

  for record := range records {
    // ... create file at filepath.Join(f.scanDir, record.Pathname)
    // ... io.Copy(fp, record.Reader)
    results <- record.Ok()
  }
  return nil
}
```

Usage: `plakar at $HOME/backups backup myfs:///home/tracepanic/Documents`

### Remote API (Notion)

An API-based integration like Notion doesn't deal with local paths at all. It reads config for authentication and fetches data over HTTP.

Key differences from a local filesystem connector:

- **No `FLAG_LOCALFS`** — uses `FLAG_STREAM` instead, since the import can't be replayed
- **Authentication via config** — reads an API token from `config["token"]`
- **Origin is the remote service** — `Origin()` returns `"notion.so"` instead of `"localhost"`
- **Records are built from API responses** — content comes from HTTP calls, not `os.Open`

```go
func init() {
  importer.Register("notion", location.FLAG_STREAM, NewNotionImporter)
}

func NewNotionImporter(ctx context.Context, opts *connectors.Options, name string, config map[string]string) (importer.Importer, error) {
  token, ok := config["token"]
  if !ok {
    return nil, fmt.Errorf("missing token in config")
  }
  return &NotionImporter{token: token}, nil
}

func (p *NotionImporter) Origin() string        { return "notion.so" }
func (p *NotionImporter) Flags() location.Flags { return location.FLAG_STREAM }
```

Usage: `plakar at $HOME/backups backup notion://`

See the full implementation at [github.com/PlakarKorp/integration-notion](https://github.com/PlakarKorp/integration-notion/).

### Remote Service (S3)

S3 provides all three connector types — importer, exporter, and storage. The example below shows the importer, which lists objects in a bucket and sends each as a record.

Key differences from a local filesystem connector:

- **No `FLAG_LOCALFS`** — uses `0` flags since it's a remote backend
- **Reads credentials from config** — `access_key`, `secret_access_key`, etc.
- **Content comes from the S3 API** — `minioClient.GetObject` instead of `os.Open`

```go
func init() {
    importer.Register("s3", 0, NewS3Importer)
}

// Parse bucket and path from config["location"] (e.g., "s3://host/bucket/prefix")
// Connect to S3 using access_key and secret_access_key from config

func (p *S3Importer) Import(ctx context.Context, records chan<- *connectors.Record, results <-chan *connectors.Result) error {
  defer close(records)

  for object := range p.minioClient.ListObjects(ctx, p.bucket, listopts) {
    fi := objects.FileInfo{
      Lname:    path.Base("/" + object.Key),
      Lsize:    object.Size,
      Lmode:    0700,
      LmodTime: object.LastModified,
      Ldev:     1,
    }

    records <- connectors.NewRecord("/"+object.Key, "", fi, nil, func() (io.ReadCloser, error) {
      return p.minioClient.GetObject(ctx, p.bucket, object.Key, minio.GetObjectOptions{})
    })
  }
  return nil
}

func (p *S3Importer) Origin() string        { return p.host + "/" + p.bucket }
func (p *S3Importer) Flags() location.Flags { return 0 }
```

Usage: `plakar at $HOME/backups backup s3://us-east-1.amazonaws.com/my-bucket`

See the full implementation at [github.com/PlakarKorp/integration-s3](https://github.com/PlakarKorp/integration-s3).

## Important: Do Not Write to Stdout

Plugins communicate with plakar over gRPC through stdin/stdout. Any writes to `os.Stdout` will corrupt the gRPC stream and break communication. If you need to log or print debug output, always use `os.Stderr`:

```go
fmt.Fprintf(os.Stderr, "debug: processing %s\n", path)
```
