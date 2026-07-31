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
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{ //nolint:gosec
		Profile:         "default",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})
	assert.NilError(t, err)

	got := readRecord(t, rec)
	assertContainsSequence(t, got, "configure", "set", "aws_access_key_id", "AKIAIOSFODNN7EXAMPLE", "--profile", "default")
	assertContainsSequence(t, got, "configure", "set", "aws_secret_access_key", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "--profile", "default")
}

func TestConfigure_WithSessionToken(t *testing.T) {
	rec, stubDir := stubAWS(t)
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	err := awsconfigure.Configure(context.Background(), awsconfigure.Options{
		Profile:         "myrole",
		AccessKeyID:     "ASIA...",
		SecretAccessKey: "secret",
		SessionToken:    "token123",
	})
	assert.NilError(t, err)

	got := readRecord(t, rec)
	assertContainsSequence(t, got, "configure", "set", "aws_session_token", "token123", "--profile", "myrole")
}

func TestConfigure_WithRegion(t *testing.T) {
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

// readRecord returns the content of the stub record file.
func readRecord(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	assert.NilError(t, err)
	return string(b)
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
