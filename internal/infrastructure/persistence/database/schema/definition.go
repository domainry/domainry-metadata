// Package schema owns the logical Metadata persistence schema. Concrete DDL is
// rendered through domainry-orm in database/metadata because the host supplies
// the active SQL dialect at module-open time.
package schema
