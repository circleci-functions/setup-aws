package awsconfigure_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/setup-aws/awsconfigure"
)

func TestConfigure_BasicCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{ //nolint:gosec
		Profile:         "default",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})
	assert.NilError(t, err)

	path := filepath.Join(home, ".aws", "credentials")
	assert.Equal(t, readFile(t, path), "[default]\n"+
		"aws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"+
		"aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n")
	assertMode(t, path, 0o600)
	assert.Assert(t, !strings.Contains(readRecord(t, rec), "wJalrXUtnFEMI"), "secret must not be passed to aws")
}

func TestConfigure_WithSessionToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		Profile:         "myrole",
		AccessKeyID:     "ASIA...",
		SecretAccessKey: "secret",
		SessionToken:    "token123",
	})
	assert.NilError(t, err)

	assert.Equal(t, readFile(t, filepath.Join(home, ".aws", "credentials")), "[myrole]\n"+
		"aws_access_key_id = ASIA...\n"+
		"aws_secret_access_key = secret\n"+
		"aws_session_token = token123\n")
	got := readRecord(t, rec)
	assert.Assert(t, !strings.Contains(got, "secret") && !strings.Contains(got, "token123"), "secrets must not be passed to aws, got: %s", got)
}

func TestConfigure_KeepsOtherProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	path := filepath.Join(home, ".aws", "credentials")
	assert.NilError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assert.NilError(t, os.WriteFile(path, []byte("[other]\naws_access_key_id = OTHER\n\n[deploy]\naws_access_key_id = OLD\naws_session_token = keep\n"), 0o644))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		Profile:         "deploy",
		AccessKeyID:     "NEW",
		SecretAccessKey: "secret",
	})
	assert.NilError(t, err)

	assert.Equal(t, readFile(t, path), "[other]\n"+
		"aws_access_key_id = OTHER\n"+
		"\n"+
		"[deploy]\n"+
		"aws_access_key_id = NEW\n"+
		"aws_session_token = keep\n"+
		"aws_secret_access_key = secret\n")
	assertMode(t, path, 0o600)
}

func TestConfigure_HonoursSharedCredentialsFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "creds")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	_, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	})
	assert.NilError(t, err)

	assert.Equal(t, readFile(t, path), "[default]\naws_access_key_id = key\naws_secret_access_key = secret\n")
}

func TestConfigure_WithRegion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		Profile:                "default",
		AccessKeyID:            "key",
		SecretAccessKey:        "secret",
		Region:                 "us-east-1",
		ConfigureDefaultRegion: true,
		ConfigureProfileRegion: true,
	})
	assert.NilError(t, err)

	got := readRecord(t, rec)
	assertContainsSequence(t, got, "configure", "set", "default.region", "us-east-1")
	assertContainsSequence(t, got, "configure", "set", "region", "us-east-1", "--profile", "default")
}

func TestConfigure_SkipsRegionWhenEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		Profile:         "default",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Region:          "",
	})
	assert.NilError(t, err)

	got := readRecord(t, rec)
	assert.Assert(t, !strings.Contains(got, "region"), "expected no region config when empty, got: %s", got)
}

func TestConfigureRoleARN(t *testing.T) {
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.ConfigureRoleARN(context.Background(), "deploy", "arn:aws:iam::123:role/Deploy", "default")
	assert.NilError(t, err)

	got := readRecord(t, rec)
	assertContainsSequence(t, got, "configure", "set", "profile.deploy.role_arn", "arn:aws:iam::123:role/Deploy")
	assertContainsSequence(t, got, "configure", "set", "profile.deploy.source_profile", "default")
}

// stubAWS creates a stub aws script that records all invocations (one arg per
// line, invocations separated by "---") to a file. Returns the record file path
// and the directory containing the stub.
func stubAWS(t *testing.T) (recordFile, dir string) {
	t.Helper()
	dir = t.TempDir()
	recordFile = filepath.Join(t.TempDir(), "aws-record.txt")
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do echo "$arg"; done >> %s
echo '---' >> %s
`, recordFile, recordFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0750))
	return recordFile, dir
}

// readRecord returns the content of the stub record file, or "" if aws was
// never called.
func readRecord(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	assert.NilError(t, err)
	return string(b)
}

// readFile returns the content of the file at path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.NilError(t, err)
	return string(b)
}

// assertMode checks the permission bits of the file at path.
func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), want)
}

// assertContainsSequence verifies that want appears as a contiguous subsequence
// in the recorded arguments (within a single invocation block).
func assertContainsSequence(t *testing.T, got string, want ...string) {
	t.Helper()
	// Each invocation block is separated by "---\n". Check each block.
	for _, block := range strings.Split(got, "---\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if containsSubseq(lines, want) {
			return
		}
	}
	t.Errorf("recorded calls do not contain sequence %v\nrecord:\n%s", want, got)
}

func containsSubseq(lines, want []string) bool {
	for i := 0; i <= len(lines)-len(want); i++ {
		match := true
		for j, w := range want {
			if lines[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
