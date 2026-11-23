package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLinuxExecRunsStagedProgram(t *testing.T) {
	if root := os.Getenv("PMAIL_FIXTURE_EXEC_ROOT"); root != "" {
		key, err := base64.StdEncoding.DecodeString(os.Getenv("PMAIL_FIXTURE_EXEC_PUB"))
		if err != nil {
			t.Fatal(err)
		}
		in := installer{store: storeAt(root), key: ed25519.PublicKey(key), arch: "amd64", supported: true}
		if err := in.bootImage(testOld, root, []string{"pmail", "smoke"}, os.Environ(), execUpdate); err != nil {
			t.Fatal(err)
		}
		t.Fatal("successful Exec returned to old program")
	}
	t.Setenv("PMAIL_DISABLE_STAGED_UPDATE", "")
	in, root, _ := seedStage(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestLinuxExecRunsStagedProgram$")
	cmd.Env = append(os.Environ(), "PMAIL_FIXTURE_EXEC_ROOT="+root, "PMAIL_FIXTURE_EXEC_PUB="+base64.StdEncoding.EncodeToString(in.key))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "staged version=v0.0.4 root="+root+" arg=smoke") {
		t.Fatalf("real exec handoff: %v\n%s", err, out)
	}
}
