// metadata-server reserves the standalone process entry point. The process is
// deliberately unavailable until domainry-metadata-sdk defines a SaaS factory
// and transport contract.
package main

import (
	"context"
	"os"

	"github.com/domainry/domainry-foundation/processhost"
	metadataassembly "github.com/domainry/domainry-metadata/internal/assembly/saas"
)

func main() {
	if run() != nil {
		os.Exit(1)
	}
}

func run() (runErr error) {
	host, err := processhost.Open(context.Background(), processhost.Config{ServiceName: "domainry-metadata"})
	if err != nil {
		return err
	}
	defer func() { runErr = host.Finish(context.Background(), nil, runErr) }()
	return metadataassembly.ErrNotSupported
}
