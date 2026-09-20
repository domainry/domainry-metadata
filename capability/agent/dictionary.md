# When should reusable values use Metadata dictionaries?

## Problems solved

- Governs shared, localizable reference values once instead of duplicating the same catalog and ordering rules in multiple domains.

## Business scenarios

- Reusing a country, reason-code, or shared classification catalog across several modules.
- Publishing centrally ordered and localized values whose lifecycle is independent of one business Object.

## Use when

Use a dictionary when one source owner publishes a reusable, discoverable, localizable value set for multiple consumers.

## Do not use when

Do not move one Object’s select options into Metadata without a reuse/ownership requirement. Do not encode workflow or scheduler policy as dictionary items.

## How to use

Choose one source owner, stable dictionary/item keys, localized labels, lifecycle, and exact reader/editor permissions.

## Adaptation cookbook

| Business requirement | Adapt with | Concrete implementation | Wrong adaptation |
| --- | --- | --- | --- |
| Country, cancellation reason, or shared classification is reused by several owners | Metadata dictionary with stable keys, lifecycle state, ordering, and localization | Publish one governed catalog; business records store stable keys; Metadata serves display labels by locale | Copying the same values into each module or persisting translated labels as business identity |
| Order status drives transition rules inside the order lifecycle | Order-owned enum/state model, not a Metadata dictionary | Keep allowed transitions, invariants, and terminal behavior in the owning domain; Metadata may localize exposed labels only | Moving business state semantics into an editable shared dictionary |
| Administrators may add reference values without a code release | Governed mutable dictionary with explicit permission and usage checks | Validate uniqueness and active/inactive lifecycle; prevent destructive removal while referenced; audit publication | Allowing arbitrary free-text values or deleting a key still stored by business records |
| A catalog is only used by one bounded Object and changes with its schema | Owner-local field definition | Keep the value set beside the Object unless cross-owner reuse and independent governance are real requirements | Selecting Metadata for every dropdown regardless of ownership |

## Example

A shared country-code catalog used by several modules can be a dictionary. Order status values with order-specific transitions remain in the order domain.

## Permissions and scope

Dictionary read and administration are separate. Source ownership prevents unrelated modules from mutating another owner’s catalog.

## Boundaries

Dictionaries provide reusable values and labels, not executable rules or arbitrary dynamic schema.
