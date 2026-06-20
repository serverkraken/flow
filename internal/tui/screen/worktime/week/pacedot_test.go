package week

import (
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/tui/ui/glyphs"
)

func TestClassifyPaceDot(t *testing.T) {
	off := &apiclient.DayOff{Kind: "vacation"}
	if got := classifyPaceDot(apiclient.WeekDay{LoggedMin: 0}, off); got != paceDotDayOff {
		t.Fatalf("dayoff present → DayOff, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 480}, nil); got != paceDotHit {
		t.Fatalf("logged>=target → Hit, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 60, IsToday: true}, nil); got != paceDotRunning {
		t.Fatalf("today, not yet hit → Running, got %v", got)
	}
	if got := classifyPaceDot(apiclient.WeekDay{TargetMin: 480, LoggedMin: 60}, nil); got != paceDotMissed {
		t.Fatalf("past open workday → Missed, got %v", got)
	}
}

func TestPaceGlyph(t *testing.T) {
	if paceGlyph(paceDotMissed) != glyphs.Empty {
		t.Fatal("Missed must use ○ (glyphs.Empty)")
	}
	for _, k := range []paceDotKind{paceDotHit, paceDotRunning, paceDotDayOff} {
		if paceGlyph(k) != glyphs.Filled {
			t.Fatalf("kind %v must use ● (glyphs.Filled)", k)
		}
	}
}
