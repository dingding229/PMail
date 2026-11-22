package auth

import "testing"

func TestRegistrationLimitCountsAcrossPortsAndSeparatesPeerIPs(t *testing.T) {
	for i := range 5 {
		remote := []string{"192.0.2.45:5001", "192.0.2.45:5002"}[i%2]
		finish, ok := ReserveRegistration(remote)
		if !ok {
			t.Fatalf("attempt %d was limited too soon", i+1)
		}
		finish(true)
	}
	if finish, ok := ReserveRegistration("192.0.2.45:5003"); ok {
		finish(false)
		t.Fatal("sixth attempt should be limited for the same IP")
	}
	if finish, ok := ReserveRegistration("192.0.2.46:5003"); !ok {
		t.Fatal("different IP was incorrectly limited")
	} else {
		finish(false)
	}
}
