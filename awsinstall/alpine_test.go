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
	t.Setenv("PATH", dir+string(os.PathListSeparator)+writeStubAWS(t, "2.15.57"))

	err := installForPlatform(context.Background(), platformAlpine, EnsureOptions{Version: "2.13.0", InstallDir: "/opt/aws-cli"})
	assert.NilError(t, err)

	got, err := os.ReadFile(record)
	assert.NilError(t, err)
	assert.Equal(t, string(got), "add --no-cache aws-cli\n")
}

func TestInstallAlpine_WrapsAWSThatOnlyStartsWithAlpinesLibpython(t *testing.T) {
	wrapperDir, apkDir := t.TempDir(), t.TempDir()
	writeAWS(t, apkDir, "[ \"$LD_LIBRARY_PATH\" = /usr/lib ] || exit 1\necho aws-cli/2.15.57")
	assert.NilError(t, os.WriteFile(filepath.Join(apkDir, "apk"), []byte("#!/bin/sh\n"), 0750))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+apkDir)

	assert.NilError(t, installAlpine(context.Background(), wrapperDir))
	assert.Assert(t, IsInstalled("2.15.57"))
}

func TestInstallAlpine_FailsWhenAWSDoesntStart(t *testing.T) {
	wrapperDir, apkDir := t.TempDir(), t.TempDir()
	writeAWS(t, apkDir, "echo 'symbol not found' >&2\nexit 1")
	assert.NilError(t, os.WriteFile(filepath.Join(apkDir, "apk"), []byte("#!/bin/sh\n"), 0750))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+apkDir)

	err := installAlpine(context.Background(), wrapperDir)
	assert.Error(t, err, "aws --version failed after apk add aws-cli: symbol not found")
	_, statErr := os.Stat(filepath.Join(wrapperDir, "aws"))
	assert.Assert(t, os.IsNotExist(statErr), "the wrapper should be removed")
}

// writeAWS writes an aws stub running body into dir.
func writeAWS(t *testing.T, dir, body string) {
	t.Helper()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "aws"), []byte("#!/bin/sh\n"+body+"\n"), 0750))
}
