package awsinstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestAlpineIgnoredFlags(t *testing.T) {
	defaults := EnsureOptions{
		Version:    "latest",
		InstallDir: defaultInstallDir,
		BinaryDir:  defaultBinaryDir,
		BaseURL:    DefaultBaseURL,
	}
	assert.DeepEqual(t, alpineIgnoredFlags(defaults), []string(nil))

	all := EnsureOptions{
		Version:    "2.13.0",
		InstallDir: "/opt/aws-cli",
		BinaryDir:  "/opt/bin",
		UseBrew:    true,
		BaseURL:    "http://localhost",
	}
	assert.DeepEqual(t, alpineIgnoredFlags(all), []string{"--version", "--install-dir", "--binary-dir", "--use-brew", "--base-url"})
}

func TestInstallForPlatform_AlpineUsesApk(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(t.TempDir(), "apk-record")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %s\n", record)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "apk"), []byte(script), 0750))
	t.Setenv("PATH", dir)

	err := installForPlatform(context.Background(), platformAlpine, EnsureOptions{Version: "2.13.0", InstallDir: "/opt/aws-cli"})
	assert.NilError(t, err)

	got, err := os.ReadFile(record)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "add --no-cache aws-cli\n")
}
