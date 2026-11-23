package update

import "syscall"

func execUpdate(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env) // #nosec G204,G702 -- path is a private version directory and the exact binary is verified against the signed release before exec.
}
