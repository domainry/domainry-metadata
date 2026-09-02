package metadata

import (
	"testing"

	actioncontract "github.com/domainry/domainry-foundation/action"
)

func TestAuthorizationActionsFreezeAsOneExactManifest(t *testing.T) {
	definitions, err := AuthorizationActions()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 7 {
		t.Fatalf("Action count=%d", len(definitions))
	}
	registry := actioncontract.NewRegistry()
	if err := registry.Register(definitions...); err != nil {
		t.Fatal(err)
	}
	if err := registry.Freeze(); err != nil {
		t.Fatal(err)
	}
	if permissions := registry.PermissionDefinitions(); len(permissions) != 6 {
		t.Fatalf("Permission count=%d", len(permissions))
	}
	for _, definition := range registry.Definitions() {
		if definition.Owner != AuthorizationOwner || definition.HTTP == nil {
			t.Fatalf("incomplete Metadata Action: %#v", definition)
		}
		if definition.Permission == nil {
			if definition.Authorization.Strategy != actioncontract.AuthorizationAuthenticatedPrincipal {
				t.Fatalf("unexpected Permission-free Action: %#v", definition)
			}
			continue
		}
		if definition.Authorization.Strategy != actioncontract.AuthorizationExactRolePermission || definition.Permission.Key != definition.Key || definition.Permission.Owner != definition.Owner {
			t.Fatalf("non-exact Metadata Action: %#v", definition)
		}
	}
}
