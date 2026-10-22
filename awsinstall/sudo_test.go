package awsinstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRunWithSudo_NonInteractive(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("sudo is only used when not root")
	}
	dir := t.TempDir()
	record := filepath.Join(t.TempDir(), "sudo-record")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %s\n", record)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0750))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := runWithSudo(context.Background(), "", "installer", "-pkg", "AWSCLIV2.pkg", "-target", "/")
	assert.NilError(t, err)

	got, err := os.ReadFile(record)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "-n true\n-n installer -pkg AWSCLIV2.pkg -target /\n")
}

func TestRunWithSudo_PasswordRequired(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("sudo is only used when not root")
	}
	dir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$2\" = true ]; then echo 'sudo: a password is required' >&2; exit 1; fi\ntouch %s\n", marker)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0750))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := runWithSudo(context.Background(), "", "installer", "-pkg", "AWSCLIV2.pkg", "-target", "/")
	assert.Error(t, err, "installing the AWS CLI needs root or passwordless sudo: sudo: a password is required")
	_, statErr := os.Stat(marker)
	assert.Assert(t, os.IsNotExist(statErr), "the installer should not run")
}

func TestRunWithSudo_SudoMissing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("sudo is only used when not root")
	}
	t.Setenv("PATH", t.TempDir())

	err := runWithSudo(context.Background(), "", "installer", "-pkg", "AWSCLIV2.pkg", "-target", "/")
	assert.Error(t, err, "installing the AWS CLI needs root or passwordless sudo, and sudo is not installed")
}
