// Package capability exposes Metadata's source-owned capability contract
// without opening persistence or projection services.
package capability

import (
	"github.com/domainry/domainry-foundation/modulecapability"
)

type Inputs struct{}

func Open(inputs Inputs) (*modulecapability.StaticBinding, error) {
	return openContract(inputs)
}
