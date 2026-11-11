package gen

import (
	"strings"
	"testing"
)

func TestEveryServiceBuildsAlone(t *testing.T) {
	base := Settings{Mode: "simple", Subscriptions: []Subscription{{Name: "Sub", URL: "https://example.com/s"}}}
	prev, err := Build(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, svc := range Services {
		s := base
		s.Services = []string{svc.ID}
		out, err := Build(s)
		if err != nil {
			t.Errorf("%s (%s): %v", svc.ID, svc.Name, err)
			continue
		}
		if string(out) == string(prev) {
			t.Errorf("%s: enabling it does not change the config", svc.ID)
		}
	}
}

func TestUnnamedSiteListIsRepaired(t *testing.T) {
	s := Settings{Mode: "simple", Subscriptions: []Subscription{{Name: "Sub", URL: "https://example.com/s"}},
		Sites: []CustomSite{{Name: "", Domains: []string{"kino.watch"}, Policy: "group"}, {Name: "Свои сайты", Domains: []string{"a.com"}, Policy: "vpn"}}}
	TidySiteNames(&s)
	if s.Sites[0].Name == "" || s.Sites[0].Name == s.Sites[1].Name {
		t.Fatalf("names: %q %q", s.Sites[0].Name, s.Sites[1].Name)
	}
	if _, err := Build(s); err != nil {
		t.Fatal(err)
	}
}

// XKeen must run in Hybrid mode: with TPROXY alone HTTPS from the home network fails on KeeneticOS 5.
func TestHybridInbounds(t *testing.T) {
	out, err := Build(Settings{Mode: "simple", Subscriptions: []Subscription{{Name: "Sub", URL: "https://example.com/s"}}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(out)
	for _, want := range []string{"type: tproxy", "port: 1181", "type: redir", "port: 1182"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config lacks %q", want)
		}
	}
}
