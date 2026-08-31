// Package module exposes the stable in-process Metadata module facade.
// Implementation and composition details remain below internal/.
package module

import (
	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	moduleassembly "github.com/domainry/domainry-metadata/internal/assembly/module"
)

type Factory = moduleassembly.Factory

func NewFactory() *Factory { return moduleassembly.NewFactory() }

var OwnedTables = moduleassembly.OwnedTables
var SchemaMigrationsForDialect = moduleassembly.SchemaMigrationsForDialect

// NewDefinitionRepository is retained for Runtime's transaction-aware schema
// integration. The implementation remains owned by Metadata.
func NewDefinitionRepository(database moduleassembly.Database, dialect moduleassembly.Dialect) metadatarepository.DefinitionRepository {
	return moduleassembly.NewDefinitionRepository(database, dialect)
}
