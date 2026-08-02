package retry

import (
	"testing"
	"time"
)

func TestBackoffGrowsToMaximumAndResets(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 400*time.Millisecond)

	assertWithinJitter(t, backoff.Next(), 100*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 200*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 400*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 400*time.Millisecond)

	backoff.Reset()
	assertWithinJitter(t, backoff.Next(), 100*time.Millisecond)
}

func TestBackoffRaisesMaximumBelowInitial(t *testing.T) {
	backoff := NewBackoff(time.Second, 100*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), time.Second)
	assertWithinJitter(t, backoff.Next(), time.Second)
}

func assertWithinJitter(t *testing.T, actual, base time.Duration) {
	t.Helper()
	minimum := time.Duration(float64(base) * (1 - jitterFraction))
	maximum := time.Duration(float64(base) * (1 + jitterFraction))
	if actual < minimum || actual > maximum {
		t.Fatalf("delay %s is outside [%s, %s] for base %s", actual, minimum, maximum, base)
	}
}
