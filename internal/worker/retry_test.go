package worker_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/b2randon/webhook-delivery/internal/worker"
)

func TestNextAttemptAt(t *testing.T) {
	tests := []struct {
		done    int
		wantNil bool
		wantMin time.Duration
		wantMax time.Duration
	}{
		{1, false, 9 * time.Second, 11 * time.Second},
		{2, false, 29 * time.Second, 31 * time.Second},
		{3, false, 119 * time.Second, 121 * time.Second},
		{4, false, 599 * time.Second, 601 * time.Second},
		{5, true, 0, 0},
		{6, true, 0, 0},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("after_%d_failures", tc.done), func(t *testing.T) {
			before := time.Now()
			got := worker.NextAttemptAt(tc.done)
			if tc.wantNil {
				if got != nil {
					t.Errorf("want nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("want non-nil, got nil")
			}
			delta := got.Sub(before)
			if delta < tc.wantMin || delta > tc.wantMax {
				t.Errorf("delta = %v, want [%v, %v]", delta, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestMaxAttempts(t *testing.T) {
	if worker.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d, want 5", worker.MaxAttempts)
	}
}
