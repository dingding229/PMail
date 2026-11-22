// Package isotime is the additive API representation of stored timestamps.
package isotime

import "time"

// UTC preserves the instant and writes an explicit RFC3339 UTC offset. An
// unset timestamp is null, not a spurious timestamp in year one.
func UTC(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	v := t.UTC()
	return &v
}

func Optional(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	return UTC(*t)
}
