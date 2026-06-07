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

func TestDefaultAppPermissionsDoNotGrantDrawingAutomatically(t *testing.T) {
	got := AllAppPermissions(true, false)
	for _, permission := range got {
		if permission == DefaultAppDrawing {
			t.Fatalf("expected admin defaults not to include drawing automatically, got %#v", got)
		}
	}
}

func TestNormalizeAppPermissionsAllowsDrawingAsExplicitPermission(t *testing.T) {
	got := NormalizeAppPermissions([]string{DefaultAppChat, DefaultAppDrawing}, false, false)
	want := []string{DefaultAppChat, DefaultAppDrawing}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected explicit drawing permission for regular user, got %#v", got)
	}
}

func TestProxyPermissionIsSuperAdminOnly(t *testing.T) {
	got := NormalizeAppPermissions([]string{DefaultAppChat, DefaultAppProxy}, false, false)
	want := []string{DefaultAppChat}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected regular user not to receive proxy permission, got %#v", got)
	}

	superAdmin := NormalizeAppPermissions([]string{DefaultAppProxy}, true, true)
	if !reflect.DeepEqual(superAdmin, []string{DefaultAppProxy}) {
		t.Fatalf("expected super admin proxy permission, got %#v", superAdmin)
	}
}

func TestNormalizeVisibilityGroupsDefaultsAndDeduplicates(t *testing.T) {
	got := NormalizeVisibilityGroups([]string{"", " General ", "team", "general", "Team"})
	want := []string{DefaultVisibilityGroup, "team"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected visibility groups: got %#v want %#v", got, want)
	}
}
