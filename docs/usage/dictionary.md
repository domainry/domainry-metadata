# When should reusable values use Metadata dictionaries?

## Problems solved

- Governs shared, localizable reference values once instead of duplicating the same catalog and ordering rules in multiple domains.

## Business scenarios

- Reusing a country, reason-code, or shared classification catalog across several modules.
- Publishing centrally ordered and localized values whose lifecycle is independent of one business Object.
- Resolving a disabled historical value for display while preventing it from being selected by new writes.
- Mapping a rapidly changing Provider-owned catalog through Integration rather than declaring it an internal authority.

## Use when

Use dictionary resolution when one source owner publishes a reusable, discoverable, localizable value set for multiple consumers. Runtime owns the `schema.dictionary` authoring contract; Metadata resolves and localizes the published dictionary and is not a second writer.

## Do not use when

Do not move one Object’s select options into Metadata without a reuse/ownership requirement. Do not encode workflow or scheduler policy as dictionary items.

## How to use

Author stable dictionary/item keys, lifecycle, and editor permissions through Runtime `schema.dictionary`. Use Metadata for locale/fallback-aware resolution and read permissions over the published revision.

## Adaptation cookbook

| Business requirement | Adapt with | Concrete implementation | Wrong adaptation |
| --- | --- | --- | --- |
| Country, cancellation reason, or shared classification is reused by several owners | Runtime `schema.dictionary` plus Metadata resolution | Publish one Runtime-owned catalog; business records store stable keys; Metadata serves display labels by locale | Copying values into each module, persisting translated labels as identity, or creating a second Metadata-owned definition |
| Order status drives transition rules inside the order lifecycle | Order-owned enum/state model, not a Metadata dictionary | Keep allowed transitions, invariants, and terminal behavior in the owning domain; Metadata may localize exposed labels only | Moving business state semantics into an editable shared dictionary |
| Administrators may add reference values without a code release | Runtime-authored dictionary with explicit permission and usage checks | Validate uniqueness and active/inactive lifecycle through the Runtime authoring contract; prevent destructive removal while referenced; audit publication; let Metadata resolve the published revision | Allowing arbitrary free-text values, deleting a referenced key, or treating Metadata as another authoring owner |
| A catalog is only used by one bounded Object and changes with its schema | Owner-local field definition | Keep the value set beside the Object unless cross-owner reuse and independent governance are real requirements | Selecting Metadata for every dropdown regardless of ownership |

## Example

Author Runtime dictionary `customer_tier` with stable keys `standard`, `gold`, and `enterprise`; Metadata resolves the published revision using requested locale and declared fallback. Disabling `gold` prevents new selection but still resolves the historical label for records already storing that key. One Object's lifecycle status stays a local select. A Provider-owned frequently changing catalog keeps its external identity and synchronization/mapping evidence in Integration rather than masquerading as an internally authoritative dictionary.

## Permissions and scope

Runtime dictionary administration and Metadata dictionary resolution are separate permissions. Source ownership prevents Metadata or an unrelated module from mutating the authoring truth.

## Boundaries

Dictionaries provide reusable values and labels, not executable rules or arbitrary dynamic schema. Metadata owns resolution/localization behavior only; the published definition remains Runtime-owned.
