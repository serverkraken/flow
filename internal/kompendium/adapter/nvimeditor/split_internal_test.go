package nvimeditor

import (
	"reflect"
	"testing"
)

func TestSplitCommand_BasicCases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "vi", []string{"vi"}},
		{"flags", "code -w --foo", []string{"code", "-w", "--foo"}},
		{"single quote", `'a b' c`, []string{"a b", "c"}},
		{"double quote", `"a b" c`, []string{"a b", "c"}},
		{"escaped space", `a\ b c`, []string{"a b", "c"}},
		{"double quote with escape", `"a\"b" c`, []string{`a"b`, "c"}},
		{"tabs and spaces collapse", "vi\t -w  /x", []string{"vi", "-w", "/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitCommand(tc.in)
			if err != nil {
				t.Fatalf("splitCommand(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSplitCommand_UnterminatedQuote(t *testing.T) {
	if _, err := splitCommand(`'never closes`); err == nil {
		t.Error("expected error for unterminated single quote")
	}
	if _, err := splitCommand(`"never closes`); err == nil {
		t.Error("expected error for unterminated double quote")
	}
}
