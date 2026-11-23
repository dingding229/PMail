// Package intbounds converts protocol and persisted integers without wrapping.
package intbounds

import "math"

func Uint32(n int64) (uint32, bool) {
	if n < 0 || n > math.MaxUint32 {
		return 0, false
	}
	return uint32(n), true
}

func Int64(n uint64) (int64, bool) {
	if n > math.MaxInt64 {
		return 0, false
	}
	return int64(n), true
}

// SliceIndex validates a uint64 offset against both the native integer range
// and the available bytes, including a legal offset at the end of the buffer.
func SliceIndex(n uint64, available int) (int, bool) {
	if n > math.MaxInt {
		return 0, false
	}
	i := int(n)
	return i, i <= available
}
