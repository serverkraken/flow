package main

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/clientmachine"
)

func TestValidateBindRef(t *testing.T) {
	cases := []struct {
		name    string
		in      bindNodeIn
		wantErr bool
	}{
		{"project only", bindNodeIn{Project: "alpha"}, false},
		{"create with parent", bindNodeIn{CreateName: "New", CreateParent: "privat"}, false},
		{"create without parent", bindNodeIn{CreateName: "New"}, true}, // repo needs an engagement/vorhaben parent
		{"both", bindNodeIn{Project: "alpha", CreateName: "New"}, true},
		{"neither", bindNodeIn{}, true},
		{"whitespace neither", bindNodeIn{Project: "  ", CreateName: " "}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateBindRef(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateBindRef(%#v) err=%v, wantErr=%v", c.in, err, c.wantErr)
			}
		})
	}
}

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

func TestBindNodeCore_CreatePathRejectsMissingMachineBeforeAnyAPIWrite(t *testing.T) {
	h := &handlers{}
	_, _, err := h.bindNodeCore(
		context.Background(), nil,
		bindNodeIn{CreateName: "Flow", CreateParent: "work", Kind: "path"},
		"", false, clientmachine.Machine{}, "/work/flow",
	)
	if err == nil || !strings.Contains(err.Error(), "machine id") {
		t.Fatalf("want missing machine-id guard, got %v", err)
	}
}
