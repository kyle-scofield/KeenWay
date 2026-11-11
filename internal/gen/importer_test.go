package gen

import (
	"os"
	"strings"
	"testing"
)

// Importing the user's live config and generating again must give the same routing.
func TestImportRoundTrip(t *testing.T) {
	ref, err := os.ReadFile("../../reference/rockblack-simple-subscription.yaml")
	if err != nil {
		t.Skip("reference config not available")
	}
	s, warnings, err := Import(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(s.Subscriptions) != 1 || s.Subscriptions[0].Headers["x-hwid"] == "" || s.Subscriptions[0].Interval != 3600 {
		t.Errorf("subscription: %+v", s.Subscriptions)
	}
	if strings.Join(s.Services, ",") != strings.Join(DefaultServiceIDs(), ",") || !s.BlockAds || s.Mode != "simple" {
		t.Errorf("services=%v ads=%v mode=%s", s.Services, s.BlockAds, s.Mode)
	}
	if len(s.Sites) != 2 || len(s.Sites[0].Domains) != 10 || len(s.Sites[0].Ports) != 1 || s.Sites[1].Target != "geoblock" {
		t.Errorf("sites: %+v", s.Sites)
	}

	out, err := Build(s)
	if err != nil {
		t.Fatal(err)
	}
	want, got := load(t, ref), load(t, out)
	a, b := sorted(want.Rules), sorted(got.Rules)
	if missing := diff(diff(a, b), droppedTemplateRules); len(missing) > 0 {
		t.Errorf("round trip lost rules: %v", missing)
	}
	// Only lists the catalog added on top of the reference may appear.
	for _, r := range diff(b, a) {
		if !strings.HasPrefix(r, "RULE-SET,gs-") && !strings.HasPrefix(r, "RULE-SET,telegram_extra,") {
			t.Errorf("unexpected rule after round trip: %s", r)
		}
	}
	if len(want.ProxyGroups) != len(got.ProxyGroups) {
		t.Errorf("groups: %d vs %d", len(want.ProxyGroups), len(got.ProxyGroups))
	}
}

func TestTidySiteNames(t *testing.T) {
	s := Settings{Sites: []CustomSite{
		{Name: "Мои: Заблокировано в РФ", Policy: PolicyGroup},
		{Name: "Мои: Недоступно из РФ", Target: "geoblock"},
		{Name: "Мои сайты через VPN", Policy: PolicyVPN},
		{Name: "Кино"},
	}}
	if !TidySiteNames(&s) {
		t.Fatal("expected changes")
	}
	got := []string{s.Sites[0].Name, s.Sites[1].Name, s.Sites[2].Name, s.Sites[3].Name}
	want := []string{"Сайты через VPN", "Зарубежные сайты", "Сайты через VPN 2", "Кино"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("site %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if TidySiteNames(&s) {
		t.Error("second run should change nothing")
	}
}
