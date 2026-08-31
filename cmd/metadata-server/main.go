// metadata-server reserves the standalone process entry point. The process is
// deliberately unavailable until domainry-metadata-sdk defines a SaaS factory
// and transport contract.
package main

import (
	"fmt"
	"os"

	metadataassembly "github.com/domainry/domainry-metadata/internal/assembly/saas"
)

func main() {
	fmt.Fprintln(os.Stderr, metadataassembly.ErrNotSupported)
	os.Exit(1)
}
