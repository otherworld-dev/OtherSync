package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/otherworld/nimbo/internal/account"
	"github.com/otherworld/nimbo/internal/config"
	"github.com/otherworld/nimbo/internal/shellns"
)

func TestSidebarGUIDFor(t *testing.T) {
	st := account.Store{Accounts: []account.Account{{ID: "first"}, {ID: "second"}}}
	if g := sidebarGUIDFor("first", st); g != shellns.NavGUID {
		t.Errorf("the first account keeps the original entry, got %s", g)
	}
	if g := sidebarGUIDFor("second", st); g == shellns.NavGUID || g != shellns.EntryGUID("second") {
		t.Errorf("a later account gets its own entry, got %s", g)
	}
}

func TestPlanSidebarEntries(t *testing.T) {
	adam := sidebarWant{ID: "adam", GUID: "{NAV}", Target: `E:\Nextcloud`, Name: "OtherSync - adam"}
	ow := sidebarWant{ID: "ow", GUID: "{OW}", Target: `E:\Otherworld Dev`, Name: "OtherSync - otherworld"}
	rec := func(w sidebarWant) config.SidebarEntry {
		return config.SidebarEntry{GUID: w.GUID, Target: w.Target, Name: w.Name}
	}
	ids := func(ws []sidebarWant) []string {
		var out []string
		for _, w := range ws {
			out = append(out, w.ID)
		}
		return out
	}
	cases := []struct {
		name                string
		want                []sidebarWant
		have                map[string]config.SidebarEntry
		write, remove, drop []string
	}{
		{"nothing recorded yet (upgrade): every account written", []sidebarWant{adam, ow}, nil, []string{"adam", "ow"}, nil, nil},
		{"all up to date: nothing to do", []sidebarWant{adam, ow}, map[string]config.SidebarEntry{"adam": rec(adam), "ow": rec(ow)}, nil, nil, nil},
		{"second account added: only it is written, the first is untouched", []sidebarWant{adam, ow}, map[string]config.SidebarEntry{"adam": rec(adam)}, []string{"ow"}, nil, nil},
		{"folder moved: rewritten in place", []sidebarWant{adam}, map[string]config.SidebarEntry{"adam": {GUID: "{NAV}", Target: `C:\old`, Name: adam.Name}}, []string{"adam"}, nil, nil},
		{"account gone: its entry removed", []sidebarWant{adam}, map[string]config.SidebarEntry{"adam": rec(adam), "ow": rec(ow)}, nil, []string{"{OW}"}, []string{"ow"}},
		{"first account gone: the next takes the original node, nothing removed under it",
			[]sidebarWant{{ID: "ow", GUID: "{NAV}", Target: ow.Target, Name: ow.Name}},
			map[string]config.SidebarEntry{"adam": rec(adam), "ow": rec(ow)},
			[]string{"ow"}, []string{"{OW}"}, []string{"adam"}},
	}
	for _, c := range cases {
		write, remove, drop := planSidebarEntries(c.want, c.have)
		sort.Strings(remove)
		sort.Strings(drop)
		if got := ids(write); !reflect.DeepEqual(got, c.write) {
			t.Errorf("%s: write %v, want %v", c.name, got, c.write)
		}
		if !reflect.DeepEqual(remove, c.remove) {
			t.Errorf("%s: remove %v, want %v", c.name, remove, c.remove)
		}
		if !reflect.DeepEqual(drop, c.drop) {
			t.Errorf("%s: drop %v, want %v", c.name, drop, c.drop)
		}
	}
}
