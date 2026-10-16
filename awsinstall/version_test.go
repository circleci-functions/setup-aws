package awsinstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIsInstalled_ExactDoesNotMatchOtherVersions(t *testing.T) {
	tests := []struct {
		output, version string
	}{
		{"aws-cli/2.13.30 Python/3.11.4 Linux/6.1", "2.13.3"},
		{"aws-cli/2.13.0 Python/3.11.4 Linux/6.1", "3.11.4"},
		{"aws-cli/1.18.0 Python/3.8.10 Linux/5.4 botocore/1.17.0", "1.17.0"},
		{"aws-cli/2.13.3 Python/3.11.4 Linux/6.1", "2.13"},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		script := fmt.Sprintf("#!/bin/sh\necho '%s'\n", tt.output)
		assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0750))
		t.Setenv("PATH", dir)
		assert.Assert(t, !IsInstalled(tt.version), "%q should not satisfy %s", tt.output, tt.version)
	}
}

func TestValidateVersion(t *testing.T) {
	for _, v := range []string{"latest", "2.13.0", "2.0.30", "2.34.60"} {
		assert.NilError(t, validateVersion(v), v)
	}
	for _, v := range []string{"", "2", "2.13", "v2.13.0", "2.13.0-rc1", "Latest", "2.13.0 "} {
		assert.ErrorContains(t, validateVersion(v), "--version must be", v)
	}
}

func TestEnsure_RejectsPartialVersionBeforeInstalling(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(t.TempDir(), "calls")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\n", record)
	for _, name := range []string{"aws", "sudo", "apk", "installer"} {
		assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0750))
	}
	t.Setenv("PATH", dir)

	err := Ensure(context.Background(), EnsureOptions{Version: "2.13", BaseURL: "http://127.0.0.1:0"})
	assert.Error(t, err, `--version must be "latest" or a full AWS CLI version like 2.13.0, got "2.13"`)
	_, statErr := os.Stat(record)
	assert.Assert(t, os.IsNotExist(statErr), "nothing should run")
}
