package releasebundle

import "testing"

func TestRankOrdersMajorMinorPatch(t *testing.T) {
	ordered := []string{"v0.0.1", "v0.0.8", "v0.1.0", "v0.10.0", "v0.999999.999999", "v1.0.0", "v1.0.1", "v1.2.0", "v2.0.0"}
	for i := 1; i < len(ordered); i++ {
		if Rank(ordered[i-1]) >= Rank(ordered[i]) {
			t.Fatalf("%s should rank below %s", ordered[i-1], ordered[i])
		}
	}
	for _, bad := range []string{"1.0.0", "v01.0.0", "v1.0", "v1.0.0-rc.1", "v1.0.0+build", "v1000.0.0", "v1.1000000.0", ""} {
		if Rank(bad) >= 0 {
			t.Fatalf("accepted invalid tag %q", bad)
		}
	}
}
