# domainry-metadata

Source-owned Metadata implementation for Domainry. The repository currently
ships an embedded Module topology; Runtime provides its database, SQL dialect
and migration registrar. Metadata owns its transactions, tables, DML and HTTP
Surface behind the SDK Binding.

## Layout

- `internal/domain/metadata`: dictionary resolution and localization rules.
- `internal/application/metadata`: definition, localization and projection use cases.
- `internal/adapter/metadatasdk`: adapter from application services to the public SDK.
- `internal/assembly/module`: embedded Module composition root.
- `internal/assembly/saas`: reserved standalone composition boundary; not supported by the current SDK.
- `internal/infrastructure/persistence`: Domainry ORM-backed persistence and dialect profiles.
- `internal/transport/http/module`: Metadata-owned definition, localization and dictionary product HTTP Surface.
- `module`: thin public facade consumed by Runtime and generated projects.
- `cmd/metadata-server`: reserved standalone entry point; exits unsupported until a SaaS SDK contract exists.

Metadata initializes only its current source-owned catalog, definition-version,
localization and projection tables. The project has not shipped, so no legacy
table import or compatibility migration is retained. The SDK exposes business
ports only; no consumer can type-assert a Metadata persistence repository.

## Verification

```sh
go test ./...
```

After publishing and pinning the matching Metadata SDK version, also run the
same suite with `GOWORK=off` to verify the immutable module dependency graph.
