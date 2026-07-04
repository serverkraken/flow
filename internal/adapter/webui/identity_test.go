package webui

import "testing"

func TestInitials(t *testing.T) {
	cases := map[string]string{
		"backstage":                        "BA",
		"RTL Extern":                       "RE",
		"flow":                             "FL",
		"kickstart-aws-infra":              "KI",
		"gitlab.com/dataalliance/x/y/cmdb": "CM",
		"":                                 "?",
	}
	for in, want := range cases {
		if got := Initials(ShortName(in)); got != want {
			t.Errorf("Initials(ShortName(%q)) = %q, want %q", in, got, want)
		}
	}
}

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"gitlab.com/dataalliance/products/foolu/product/backstage": "backstage",
		"RTL Extern": "RTL Extern",
		"a/b/":       "b",
		"":           "",
	}
	for in, want := range cases {
		if got := ShortName(in); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAvatarTone_DeterministicAndSpread(t *testing.T) {
	if AvatarTone("backstage") != AvatarTone("backstage") {
		t.Fatal("tone not deterministic")
	}
	seen := map[string]bool{}
	for _, n := range []string{"backstage", "flow", "RTL Extern", "cmdb", "infra", "skopeo", "tf-modules", "k8s-infra"} {
		seen[AvatarTone(n)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("tones not spread: %v", seen)
	}
	for tone := range seen {
		if len(tone) != 4 || tone[:3] != "av-" || tone[3] < 'a' || tone[3] > 'f' {
			t.Fatalf("bad tone %q", tone)
		}
	}
}
