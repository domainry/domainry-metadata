// Package module exposes the stable in-process Metadata module facade.
// Implementation and composition details remain below internal/.
package module

import (
	"github.com/domainry/domainry-foundation/schemaownership"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	moduleassembly "github.com/domainry/domainry-metadata/internal/assembly/module"
	metadatastore "github.com/domainry/domainry-metadata/internal/infrastructure/persistence/database/metadata"
)

type Factory = moduleassembly.Factory

func NewFactory() *Factory { return moduleassembly.NewFactory() }

func OwnedTables() []string { return moduleassembly.OwnedTables() }

func SchemaOwnership() []schemaownership.Table { return moduleassembly.SchemaOwnership() }

// SchemaMigrations exposes Metadata's one source-owned localization schema for
// cross-module composition verification. Shared Definition DDL remains owned
// and installed by Foundation.
func SchemaMigrations(driver, schema string) ([]modulehost.SchemaMigration, error) {
	return metadatastore.SchemaMigrations(driver, schema)
}

var OpenDefinitionStore = moduleassembly.OpenDefinitionStore
