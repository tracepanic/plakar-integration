package main

import (
	"os"

	sdk "github.com/PlakarKorp/go-kloset-sdk"
	"github.com/tracepanic/plakar-integration/importer"
)

func main() {
	sdk.EntrypointImporter(os.Args, importer.NewTestImporter)
}
