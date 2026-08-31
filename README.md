# domainry-metadata

Source-owned Metadata implementation for Domainry. The repository currently
ships an embedded Module topology; Runtime provides its database, SQL dialect,
transaction boundary and migration registrar.

## Layout

- `internal/domain/metadata`: Metadata model, repository ports and domain service.
- `internal/application/metadata`: application use-case boundary.
- `internal/adapter/metadatasdk`: adapter from application services to the public SDK.
- `internal/assembly/module`: embedded Module composition root.
- `internal/assembly/saas`: reserved standalone composition boundary; not supported by the current SDK.
- `internal/infrastructure/persistence`: Domainry ORM-backed persistence and dialect profiles.
- `internal/transport/http`: reserved Module/SaaS HTTP boundaries.
- `module`: thin public facade consumed by Runtime and generated projects.
- `cmd/metadata-server`: reserved standalone entry point; exits unsupported until a SaaS SDK contract exists.

## Verification

```sh
GOWORK=off go test ./...
```
