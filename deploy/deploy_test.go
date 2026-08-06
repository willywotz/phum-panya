package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runLib(t *testing.T, snippet string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", ". ./deploy-lib.sh; "+snippet)
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

func TestValidateVersionAccepts(t *testing.T) {
	_, _, code := runLib(t, "validate_version 1.5.0")
	if code != 0 {
		t.Fatalf("validate_version 1.5.0 exit = %d, want 0", code)
	}
}

func TestValidateVersionRejects(t *testing.T) {
	for _, bad := range []string{"v1.5.0", "1.5", "latest", "1.5.0-rc1", "1.2.3.4"} {
		_, errOut, code := runLib(t, "validate_version "+bad)
		if code == 0 {
			t.Fatalf("validate_version %q exit = 0, want non-zero", bad)
		}
		if !strings.Contains(errOut, bad) {
			t.Fatalf("validate_version %q stderr = %q, want it to name the bad value", bad, errOut)
		}
	}
}

func TestReadTag(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\nAPP_IMAGE_TAG=1.4.0\nFOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runLib(t, "read_tag "+env)
	if code != 0 {
		t.Fatalf("read_tag exit = %d", code)
	}
	if got := strings.TrimSpace(out); got != "1.4.0" {
		t.Fatalf("read_tag = %q, want 1.4.0", got)
	}
}

func TestReadTagAbsentIsEmpty(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runLib(t, "read_tag "+env)
	if got := strings.TrimSpace(out); got != "" {
		t.Fatalf("read_tag absent = %q, want empty", got)
	}
}

func TestWriteTagReplaceInPlace(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\nAPP_IMAGE_TAG=1.4.0\nFOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runLib(t, "write_tag "+env+" 1.5.0"); code != 0 {
		t.Fatalf("write_tag exit = %d", code)
	}
	b, _ := os.ReadFile(env)
	got := string(b)
	if !strings.Contains(got, "APP_IMAGE_TAG=1.5.0") {
		t.Fatalf("after write_tag, file = %q, want APP_IMAGE_TAG=1.5.0", got)
	}
	if strings.Contains(got, "1.4.0") {
		t.Fatalf("old tag still present: %q", got)
	}
	if !strings.Contains(got, "APP_DOMAIN=x.org") || !strings.Contains(got, "FOO=bar") {
		t.Fatalf("write_tag disturbed other lines: %q", got)
	}
}

func TestWriteTagAppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("APP_DOMAIN=x.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runLib(t, "write_tag "+env+" 1.5.0"); code != 0 {
		t.Fatalf("write_tag exit = %d", code)
	}
	out, _, _ := runLib(t, "read_tag "+env)
	if got := strings.TrimSpace(out); got != "1.5.0" {
		t.Fatalf("round-trip read_tag = %q, want 1.5.0", got)
	}
}
