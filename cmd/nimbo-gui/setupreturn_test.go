package main

import "testing"

// Adding an account keeps the main one: the account to show again is handed
// back once, so a second end of the same setup (Connect, then the window
// closing) does not switch twice.
func TestSetupReturnTakenOnce(t *testing.T) {
	a := &App{}
	if got := a.takeSetupReturn("new"); got != "" {
		t.Fatalf("nothing held, got %q", got)
	}
	a.holdSetupReturn("new", "main")
	if got := a.takeSetupReturn("new"); got != "main" {
		t.Fatalf("got %q, want main", got)
	}
	if got := a.takeSetupReturn("new"); got != "" {
		t.Fatalf("taken twice: %q", got)
	}
}
