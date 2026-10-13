package character

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestCatalogLicensedCompleteStarter(t *testing.T) {
	if len(Catalog()) != 20 {
		t.Fatal("20 assets required")
	}
	seen := map[string]bool{}
	starters := map[string]bool{}
	for _, i := range Catalog() {
		if seen[i.ID] || !validSlot(i.Slot) {
			t.Fatal(i)
		}
		seen[i.ID] = true
		if i.Starter {
			if starters[i.Slot] {
				t.Fatal("duplicate starter")
			}
			starters[i.Slot] = true
		}
		b, e := Assets.ReadFile("assets/" + i.ID + ".svg")
		if e != nil {
			t.Fatal(e)
		}
		var doc struct{ XMLName xml.Name }
		if e = xml.Unmarshal(b, &doc); e != nil || doc.XMLName.Local != "svg" {
			t.Fatal(i.ID, e)
		}
		if strings.Contains(string(b), "script") || strings.Contains(string(b), "href") {
			t.Fatal("nonlocal SVG")
		}
	}
	if len(starters) != 6 {
		t.Fatal("incomplete starter")
	}
}
