// Package awsinstall handles installing the AWS CLI v2 on the current platform.
package awsinstall

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/circleci-functions/toolkit/download"

	"github.com/circleci-functions/toolkit/archive"
)

// DefaultBaseURL is the base download URL for AWS CLI installers.
const DefaultBaseURL = "https://awscli.amazonaws.com"

const osReleasePath = "/etc/os-release"

type platform int

const (
	platformLinux platform = iota
	platformAlpine
	platformDarwin
	platformWindows
)

func detectPlatform(osReleaseFile string) platform {
	switch runtime.GOOS {
	case "windows":
		return platformWindows
	case "darwin":
		return platformDarwin
	}
	data, err := os.ReadFile(osReleaseFile) //nolint:gosec
	if err == nil && strings.Contains(string(data), "Alpine") {
		return platformAlpine
	}
	return platformLinux
}

// IsInstalled reports whether the AWS CLI is installed. With version "latest",
// any installed version satisfies the check; otherwise the aws --version output
// must contain version.
func IsInstalled(version string) bool {
	out, err := exec.Command("aws", "--version").Output() //nolint:gosec
	if err != nil {
		return false
	}
	if version == "latest" {
		return true
	}
	return strings.Contains(string(out), version)
}

func buildLinuxURL(baseURL, version, arch string) string {
	name := "awscli-exe-linux-" + arch
	if version != "latest" {
		name += "-" + version
	}
	return strings.TrimSuffix(baseURL, "/") + "/" + name + ".zip"
}

func buildDarwinURL(baseURL, version string) string {
	if version == "latest" {
		return strings.TrimSuffix(baseURL, "/") + "/AWSCLIV2.pkg"
	}
	return strings.TrimSuffix(baseURL, "/") + "/AWSCLIV2-" + version + ".pkg"
}

// EnsureOptions controls how Ensure installs the AWS CLI.
type EnsureOptions struct {
	Version           string
	OverrideInstalled bool
	InstallDir        string
	BinaryDir         string
	UseBrew           bool
	// BaseURL overrides the download base URL. Intended for testing.
	BaseURL string
}

// Ensure installs the AWS CLI if not already installed. When OverrideInstalled
// is true, it reinstalls even if a version is already present.
func Ensure(ctx context.Context, opts EnsureOptions) error {
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.InstallDir == "" {
		opts.InstallDir = "/usr/local/aws-cli"
	}
	if opts.BinaryDir == "" {
		opts.BinaryDir = "/usr/local/bin"
	}

	p := detectPlatform(osReleasePath)
	if p == platformWindows {
		return fmt.Errorf("setup-aws: Windows is not yet supported")
	}

	if IsInstalled(opts.Version) && !opts.OverrideInstalled {
		fmt.Fprintln(os.Stderr, "setup-aws: AWS CLI already installed, skipping")
		return nil
	}

	fmt.Fprintf(os.Stderr, "setup-aws: installing AWS CLI %s\n", opts.Version)
	return installForPlatform(ctx, p, opts)
}

func installForPlatform(ctx context.Context, p platform, opts EnsureOptions) error {
	switch p {
	case platformAlpine:
		return installAlpine(ctx)
	case platformLinux:
		return installLinux(ctx, opts)
	case platformDarwin:
		return installDarwin(ctx, opts)
	default:
		return fmt.Errorf("setup-aws: unsupported platform")
	}
}

func installAlpine(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "apk", "add", "--no-cache", "aws-cli")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apk add aws-cli: %w", err)
	}
	return nil
}

func installLinux(ctx context.Context, opts EnsureOptions) error {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}

	url := buildLinuxURL(opts.BaseURL, opts.Version, arch)
	fmt.Fprintf(os.Stderr, "setup-aws: downloading %s\n", url)

	zipPath, err := downloadFile(ctx, url)
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	extractDir, err := os.MkdirTemp("", "awscli-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	args := []string{"./aws/install", "-i", opts.InstallDir, "-b", opts.BinaryDir}
	if opts.OverrideInstalled {
		args = append(args, "--update")
	}
	return runWithSudo(ctx, extractDir, args...)
}

func installDarwin(ctx context.Context, opts EnsureOptions) error {
	if opts.UseBrew {
		return installDarwinBrew(ctx, opts.OverrideInstalled)
	}

	url := buildDarwinURL(opts.BaseURL, opts.Version)
	fmt.Fprintf(os.Stderr, "setup-aws: downloading %s\n", url)

	pkgPath, err := downloadFile(ctx, url)
	if err != nil {
		return err
	}
	defer os.Remove(pkgPath)

	return runWithSudo(ctx, "", "installer", "-pkg", pkgPath, "-target", "/")
}

func installDarwinBrew(ctx context.Context, override bool) error {
	verb := "install"
	if override {
		verb = "reinstall"
	}
	cmd := exec.CommandContext(ctx, "brew", verb, "awscli") //nolint:gosec
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ASK=1")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew %s awscli: %w", verb, err)
	}
	return nil
}

// checkSudo fails with a clear message when sudo is missing or wants a
// password, before the install starts.
func checkSudo(ctx context.Context) error {
	const msg = "installing the AWS CLI needs root or passwordless sudo"
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("%s, and sudo is not installed", msg)
	}
	out, err := exec.CommandContext(ctx, "sudo", "-n", "true").CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(out)); detail != "" {
			return fmt.Errorf("%s: %s", msg, detail)
		}
		return fmt.Errorf("%s: sudo -n: %w", msg, err)
	}
	return nil
}

// runWithSudo runs args as root: directly when already root, otherwise through
// non-interactive sudo, since a CI step has no terminal to answer a password
// prompt and would hang instead.
func runWithSudo(ctx context.Context, dir string, args ...string) error {
	name, cmdArgs := args[0], args[1:]
	if os.Geteuid() != 0 {
		if err := checkSudo(ctx); err != nil {
			return err
		}
		name, cmdArgs = "sudo", append([]string{"-n"}, args...)
	}
	cmd := exec.CommandContext(ctx, name, cmdArgs...) //nolint:gosec
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	return nil
}

func downloadFile(ctx context.Context, url string) (string, error) {
	dldr, err := download.NewDownloader(ctx, 10*time.Minute, os.TempDir())
	if err != nil {
		return "", fmt.Errorf("create downloader: %w", err)
	}
	path, err := dldr.Download(ctx, url, 0600)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	return path, nil
}

func extractZip(zipPath, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	destAbs := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range zr.File {
		target := filepath.Join(destDir, f.Name) //nolint:gosec // traversal guarded below
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), destAbs) {
			return fmt.Errorf("illegal path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		writeErr := archive.WriteFile(target, rc, f.Mode())
		if cerr := rc.Close(); cerr != nil && writeErr == nil {
			writeErr = cerr
		}
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}
