package intbounds

import (
	"math"
	"testing"
)

func TestCheckedProtocolConversions(t *testing.T) {
	for _, n := range []int64{-1, math.MaxUint32 + 1, math.MaxInt64} {
		if _, ok := Uint32(n); ok {
			t.Fatalf("wrapped %d", n)
		}
	}
	if n, ok := Uint32(math.MaxUint32); !ok || n != math.MaxUint32 {
		t.Fatal("rejected valid uint32")
	}
	if _, ok := SliceIndex(math.MaxUint64, 100); ok {
		t.Fatal("accepted oversized archive offset")
	}
	if _, ok := SliceIndex(9, 8); ok {
		t.Fatal("out-of-buffer offset")
	}
	if p, ok := SliceIndex(8, 8); !ok || p != 8 {
		t.Fatal("rejected end index")
	}
}
