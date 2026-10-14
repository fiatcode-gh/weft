package views

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestActionTableShape(t *testing.T) {
	seen := map[string]int{}
	var sidebarKeys []string
	for i, act := range readActions {
		if act.sidebar {
			if len(act.keys) == 0 {
				t.Errorf("entry %d (%q) is sidebar-flagged without keys", i, act.key)
			}
			sidebarKeys = append(sidebarKeys, act.keys...)
		}
		for _, k := range act.keys {
			if j, dup := seen[k]; dup {
				t.Errorf("key %q is in entries %d and %d", k, j, i)
			}
			seen[k] = i
		}
		if len(act.keys) == 0 {
			if act.run != nil || act.palette || act.key == "" {
				t.Errorf("help-only entry %d (%q): want nil run, !palette, non-empty key", i, act.key)
			}
		} else if act.run == nil {
			t.Errorf("entry %d (%q) has keys but no run", i, act.key)
		}
		if act.key == "" {
			if i == 0 || readActions[i-1].group != act.group || readActions[i-1].key == "" {
				t.Errorf("entry %d has no help row of its own and no row above it in its group to share", i)
			}
			if act.palette {
				t.Errorf("entry %d shares the help row above and must not be palette-listed", i)
			}
		}
	}
	if want := "[1 2 3 4 5 |]"; fmt.Sprint(sidebarKeys) != want {
		t.Errorf("sidebar-flagged keys %v, want %s", sidebarKeys, want)
	}
}

func TestActionsHaveHelpRows(t *testing.T) {
	type groupedRow struct {
		group string
		row   helpRow
	}
	var got, want []groupedRow
	for _, g := range helpGroups {
		if g.title == "Editor" || g.title == "Find" {
			continue
		}
		for _, r := range g.rows {
			got = append(got, groupedRow{g.title, r})
		}
	}
	for _, act := range readActions {
		if act.key != "" {
			want = append(want, groupedRow{act.group, helpRow{act.key, act.title}})
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("help rows differ from the action table\n got: %v\nwant: %v", got, want)
	}
}

// probeKeys is every key press the read view could be handed that the tests
// care about: printable ASCII, the named keys and ctrl-letters.
func probeKeys() []tea.KeyPressMsg {
	var probes []tea.KeyPressMsg
	for r := rune(0x21); r <= 0x7e; r++ {
		probes = append(probes, key(string(r)))
	}
	named := []rune{
		tea.KeyEnter, tea.KeyTab, tea.KeySpace, tea.KeyEscape, tea.KeyBackspace,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd,
		tea.KeyPgUp, tea.KeyPgDown, tea.KeyDelete, tea.KeyInsert,
	}
	for c := tea.KeyF1; c <= tea.KeyF12; c++ {
		named = append(named, c)
	}
	for _, c := range named {
		probes = append(probes, tea.KeyPressMsg{Code: c})
	}
	probes = append(probes, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	for c := 'a'; c <= 'z'; c++ {
		probes = append(probes, tea.KeyPressMsg{Code: c, Mod: tea.ModCtrl})
	}
	return probes
}

func TestReadViewKeysOnlyThroughTable(t *testing.T) {
	probes := probeKeys()

	covered := map[string]bool{}
	for _, p := range probes {
		covered[p.String()] = true
	}
	for _, act := range readActions {
		for _, k := range act.keys {
			if !covered[k] {
				t.Errorf("table key %q is not in the probe set", k)
			}
		}
	}

	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	alpha := bootApp(t, bootConfig{now: now})
	alpha.navigate("Alpha")
	alpha.Update(key("n"))
	journal := bootApp(t, bootConfig{now: now})
	journal.navigate("2026_05_24")

	saved := readKeys
	readKeys = map[string]*action{}
	t.Cleanup(func() { readKeys = saved })

	for name, a := range map[string]*App{"Alpha": alpha, "journal": journal} {
		for _, p := range probes {
			before := appText(a)
			hist, idx, page := len(a.hist), a.histIdx, a.page.Page()
			_, cmd := a.Update(p)
			if cmd != nil {
				t.Errorf("%s: %q returned a cmd outside the table", name, p.String())
			}
			if a.active != nil || a.capture != nil || a.rename != nil || a.editor != nil {
				t.Errorf("%s: %q opened a view outside the table", name, p.String())
			}
			if len(a.hist) != hist || a.histIdx != idx || a.page.Page() != page {
				t.Errorf("%s: %q navigated outside the table", name, p.String())
			}
			if after := appText(a); after != before {
				t.Errorf("%s: %q changed the frame outside the table", name, p.String())
			}
		}
	}
}
