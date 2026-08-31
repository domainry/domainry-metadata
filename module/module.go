// Package module exposes the stable in-process Metadata module facade.
// Implementation and composition details remain below internal/.
package module

import (
	moduleassembly "github.com/domainry/domainry-metadata/internal/assembly/module"
)

type Factory = moduleassembly.Factory

func NewFactory() *Factory { return moduleassembly.NewFactory() }

var OwnedTables = moduleassembly.OwnedTables
