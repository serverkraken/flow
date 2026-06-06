package sse

// metrics_test.go — Plan F · Task 6.
//
// Verifies the flow_sse_subscribers gauge is incremented on Subscribe
// and decremented on cancel — and is idempotent on double-cancel so a
// "defer cancel() + explicit cancel()" pattern doesn't push the gauge
// negative.

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// gaugeValue reads the current value of a no-label gauge from the
// default registry. Returns 0 if the metric hasn't been registered yet.
func gaugeValue(t *testing.T, name string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

// TestUnit_SSESubscribers_GaugeTracksLifecycle subscribes/cancels and
// asserts the gauge moves in lockstep. Uses delta math so the test is
// robust to other tests in the same package having moved the gauge.
func TestUnit_SSESubscribers_GaugeTracksLifecycle(t *testing.T) {
	b := New()

	before := gaugeValue(t, "flow_sse_subscribers")

	_, cancel1 := b.Subscribe("u1")
	mid1 := gaugeValue(t, "flow_sse_subscribers")
	if mid1 != before+1 {
		t.Fatalf("after 1 subscribe: gauge = %v, want %v", mid1, before+1)
	}

	_, cancel2 := b.Subscribe("u2")
	mid2 := gaugeValue(t, "flow_sse_subscribers")
	if mid2 != before+2 {
		t.Fatalf("after 2 subscribes: gauge = %v, want %v", mid2, before+2)
	}

	cancel1()
	after1 := gaugeValue(t, "flow_sse_subscribers")
	if after1 != before+1 {
		t.Errorf("after 1 cancel: gauge = %v, want %v", after1, before+1)
	}

	cancel2()
	after2 := gaugeValue(t, "flow_sse_subscribers")
	if after2 != before {
		t.Errorf("after 2 cancels: gauge = %v, want %v", after2, before)
	}
}

// TestUnit_SSESubscribers_GaugeIdempotentOnDoubleCancel guards the
// invariant that calling cancel twice — common with `defer cancel()`
// plus explicit early exit — does NOT push the gauge below zero.
func TestUnit_SSESubscribers_GaugeIdempotentOnDoubleCancel(t *testing.T) {
	b := New()

	before := gaugeValue(t, "flow_sse_subscribers")

	_, cancel := b.Subscribe("u")
	cancel()
	cancel() // double — must be a no-op for the gauge

	after := gaugeValue(t, "flow_sse_subscribers")
	if after != before {
		t.Errorf("double-cancel left gauge at %v, want %v", after, before)
	}
}
