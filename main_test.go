package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/setup-aws/awsoidc"
)

func TestExportCredsToEnv_QuotesValues(t *testing.T) {
	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	t.Setenv("BASH_ENV", bashEnv)
	marker := filepath.Join(t.TempDir(), "pwned")
	creds := awsoidc.Credentials{
		AccessKeyID:     "ASIAEXAMPLE",
		SecretAccessKey: `it's "secret"`,
		SessionToken:    "token$(touch " + marker + ")`id`$HOME",
	}

	assert.NilError(t, exportCredsToEnv(creds))

	content, err := os.ReadFile(bashEnv)
	assert.NilError(t, err)
	assert.Equal(t, string(content), "export AWS_ACCESS_KEY_ID='ASIAEXAMPLE'\n"+
		`export AWS_SECRET_ACCESS_KEY='it'\''s "secret"'`+"\n"+
		"export AWS_SESSION_TOKEN='token$(touch "+marker+")`id`$HOME'\n")

	out, err := exec.Command("bash", "-c", `. "$BASH_ENV" && printf '%s\n%s' "$AWS_SECRET_ACCESS_KEY" "$AWS_SESSION_TOKEN"`).Output()
	assert.NilError(t, err)
	assert.Equal(t, string(out), creds.SecretAccessKey+"\n"+creds.SessionToken)
	_, statErr := os.Stat(marker)
	assert.Assert(t, os.IsNotExist(statErr), "sourcing $BASH_ENV must not run commands from the values")
}

func TestAppendToBashEnv_CreatesPrivateFile(t *testing.T) {
	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	t.Setenv("BASH_ENV", bashEnv)

	assert.NilError(t, disableAWSPager())

	info, err := os.Stat(bashEnv)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
}

func TestAppendToBashEnv_TightensExistingFile(t *testing.T) {
	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	assert.NilError(t, os.WriteFile(bashEnv, []byte("export FOO=bar\n"), 0o644))
	assert.NilError(t, os.Chmod(bashEnv, 0o644)) //nolint:gosec
	t.Setenv("BASH_ENV", bashEnv)

	assert.NilError(t, exportCredsToEnv(awsoidc.Credentials{AccessKeyID: "a", SecretAccessKey: "b", SessionToken: "c"}))

	info, err := os.Stat(bashEnv)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
	content, err := os.ReadFile(bashEnv)
	assert.NilError(t, err)
	assert.Equal(t, string(content), "export FOO=bar\nexport AWS_ACCESS_KEY_ID='a'\nexport AWS_SECRET_ACCESS_KEY='b'\nexport AWS_SESSION_TOKEN='c'\n")
}

// A $BASH_ENV another user owns can't be tightened, so its mode is what it
// was. The credentials only go into it when that already keeps other users
// out.
func TestCheckPrivate(t *testing.T) {
	assert.NilError(t, checkPrivate("bash_env", 0o600))
	assert.NilError(t, checkPrivate("bash_env", 0o400))
	for _, mode := range []fs.FileMode{0o640, 0o604, 0o620, 0o666} {
		err := checkPrivate("bash_env", mode)
		assert.ErrorContains(t, err, fmt.Sprintf("bash_env has mode %04o and belongs to another user", mode))
		assert.ErrorContains(t, err, "--set-aws-env-vars=false")
	}
}

func TestDisableAWSPager_WritesTheLineOnce(t *testing.T) {
	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	t.Setenv("BASH_ENV", bashEnv)

	for range 3 {
		assert.NilError(t, disableAWSPager())
	}

	content, err := os.ReadFile(bashEnv)
	assert.NilError(t, err)
	assert.Equal(t, string(content), "export AWS_PAGER=\"\"\n")
}

func TestDisableAWSPager_BashEnvUnset(t *testing.T) {
	t.Setenv("BASH_ENV", "")
	assert.NilError(t, disableAWSPager())
}

func TestExportCredsToEnv_BashEnvUnset(t *testing.T) {
	t.Setenv("BASH_ENV", "")
	err := exportCredsToEnv(awsoidc.Credentials{AccessKeyID: "a", SecretAccessKey: "b", SessionToken: "c"})
	assert.Error(t, err, "BASH_ENV must be set to export the assumed-role credentials (pass --set-aws-env-vars=false to skip)")
}
