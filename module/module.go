// Package module exposes the stable in-process Metadata module facade.
// Implementation and composition details remain below internal/.
package module

import (
	"github.com/domainry/domainry-foundation/schemaownership"
	moduleassembly "github.com/domainry/domainry-metadata/internal/assembly/module"
)

type Factory = moduleassembly.Factory

func NewFactory() *Factory { return moduleassembly.NewFactory() }

func OwnedTables() []string { return moduleassembly.OwnedTables() }

func SchemaOwnership() []schemaownership.Table { return moduleassembly.SchemaOwnership() }

var OpenDefinitionStore = moduleassembly.OpenDefinitionStore
