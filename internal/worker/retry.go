package worker

import "time"

// MaxAttempts is the maximum number of delivery attempts before marking terminal failure.
const MaxAttempts = 5

// backoffDelays defines the wait before each retry (index 0 = after first failure).
var backoffDelays = []time.Duration{
	10 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
}

// NextAttemptAt returns the scheduled time of the next retry after attemptsDone failures,
// or nil when MaxAttempts is reached (delivery is terminal).
func NextAttemptAt(attemptsDone int) *time.Time {
	if attemptsDone >= MaxAttempts {
		return nil
	}
	idx := attemptsDone - 1
	if idx < 0 {
		idx = 0
	}
	t := time.Now().Add(backoffDelays[idx])
	return &t
}
