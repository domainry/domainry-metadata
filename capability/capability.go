// Package capability exposes Metadata's source-owned capability contract
// without opening persistence or projection services.
package capability

import (
	"github.com/domainry/domainry-foundation/modulecapability"
	metadatahttp "github.com/domainry/domainry-metadata/internal/transport/http/module"
)

type Inputs struct{}

func Open(Inputs) (*modulecapability.StaticBinding, error) {
	return metadatahttp.NewCapabilityBinding(metadatahttp.ValidateCapabilityCandidate)
}
