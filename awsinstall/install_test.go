package awsinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	stubDir := writeStubAWS(t, "2.13.0")
	t.Setenv("PATH", stubDir)
	assert.Assert(t, IsInstalled("latest"))
}

func TestIsInstalled_ExactMatch(t *testing.T) {
	stubDir := writeStubAWS(t, "2.13.0")
	t.Setenv("PATH", stubDir)
	assert.Assert(t, IsInstalled("2.13.0"))
}

func TestIsInstalled_NoMatch(t *testing.T) {
	stubDir := writeStubAWS(t, "2.13.0")
	t.Setenv("PATH", stubDir)
	assert.Assert(t, !IsInstalled("2.14.0"))
}

func TestInstallLinux_IgnoresInstallerPlantedInTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	planted := filepath.Join(tmp, "awscli-exe-linux-"+arch+".zip")
	assert.NilError(t, os.WriteFile(planted, []byte("planted"), 0600))

	marker := filepath.Join(t.TempDir(), "installed")
	installer := fmt.Sprintf("#!/bin/sh\necho \"$@\" > %s\n", marker)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(zipWith(t, "aws/install", installer))
	}))
	defer srv.Close()
	t.Setenv("PATH", writeStubSudo(t)+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := installLinux(context.Background(), EnsureOptions{
		Version:    "latest",
		InstallDir: "/opt/aws-cli",
		BinaryDir:  "/opt/bin",
		BaseURL:    srv.URL,
	})
	assert.NilError(t, err)

	got, err := os.ReadFile(marker)
	assert.NilError(t, err, "the downloaded installer should have run")
	assert.Equal(t, string(got), "-i /opt/aws-cli -b /opt/bin\n")

	entries, err := os.ReadDir(tmp)
	assert.NilError(t, err)
	assert.Equal(t, len(entries), 1, "the run's temp dir should be removed")
	assert.Equal(t, entries[0].Name(), filepath.Base(planted))
}

// zipWith returns a zip archive holding one executable file.
func zipWith(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	assert.NilError(t, err)
	_, err = w.Write([]byte(content))
	assert.NilError(t, err)
	assert.NilError(t, zw.Close())
	return buf.Bytes()
}

// writeStubSudo writes a sudo stub that drops its own options and runs the
// rest of its arguments as the current user, and returns its directory.
func writeStubSudo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nwhile [ \"${1#-}\" != \"$1\" ]; do shift; done\nexec \"$@\"\n"
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0750))
	return dir
}

// writeOSRelease writes content to a temp file and returns its path.
func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "os-release")
	assert.NilError(t, os.WriteFile(f, []byte(content), 0600))
	return f
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
