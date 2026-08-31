# Development rules

- Answer architecture and implementation questions from the current repository code and tests.
- Keep implementation code under `internal`; `module/` is a thin stable facade.
- Metadata persistence is source-owned but uses the embedding host's database, transaction boundary, SQL dialect, migration lock, and sole `_schema_migrations` ledger.
- Submit embedded migrations through `modulehost.MigrationRegistrar`; never create a Metadata-specific migration ledger.
- Persistence DDL and DML must use `github.com/domainry/domainry-orm`. Raw SQL requires a local justification and dialect-focused tests.
- Do not use worktrees for development.
