package main

import (
	"testing"
)

func TestDecideBindKind(t *testing.T) {
	cases := []struct {
		name     string
		override string
		originOK bool
		want     string
		wantErr  bool
	}{
		{"auto with origin", "", true, "remote", false},
		{"auto without origin", "", false, "path", false},
		{"force remote with origin", "remote", true, "remote", false},
		{"force remote without origin", "remote", false, "", true},
		{"force path", "path", false, "path", false},
		{"force path even with origin", "path", true, "path", false},
		{"invalid", "bogus", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decideBindKind(c.override, c.originOK)
			if (err != nil) != c.wantErr {
				t.Fatalf("decideBindKind(%q,%v) err=%v wantErr=%v", c.override, c.originOK, err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("decideBindKind(%q,%v)=%q want %q", c.override, c.originOK, got, c.want)
			}
		})
	}
}
