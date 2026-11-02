package service

import (
	"testing"
)

func TestSplitScopes(t *testing.T) {
	got := SplitScopes("openid profile  email openid")
	want := []string{"openid", "profile", "email"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(SplitScopes("   ")) != 0 {
		t.Fatal("blank scope should be empty")
	}
}

func TestScopeSubset(t *testing.T) {
	if !ScopeSubset([]string{"openid", "email"}, []string{"openid", "profile", "email"}) {
		t.Fatal("expected subset")
	}
	if ScopeSubset([]string{"admin"}, []string{"openid", "profile"}) {
		t.Fatal("unexpected subset")
	}
	if !ScopeSubset(nil, []string{"openid"}) {
		t.Fatal("empty scope should be a subset")
	}
}
