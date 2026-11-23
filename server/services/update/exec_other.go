//go:build !linux

package update

import "errors"

func execUpdate(string, []string, []string) error { return errors.New("staged exec requires Linux") }
