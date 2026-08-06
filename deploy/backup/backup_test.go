package backup

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runLib(t *testing.T, env []string, snippet string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", ". ./media-backup-lib.sh; "+snippet)
	if env != nil {
		cmd.Env = env
	}
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func TestArchivePath(t *testing.T) {
	out, _, code := runLib(t, nil, "archive_path 1000 2026-08-06T13-41-00Z")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := strings.TrimSpace(out), "archive/1000-2026-08-06T13-41-00Z"; got != want {
		t.Fatalf("archive_path = %q, want %q", got, want)
	}
}

func TestPrunePlanSelectsOnlyOlderThanCutoff(t *testing.T) {
	// keep=100, now=1000 -> cutoff=900. Only epochs < 900 are pruned; 900 (==cutoff) is kept.
	out, _, code := runLib(t, nil, "prune_plan 100 1000 899-a 900-b 901-c")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := strings.TrimSpace(out), "899-a"; got != want {
		t.Fatalf("prune_plan = %q, want %q", got, want)
	}
}

func TestRequireEnvPassesWhenSet(t *testing.T) {
	_, _, code := runLib(t, append(os.Environ(), "FOO=bar"), "require_env FOO")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

func TestRequireEnvFailsAndNamesMissing(t *testing.T) {
	_, errOut, code := runLib(t, []string{"PATH=" + os.Getenv("PATH")}, "require_env FOO")
	if code == 0 {
		t.Fatal("exit = 0, want non-zero for unset FOO")
	}
	if !strings.Contains(errOut, "FOO") {
		t.Fatalf("stderr = %q, want it to name FOO", errOut)
	}
}
