package main

import (
	"testing"

	"github.com/otherworld/nimbo/internal/brand"
	"github.com/otherworld/nimbo/internal/policy"
)

// A partner build's baked-in server lock applies with no licence or registry
// policy; an admin policy's own address wins; the stock build changes nothing
// (Deck d1010).
func TestWithBrandServer(t *testing.T) {
	acme := brand.Brand{ServerURL: "https://cloud.acme.example", LockServer: true}

	p := withBrandServer(policy.Policy{AllowSignOut: true}, acme)
	if !p.LockServer || p.ServerURL != "https://cloud.acme.example" || !p.AllowSignOut {
		t.Errorf("partner lock not applied: %+v", p)
	}

	p = withBrandServer(policy.Policy{ServerURL: "https://admin.example"}, acme)
	if p.ServerURL != "https://admin.example" || !p.LockServer {
		t.Errorf("admin address should win, lock should hold: %+v", p)
	}

	p = withBrandServer(policy.Policy{AllowSignOut: true}, brand.Brand{ServerURL: "https://preset.example"})
	if p.ServerURL != "https://preset.example" || p.LockServer {
		t.Errorf("a preset without lockServer must not lock: %+v", p)
	}

	in := policy.Policy{AllowSignOut: true}
	if p := withBrandServer(in, brand.Brand{Name: "OtherSync"}); p != in {
		t.Errorf("stock brand changed the policy: %+v", p)
	}
}
