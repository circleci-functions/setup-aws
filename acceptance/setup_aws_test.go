package setupaws_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestCLI_Install_SkipWhenAlreadyInstalled verifies that install is skipped
// when the aws CLI is already on PATH at the requested version. The stub aws
// binary reports version 2.13.0, so passing --version 2.13.0 should skip.
func TestCLI_Install_SkipWhenAlreadyInstalled(t *testing.T) {
	stubDir := writeStubAWS(t, "2.13.0")
	_, run := sandbox(t)

	out, err := run(t, map[string]string{
		"PATH":      stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER": "disabled",
	}, "--version", "2.13.0")
	assert.NilError(t, err, "got: %s", out)
	assert.Assert(t, strings.Contains(out, "already installed"), "expected skip message, got: %s", out)
}

// TestCLI_Install_WritesBashEnv verifies that AWS_PAGER is written to $BASH_ENV
// when the flag is not disabled and AWS_PAGER is not already set.
func TestCLI_Install_WritesBashEnv(t *testing.T) {
	stubDir := writeStubAWS(t, "2.13.0")
	s, run := sandbox(t)

	out, err := run(t, map[string]string{
		"PATH":      stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER": "",
	}, "--version", "2.13.0")
	assert.NilError(t, err, "got: %s", out)

	content, readErr := os.ReadFile(s.BashEnv)
	assert.NilError(t, readErr)
	assert.Assert(t, strings.Contains(string(content), "AWS_PAGER"), "expected AWS_PAGER in BASH_ENV, got: %s", content)
}

// writeStubAWS writes a stub aws script that reports the given version and
// returns the directory containing it.
func writeStubAWS(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\necho 'aws-cli/%s Python/3.11 Linux/x86_64'\n", version)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0750))
	return dir
}
