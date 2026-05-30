// White-box tests for the worktime menu's output-target sub-picker.

package worktime

import (
	"strings"
	"testing"
)

func TestTargetPicker_DefaultsToSplit(t *testing.T) {
	tp := newTargetPicker("less -S")
	if tp.cursor != int(outputTargetSplit) {
		t.Errorf("cursor = %d, want %d (split)", tp.cursor, outputTargetSplit)
	}
}

func TestTargetPicker_NavigationWraps(t *testing.T) {
	tp := newTargetPicker("less -S")
	tp, _ = tp.handleKey(keyName("j"))
	if tp.cursor != int(outputTargetClipboard) {
		t.Errorf("after j, cursor = %d, want %d (clipboard)", tp.cursor, outputTargetClipboard)
	}
	tp, _ = tp.handleKey(keyName("j"))
	tp, _ = tp.handleKey(keyName("j"))
	// Wrapped — should be back at split (cursor 0)
	if tp.cursor != int(outputTargetSplit) {
		t.Errorf("after 3× j (wrap), cursor = %d, want %d", tp.cursor, outputTargetSplit)
	}
	// k from 0 wraps to last (file)
	tp, _ = tp.handleKey(keyName("k"))
	if tp.cursor != int(outputTargetFile) {
		t.Errorf("after k from split, cursor = %d, want %d (file)", tp.cursor, outputTargetFile)
	}
}

func TestTargetPicker_GAndShiftGJump(t *testing.T) {
	tp := newTargetPicker("less -S")
	tp, _ = tp.handleKey(runeKey('G'))
	if tp.cursor != targetCount-1 {
		t.Errorf("G should jump to last, got %d", tp.cursor)
	}
	tp, _ = tp.handleKey(runeKey('g'))
	if tp.cursor != 0 {
		t.Errorf("g should jump to first, got %d", tp.cursor)
	}
}

func TestTargetPicker_HotkeysPickDirectly(t *testing.T) {
	tests := []struct {
		key   rune
		want  outputTarget
		label string
	}{
		{'c', outputTargetClipboard, "clipboard"},
		{'s', outputTargetSplit, "split"},
		{'f', outputTargetFile, "file"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			tp := newTargetPicker("less -S")
			_, ev := tp.handleKey(runeKey(tc.key))
			if !ev.picked {
				t.Errorf("%c should pick a target", tc.key)
			}
			if ev.target != tc.want {
				t.Errorf("%c picked %d, want %d", tc.key, ev.target, tc.want)
			}
		})
	}
}

func TestTargetPicker_EnterPicksAtCursor(t *testing.T) {
	tp := newTargetPicker("less -S")
	tp, _ = tp.handleKey(keyName("j")) // → clipboard
	_, ev := tp.handleKey(keyName("enter"))
	if !ev.picked {
		t.Fatal("Enter must pick a target")
	}
	if ev.target != outputTargetClipboard {
		t.Errorf("Enter at clipboard cursor should pick clipboard, got %d", ev.target)
	}
}

func TestTargetPicker_EscCancels(t *testing.T) {
	tp := newTargetPicker("less -S")
	_, ev := tp.handleKey(keyName("esc"))
	if !ev.canceled {
		t.Error("Esc should set canceled")
	}
	if ev.picked {
		t.Error("Esc must not pick a target")
	}
}

func TestTargetPicker_NavOnlyDoesNotPick(t *testing.T) {
	tp := newTargetPicker("less -S")
	_, ev := tp.handleKey(keyName("j"))
	if ev.picked || ev.canceled {
		t.Errorf("nav-only key should produce a passthrough event, got %+v", ev)
	}
}

func TestTargetPicker_ViewRendersAllThreeTargets(t *testing.T) {
	tp := newTargetPicker("less -S")
	out := tp.view("Brief Wochenbericht", pal(), 80)
	for _, want := range []string{
		"Brief Wochenbericht",
		"tmux-Split",
		"Zwischenablage",
		"~/Downloads",
		"less -S",
		"pbcopy",
		"c · s · f",
		"Esc → zurück",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("target picker view missing %q in:\n%s", want, out)
		}
	}
}

func TestOutputTarget_HotkeyAndLabel(t *testing.T) {
	cases := []struct {
		t     outputTarget
		hk    string
		label string
	}{
		{outputTargetSplit, "s", "tmux-Split"},
		{outputTargetClipboard, "c", "Zwischenablage"},
		{outputTargetFile, "f", "Datei in ~/Downloads"},
	}
	for _, c := range cases {
		if got := c.t.hotkey(); got != c.hk {
			t.Errorf("hotkey(%d) = %q, want %q", c.t, got, c.hk)
		}
		if got := c.t.label(); got != c.label {
			t.Errorf("label(%d) = %q, want %q", c.t, got, c.label)
		}
	}
}

func TestOutputTarget_HintShowsViewerForSplit(t *testing.T) {
	if got := outputTargetSplit.hint("less -S"); got != "less -S" {
		t.Errorf("split hint with viewer=less -S = %q, want less -S", got)
	}
	if got := outputTargetClipboard.hint("any"); got != "pbcopy" {
		t.Errorf("clipboard hint = %q, want pbcopy", got)
	}
	if got := outputTargetFile.hint("any"); got != "" {
		t.Errorf("file hint = %q, want empty", got)
	}
}
