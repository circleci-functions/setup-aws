package setupaws_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// TestCLI_WarnsAboutIgnoredCredentialFlags verifies that credential flags the
// user passed are named when this run ignores them, and that values that only
// come from env vars are not.
func TestCLI_WarnsAboutIgnoredCredentialFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		notWant []string
	}{
		{
			name:    "without --configure-credentials",
			args:    []string{"--role-arn", "arn:aws:iam::123456789012:role/Deploy", "--region", "us-east-1"},
			want:    "setup-aws: warning: ignoring --region, --role-arn without --configure-credentials",
			notWant: []string{"--aws-access-key-id", "--role-session-name"},
		},
		{
			name: "with --source-profile",
			args: []string{
				"--configure-credentials", "--profile-name", "deploy",
				"--role-arn", "arn:aws:iam::123456789012:role/Deploy", "--source-profile", "default",
				"--aws-secret-access-key", "secret", "--session-duration", "900", "--region", "us-east-1",
			},
			want:    "setup-aws: warning: ignoring --aws-secret-access-key, --session-duration with --source-profile",
			notWant: []string{"--aws-access-key-id,", "--region,", "--role-session-name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubDir, _ := writeRecordingAWS(t, "2.13.0")
			_, run := sandbox(t)

			out, err := run(t, map[string]string{
				"PATH":                  stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"AWS_PAGER":             "disabled",
				"AWS_ACCESS_KEY_ID":     "key-from-env",
				"AWS_SECRET_ACCESS_KEY": "from-env",
				"AWS_DEFAULT_REGION":    "us-west-2",
				"CIRCLE_JOB":            "build",
			}, tt.args...)
			assert.NilError(t, err, "got: %s", out)
			assert.Assert(t, strings.Contains(out, tt.want), "got: %s", out)
			for _, nw := range tt.notWant {
				assert.Assert(t, !strings.Contains(out, nw), "env-only value %s should not be named, got: %s", nw, out)
			}
		})
	}
}

// TestCLI_SourceProfile_WritesRegion verifies that --source-profile writes the
// region to the role profile alongside role_arn and source_profile.
func TestCLI_SourceProfile_WritesRegion(t *testing.T) {
	stubDir, record := writeRecordingAWS(t, "2.13.0")
	_, run := sandbox(t)

	out, err := run(t, map[string]string{
		"PATH":      stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"AWS_PAGER": "disabled",
	}, "--configure-credentials", "--profile-name", "deploy",
		"--role-arn", "arn:aws:iam::123456789012:role/Deploy", "--source-profile", "default",
		"--region", "eu-west-1", "--configure-default-region=false")
	assert.NilError(t, err, "got: %s", out)

	got, err := os.ReadFile(record)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "--version\n"+
		"configure set profile.deploy.role_arn arn:aws:iam::123456789012:role/Deploy\n"+
		"configure set profile.deploy.source_profile default\n"+
		"configure set region eu-west-1 --profile deploy\n")
}

// writeRecordingAWS writes a stub aws script that reports the given version
// and appends each call's arguments as a line to a record file. It returns the
// directory containing the stub and the record file's path.
func writeRecordingAWS(t *testing.T, version string) (dir, record string) {
	t.Helper()
	dir = t.TempDir()
	record = filepath.Join(t.TempDir(), "aws-calls")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\necho 'aws-cli/%s Python/3.11 Linux/x86_64'\n", record, version)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0750))
	return dir, record
}
