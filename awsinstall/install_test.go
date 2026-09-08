package awsinstall

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func TestDetectPlatform_Alpine(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("os-release detection only applies on Linux")
	}
	f := writeOSRelease(t, "ID=alpine\nNAME=\"Alpine Linux\"\n")
	assert.Equal(t, detectPlatform(f), platformAlpine)
}

func TestDetectPlatform_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("os-release detection only applies on Linux")
	}
	f := writeOSRelease(t, "ID=ubuntu\nNAME=\"Ubuntu\"\n")
	assert.Equal(t, detectPlatform(f), platformLinux)
}

func TestDetectPlatform_LinuxMissingFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("os-release detection only applies on Linux")
	}
	assert.Equal(t, detectPlatform("/nonexistent/os-release"), platformLinux)
}

func TestBuildLinuxURL(t *testing.T) {
	base := "https://awscli.amazonaws.com"
	tests := []struct {
		version, arch, want string
	}{
		{"latest", "x86_64", base + "/awscli-exe-linux-x86_64.zip"},
		{"2.13.0", "x86_64", base + "/awscli-exe-linux-x86_64-2.13.0.zip"},
		{"latest", "aarch64", base + "/awscli-exe-linux-aarch64.zip"},
		{"2.13.0", "aarch64", base + "/awscli-exe-linux-aarch64-2.13.0.zip"},
	}
	for _, tt := range tests {
		got := buildLinuxURL(base, tt.version, tt.arch)
		assert.Equal(t, got, tt.want, "buildLinuxURL(%q, %q)", tt.version, tt.arch)
	}
}

func TestBuildDarwinURL(t *testing.T) {
	base := "https://awscli.amazonaws.com"
	tests := []struct {
		version, want string
	}{
		{"latest", base + "/AWSCLIV2.pkg"},
		{"2.13.0", base + "/AWSCLIV2-2.13.0.pkg"},
	}
	for _, tt := range tests {
		got := buildDarwinURL(base, tt.version)
		assert.Equal(t, got, tt.want, "buildDarwinURL(%q)", tt.version)
	}
}

func TestIsInstalled_NotOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	assert.Assert(t, !IsInstalled("latest"))
}

func TestIsInstalled_AnyVersion(t *testing.T) {
	stubDir := writeStubAWS(t)
	t.Setenv("PATH", stubDir)
	assert.Assert(t, IsInstalled("latest"))
}

func TestIsInstalled_ExactMatch(t *testing.T) {
	stubDir := writeStubAWS(t)
	t.Setenv("PATH", stubDir)
	assert.Assert(t, IsInstalled(stubVersion))
}

func TestIsInstalled_NoMatch(t *testing.T) {
	stubDir := writeStubAWS(t)
	t.Setenv("PATH", stubDir)
	assert.Assert(t, !IsInstalled("2.14.0"))
}

// TestLinkAWS verifies the aws binary is symlinked into a directory on PATH, so
// later steps resolve it in any shell. A link into a temp directory via the
// linkAWSInto seam, so the test never writes to the real PATH.
func TestLinkAWS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the aws stub is a shell script, not executable on Windows")
	}
	fromDir := writeStubAWS(t)
	t.Setenv("PATH", fromDir)
	linkDir := t.TempDir()

	assert.NilError(t, linkAWSInto(linkDir, fromDir))

	target, err := os.Readlink(filepath.Join(linkDir, "aws"))
	assert.NilError(t, err)
	assert.Equal(t, target, filepath.Join(fromDir, "aws"))
}

// TestLinkAWS_NotOnPath verifies linking is a no-op, not an error, when no aws
// is resolvable: the install step reports a missing binary better.
func TestLinkAWS_NotOnPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	assert.NilError(t, linkAWSInto("", ""))
}

// writeOSRelease writes content to a temp file and returns its path.
func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "os-release")
	assert.NilError(t, os.WriteFile(f, []byte(content), 0600))
	return f
}

// stubVersion is the version the aws stub reports.
const stubVersion = "2.13.0"

// writeStubAWS writes a stub aws script that reports stubVersion and returns
// the directory containing it.
func writeStubAWS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\necho 'aws-cli/%s Python/3.11 Linux/x86_64'\n", stubVersion)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte(script), 0750))
	return dir
}
