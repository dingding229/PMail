package id

import "crypto/rand"

// GenLogID identifies a request without leaking an interface IP or PID.
func GenLogID() string { return rand.Text() }
