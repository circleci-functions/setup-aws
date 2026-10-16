package setupaws_test

import (
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestCLI_Install_RejectsPartialVersion verifies that a --version AWS has no
// installer for fails before anything is installed or written to $BASH_ENV.
func TestCLI_Install_RejectsPartialVersion(t *testing.T) {
	stubDir := writeStubAWS(t, "2.34.60")
	s, run := sandbox(t)

	out, err := run(t, map[string]string{
		"PATH":      stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER": "",
	}, "--version", "2.13")
	assert.Assert(t, err != nil, "expected an error, got: %s", out)
	assert.Assert(t, strings.Contains(out, `--version must be "latest" or a full AWS CLI version like 2.13.0, got "2.13"`), "got: %s", out)
	assert.Assert(t, !strings.Contains(out, "already installed"), "got: %s", out)

	_, statErr := os.Stat(s.BashEnv)
	assert.Assert(t, os.IsNotExist(statErr), "$BASH_ENV should be untouched")
}
