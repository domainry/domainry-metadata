# How should source-owned definitions be localized without changing their meaning?

## Problems solved

- Localizes display text and exposes coverage gaps while preserving the source owner's keys, validation, and business semantics.

## Business scenarios

- Translating a Report measure and description into French without redefining the measure.
- Providing localized field or module labels with deterministic locale fallback and missing-translation reporting.

## Use when

Use localization when fields, reports, schedules, or other published definitions need locale-specific labels and coverage inspection.

## Do not use when

Do not copy the definition into Metadata or let a translation change validation, permissions, query logic, or retry semantics.

## How to use

The semantic owner publishes a stable definition key. Metadata stores localized display text, applies fallback rules, and reports missing translations.

## Adaptation cookbook

| Business requirement | Adapt with | Concrete implementation | Wrong adaptation |
| --- | --- | --- | --- |
| A Report measure needs French and Chinese labels | Source-owned measure key plus Metadata translations | Report owns calculation and stable key; Metadata stores locale-specific label/description and applies deterministic fallback | Recreating the measure in Metadata or translating its formula into a second semantic definition |
| Object fields and module navigation need localized display text | Metadata entries keyed by owner, resource type, resource ID, and text key | Resolve requested locale, fall back to approved default, and report missing coverage without changing the source definition | Using translated display text as a permission, API, or persistence identifier |
| Product team needs translation coverage before release | Metadata coverage query | Compare published source keys with locale entries and expose missing/stale translations by owner | Scanning rendered UI strings and assuming that proves source-owned definitions are complete |
| A business rule differs by country | Owning domain policy or Operation | Model the rule explicitly with testable semantics; localize only its explanation | Encoding different business behavior as locale text |

## Example

Report publishes a revenue measure. Metadata provides French label “Revenu” and flags a missing description; Report still owns the measure/query.

## Permissions and scope

Readers see only definitions they may discover. Translators receive localization permission, not authority to edit the source definition.

## Boundaries

Localization is presentation metadata. The source module remains authoritative for every executable semantic.
