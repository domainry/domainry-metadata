# When should a product read projected definitions from Metadata?

## Problems solved

- Provides one read-only catalog of currently projected source-owned definitions, provenance, schema version, and hash without creating a second authoring authority.

## Business scenarios

- An administration UI discovers the installed Object, Field, Action, Workflow, Report, Scheduler, Agent, and Business Calendar definitions for diagnostics or navigation.
- A release investigation compares definition source, schema hash, and projected payload to explain why an expected capability is missing or stale.
- A failed source refresh marks a projection stale/unavailable instead of presenting yesterday's payload as current truth.

## Use when

Use definition projection for discovery, diagnostics, localized presentation, or cross-owner inventory of definitions already published by their semantic owners.

## Do not use when

Do not edit a projected payload, treat Metadata as the owner of Workflow/Report/Scheduler rules, or infer that projection availability means a compiler can author that capability.

## How to use

List with an explicit owner (or an explicitly authorized cross-owner query), fetch
one stable resource key, and use `current_version_id` as the opaque revision
token. Source owners replace generated catalogs through
`DefinitionStore.ReplaceSourceSnapshot`. Independent publications use
`DefinitionStore.Publish` with the current token; a create must use
`DefinitionNoCurrentVersion`. Disable uses the same token and immutable history
is read through `GetVersion`. A stale token fails with
`metadata.definition_revision_conflict`; callers must reread instead of blindly
overwriting the winner.

## Adaptation cookbook

| Discovery requirement | Adapt with | Concrete implementation | Wrong adaptation |
| --- | --- | --- | --- |
| Admin UI lists currently installed business definitions | Metadata definition list/get | Filter the projected catalog, show stable key/name/source/schema hash, and follow owner-specific permissions for details | Querying every module's tables or treating the payload as editable Metadata state |
| Release diagnosis asks why a Workflow differs | Projection provenance and schema hash | Compare source kind/source ID, schema version, definition hash, and current owner publication; then inspect the Workflow owner contract | Patching the projected JSON or assuming Metadata caused the semantic difference |
| UI needs a French label for a projected Field | Definition projection plus localization | Use the stable owner/resource/text key to resolve localized display text with fallback and coverage evidence | Translating field validation, permission, or execution semantics |
| Product wants to define a new recurring job | Scheduler authoring contract | Author and validate the definition through Scheduler/Runtime owner | Creating a `scheduler` Metadata row and expecting execution |

## Example

An administration page lists minimal installed definitions from several owners and shows source owner, source ID, schema version, definition hash, and provenance. If the Scheduler source refresh fails, the page marks that projection stale/unavailable with its last successful revision rather than claiming it is current. Diagnosis follows the provenance back to Scheduler for correction; Metadata never edits the projected payload or turns catalog presence into compiler authorability.

## Permissions and scope

Definition list/get requires discovery permission and Workspace scope. Projection must not reveal source-owned definitions the caller is not allowed to discover.

## Boundaries

Metadata owns the read model, provenance, localization, and dictionary lookup. Object, Workflow, Report, Scheduler, Agent, and other modules retain semantic ownership and authoring validation.
