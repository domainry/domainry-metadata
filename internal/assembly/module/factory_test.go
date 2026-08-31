package moduleassembly

import (
	"context"
	"database/sql"
	"testing"

	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	_ "modernc.org/sqlite"
)

type testHost struct {
	database  *sql.DB
	dialect   modulehost.Dialect
	registrar *testRegistrar
}

func (h testHost) Database() modulehost.Database             { return h.database }
func (h testHost) Dialect() modulehost.Dialect               { return h.dialect }
func (h testHost) Migrations() modulehost.MigrationRegistrar { return h.registrar }

type testRegistrar struct {
	database *sql.DB
	owner    string
}

func (*testRegistrar) Driver() string { return "sqlite" }
func (*testRegistrar) Schema() string { return "" }
func (r *testRegistrar) ApplyOwnedMigrations(ctx context.Context, owner string, migrations []modulehost.SchemaMigration) error {
	r.owner = owner
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := r.database.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestFactoryComposesLayeredModuleAndUsesHostMigrationRegistrar(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	dialect, err := dialectForTest()
	if err != nil {
		t.Fatal(err)
	}
	registrar := &testRegistrar{database: database}
	binding, err := NewFactory().OpenModule(t.Context(), metadatasdk.ApplicationRef{InstallationID: "installation"}, testHost{database: database, dialect: dialect, registrar: registrar})
	if err != nil {
		t.Fatal(err)
	}
	if registrar.owner != "metadata" {
		t.Fatalf("migration owner=%q", registrar.owner)
	}
	if descriptor := binding.Descriptor(); descriptor.ProtocolVersion != metadatasdk.ProtocolVersionV1 || descriptor.Mode != "module" {
		t.Fatalf("descriptor=%+v", descriptor)
	}
	if err := binding.Descriptor().Validate(); err != nil {
		t.Fatal(err)
	}
	if binding.Definitions() == nil || binding.Localization() == nil || binding.Dictionaries() == nil || binding.Projection() == nil {
		t.Fatal("Metadata Binding business ports are incomplete")
	}
	provider, ok := binding.(interface{ HTTPSurfaces() []modulehttp.Surface })
	if !ok || len(provider.HTTPSurfaces()) != 1 {
		t.Fatal("Metadata Binding HTTP Surface is unavailable")
	}
	if err := binding.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}
