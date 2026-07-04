package cable

import (
	"math/rand/v2"
	"time"
)

const (
	backoffBase = 500 * time.Millisecond
	backoffCap  = 30 * time.Second
)

// backoff returns the reconnect delay for the given attempt: exponential
// from 500ms capped at 30s, with ±20% jitter so a flapping server doesn't
// see synchronized retries.
func backoff(attempt int) time.Duration {
	d := backoffBase << min(attempt, 10)
	if d > backoffCap {
		d = backoffCap
	}
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(d) * jitter)
}
