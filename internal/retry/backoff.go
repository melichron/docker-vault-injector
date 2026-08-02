// Package retry contains the small amount of retry timing shared by the
// Docker event watcher and Vault authentication. It deliberately does not
// decide which errors are retryable; callers retain that policy.
package retry

import (
	"math/rand/v2"
	"time"
)

const jitterFraction = 0.20

// Backoff grows exponentially from Initial to Max. Next adds up to 20 percent
// positive or negative jitter so several restarted controllers do not retry in
// lockstep. A successful operation should call Reset.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	current time.Duration
}

func NewBackoff(initial, maximum time.Duration) *Backoff {
	if maximum < initial {
		maximum = initial
	}
	return &Backoff{Initial: initial, Max: maximum}
}

func (b *Backoff) Next() time.Duration {
	base := b.current
	if base == 0 {
		base = b.Initial
	}

	if base >= b.Max/2 {
		b.current = b.Max
	} else {
		b.current = base * 2
	}

	jitterRange := int64(float64(base) * jitterFraction)
	if jitterRange <= 0 {
		return base
	}
	// Int64N is exclusive at the upper bound, hence +1. The result remains
	// close to the configured maximum while still varying capped retries.
	jitter := rand.Int64N(2*jitterRange+1) - jitterRange
	delay := base + time.Duration(jitter)
	if delay > b.Max {
		return b.Max
	}
	return delay
}

func (b *Backoff) Reset() {
	b.current = 0
}
