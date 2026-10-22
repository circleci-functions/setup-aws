package setupaws_test

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

// TestCLI_Install_WritesPagerLineOnce verifies that running setup-aws again in
// the same job doesn't add another AWS_PAGER line to $BASH_ENV.
func TestCLI_Install_WritesPagerLineOnce(t *testing.T) {
	stubDir := writeStubAWS(t, "2.34.60")
	s, run := sandbox(t)
	env := map[string]string{
		"PATH":      stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER": "",
	}

	for range 2 {
		out, err := run(t, env, "--version", "2.34.60")
		assert.NilError(t, err, "got: %s", out)
	}

	content, err := os.ReadFile(s.BashEnv)
	assert.NilError(t, err)
	assert.Equal(t, string(content), "export AWS_PAGER=\"\"\n")
}
