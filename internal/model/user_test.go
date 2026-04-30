package model

import (
	"reflect"
	"testing"
)

func TestNormalizeAppPermissionsAllowsAliceAsExplicitPermission(t *testing.T) {
	got := NormalizeAppPermissions([]string{DefaultAppChat, DefaultAppAlice}, false, false)
	want := []string{DefaultAppChat, DefaultAppAlice}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit alice permission for regular user, got %#v", got)
	}
}

func TestDefaultAppPermissionsDoNotGrantAliceAutomatically(t *testing.T) {
	got := AllAppPermissions(true, false)
	for _, permission := range got {
		if permission == DefaultAppAlice {
			t.Fatalf("expected admin defaults not to include alice automatically, got %#v", got)
		}
	}
}

func TestNormalizeVisibilityGroupsDefaultsAndDeduplicates(t *testing.T) {
	got := NormalizeVisibilityGroups([]string{"", " General ", "team", "general", "Team"})
	want := []string{DefaultVisibilityGroup, "team"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected visibility groups: got %#v want %#v", got, want)
	}
}
