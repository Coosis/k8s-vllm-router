package ewma

import "testing"

func TestObserveStartsFromZero(t *testing.T) {
	v := NewWithAlpha(0.5)

	v.Observe(100)

	if got := v.Get(); got != 50 {
		t.Fatalf("ewma = %v, want 50", got)
	}
}

func TestInvalidAlphaUsesDefault(t *testing.T) {
	v := NewWithAlpha(2)

	v.Observe(100)

	if got := v.Get(); got != 5 {
		t.Fatalf("ewma = %v, want 5", got)
	}
}
