package api

import "testing"

func TestPromptHas(t *testing.T) {
	if !promptHas("login consent", "login") {
		t.Fatal("login prompt not detected")
	}
	if promptHas("login consent", "none") {
		t.Fatal("none prompt should not be present")
	}
	if !promptHas("none", "none") {
		t.Fatal("none prompt not detected")
	}
}
