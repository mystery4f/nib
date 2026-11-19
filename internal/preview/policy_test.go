package preview_test

import (
	"testing"
	"time"

	"github.com/mystery4f/nib/internal/preview"
)

func TestStale(t *testing.T) {
	cases := []struct {
		name     string
		edited   uint64
		rendered uint64
		idle     time.Duration
		forced   bool
		want     bool
	}{
		{name: "same version", edited: 4, rendered: 4, idle: time.Hour, want: false},
		{name: "same version forced", edited: 4, rendered: 4, idle: time.Hour, forced: true, want: false},
		{name: "below the debounce", edited: 5, rendered: 4, idle: preview.Debounce / 2, want: false},
		{name: "after the debounce", edited: 5, rendered: 4, idle: preview.Debounce, want: true},
		{name: "forced after a save", edited: 5, rendered: 4, idle: 0, forced: true, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := preview.Stale(c.edited, c.rendered, c.idle, c.forced); got != c.want {
				t.Errorf("Stale(%d, %d, %v, %v) = %v, want %v",
					c.edited, c.rendered, c.idle, c.forced, got, c.want)
			}
		})
	}
}
