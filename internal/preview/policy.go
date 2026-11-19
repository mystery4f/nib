// Package preview decides when the rendered document may be rebuilt from the
// source. Rebuilding clears the rendered pane's selection, so it happens on a
// pause in typing rather than on every keystroke.
package preview

import "time"

// Debounce is how long the source has to stay untouched before the preview
// catches up with it.
const Debounce = 250 * time.Millisecond

// Stale reports whether the rendered copy is behind the source. forced is set
// when the document was just saved, which must show up immediately.
func Stale(edited, rendered uint64, idle time.Duration, forced bool) bool {
	if edited == rendered {
		return false
	}
	return forced || idle >= Debounce
}
