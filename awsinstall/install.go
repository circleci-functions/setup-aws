// Package awsinstall handles installing the AWS CLI v2 on the current platform.
package awsinstall

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/circleci-functions/toolkit/archive"
	"github.com/circleci-functions/toolkit/bashenv"
	"github.com/circleci-functions/toolkit/download"
	"github.com/circleci-functions/toolkit/pathlink"
)

// DefaultBaseURL is the base download URL for AWS CLI installers.
const DefaultBaseURL = "https://awscli.amazonaws.com"

const (
	defaultInstallDir = "/usr/local/aws-cli"
	defaultBinaryDir  = "/usr/local/bin"
)

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

// versionPattern matches the full X.Y.Z versions AWS publishes installers for.
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// validateVersion checks version is "latest" or a full X.Y.Z version. AWS only
// publishes installers for full versions, so anything else can't download.
func validateVersion(version string) error {
	if version == "latest" || versionPattern.MatchString(version) {
		return nil
	}
	return fmt.Errorf("--version must be \"latest\" or a full AWS CLI version like 2.13.0, got %q", version)
}

// IsInstalled reports whether the AWS CLI is installed. With version "latest",
// any installed v2 satisfies the check, so a v1 gets replaced rather than
// accepted; otherwise the installed version must equal version.
func IsInstalled(version string) bool {
	out, err := exec.Command("aws", "--version").Output() //nolint:gosec
	if err != nil {
		return false
	}
	installed, ok := installedVersion(string(out))
	if !ok {
		return false
	}
	if version == "latest" {
		return strings.HasPrefix(installed, "2.")
	}
	return installed == version
}

// installedVersion returns the version in the "aws-cli/X.Y.Z" token of
// `aws --version` output, which also names the Python, OS and botocore versions.
func installedVersion(out string) (string, bool) {
	for _, field := range strings.Fields(out) {
		if v, ok := strings.CutPrefix(field, "aws-cli/"); ok {
			return v, true
		}
	}
	return "", false
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
	if err := validateVersion(opts.Version); err != nil {
		return err
	}
	if opts.BaseURL == "" {
		opts.BaseURL = DefaultBaseURL
	}
	if opts.InstallDir == "" {
		opts.InstallDir = defaultInstallDir
	}
	if opts.BinaryDir == "" {
		opts.BinaryDir = defaultBinaryDir
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
		if ignored := alpineIgnoredFlags(opts); len(ignored) > 0 {
			fmt.Fprintf(os.Stderr, "setup-aws: warning: on Alpine the AWS CLI is installed with apk, which ignores %s\n", strings.Join(ignored, ", "))
		}
		return installAlpine(ctx, defaultBinaryDir)
	case platformLinux:
		return installLinux(ctx, opts)
	case platformDarwin:
		return installDarwin(ctx, opts)
	default:
		return fmt.Errorf("setup-aws: unsupported platform")
	}
}

// alpineIgnoredFlags returns the flags set away from their defaults that apk
// can't honour. The official installer is glibc-only, so apk is the only way to
// install on musl, and it installs the packaged version where it chooses.
func alpineIgnoredFlags(opts EnsureOptions) []string {
	var ignored []string
	if opts.Version != "latest" {
		ignored = append(ignored, "--version")
	}
	if opts.InstallDir != defaultInstallDir {
		ignored = append(ignored, "--install-dir")
	}
	if opts.BinaryDir != defaultBinaryDir {
		ignored = append(ignored, "--binary-dir")
	}
	if opts.UseBrew {
		ignored = append(ignored, "--use-brew")
	}
	if opts.BaseURL != DefaultBaseURL {
		ignored = append(ignored, "--base-url")
	}
	return ignored
}

// installAlpine installs the AWS CLI with apk. If it then fails to start, it
// puts a wrapper in wrapperDir that runs it against Alpine's own libpython:
// python:*-alpine images put theirs in /usr/local/lib, where it is found first,
// and the aws-cli package can crash against an older one.
func installAlpine(ctx context.Context, wrapperDir string) error {
	cmd := exec.CommandContext(ctx, "apk", "add", "--no-cache", "aws-cli")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apk add aws-cli: %w", err)
	}

	aws, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("apk add aws-cli put no aws on PATH: %w", err)
	}
	out, err := exec.CommandContext(ctx, aws, "--version").CombinedOutput() //nolint:gosec
	if err == nil {
		return nil
	}
	failed := fmt.Errorf("aws --version failed after apk add aws-cli: %s", strings.TrimSpace(string(out)))

	wrapper := filepath.Join(wrapperDir, "aws")
	if aws == wrapper {
		return failed
	}
	script := fmt.Sprintf("#!/bin/sh\nLD_LIBRARY_PATH=/usr/lib exec '%s' \"$@\"\n", aws)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil { //nolint:gosec // an executable on PATH
		return failed
	}
	if exec.CommandContext(ctx, "aws", "--version").Run() != nil {
		_ = os.Remove(wrapper)
		return failed
	}
	fmt.Fprintf(os.Stderr, "setup-aws: %s fails to start with this image's libpython, so %s runs it with Alpine's\n", aws, wrapper)
	return nil
}

func installLinux(ctx context.Context, opts EnsureOptions) error {
	elevate, err := chooseLinuxDirs(ctx, &opts)
	if err != nil {
		return err
	}

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}

	url := buildLinuxURL(opts.BaseURL, opts.Version, arch)
	fmt.Fprintf(os.Stderr, "setup-aws: downloading %s\n", url)

	workDir, err := os.MkdirTemp("", "setup-aws-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	zipPath, err := downloadFile(ctx, workDir, url)
	if err != nil {
		return err
	}
	sigPath, err := downloadFile(ctx, workDir, url+".sig")
	if err != nil {
		return err
	}
	if err := verifyPGP(zipPath, sigPath); err != nil {
		return err
	}

	extractDir := filepath.Join(workDir, "extract")
	if err := extractZip(zipPath, extractDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	args := []string{"./aws/install", "-i", opts.InstallDir, "-b", opts.BinaryDir}
	if opts.OverrideInstalled {
		args = append(args, "--update")
	}
	if err := run(ctx, extractDir, elevate, args...); err != nil {
		return err
	}
	return ensureOnPath(opts.BinaryDir)
}

// chooseLinuxDirs reports whether installing into opts' directories needs
// sudo. When it would and sudo can't be used, it moves the default directories
// to ~/.local/aws-cli and ~/.local/bin instead.
func chooseLinuxDirs(ctx context.Context, opts *EnsureOptions) (elevate bool, err error) {
	if os.Geteuid() == 0 || (pathlink.Writable(opts.InstallDir) && pathlink.Writable(opts.BinaryDir)) {
		return false, nil
	}
	sudoErr := checkSudo(ctx)
	if sudoErr == nil {
		return true, nil
	}
	if opts.InstallDir != defaultInstallDir || opts.BinaryDir != defaultBinaryDir {
		return false, sudoErr
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, sudoErr
	}
	opts.InstallDir = filepath.Join(home, ".local", "aws-cli")
	opts.BinaryDir = filepath.Join(home, ".local", "bin")
	fmt.Fprintf(os.Stderr, "setup-aws: no root or passwordless sudo to install into %s; installing into %s instead\n", defaultInstallDir, opts.InstallDir)
	return false, nil
}

// ensureOnPath puts dir on PATH for the rest of this step and, through
// $BASH_ENV, for later steps, unless it is there already.
func ensureOnPath(dir string) error {
	path := os.Getenv("PATH")
	if slices.Contains(pathlink.SearchDirs(path), dir) {
		return nil
	}
	if err := bashenv.AddToPath(dir); err != nil {
		return fmt.Errorf("put %s on PATH for later steps: %w", dir, err)
	}
	return os.Setenv("PATH", dir+string(os.PathListSeparator)+path)
}

func installDarwin(ctx context.Context, opts EnsureOptions) error {
	if opts.UseBrew {
		return installDarwinBrew(ctx, opts.OverrideInstalled)
	}

	url := buildDarwinURL(opts.BaseURL, opts.Version)
	fmt.Fprintf(os.Stderr, "setup-aws: downloading %s\n", url)

	workDir, err := os.MkdirTemp("", "setup-aws-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	pkgPath, err := downloadFile(ctx, workDir, url)
	if err != nil {
		return err
	}
	if err := verifyPkg(ctx, pkgPath); err != nil {
		return err
	}

	return runWithSudo(ctx, "installer", "-pkg", pkgPath, "-target", "/")
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
func runWithSudo(ctx context.Context, args ...string) error {
	root := os.Geteuid() == 0
	if !root {
		if err := checkSudo(ctx); err != nil {
			return err
		}
	}
	return run(ctx, "", !root, args...)
}

// run runs args in dir, through sudo -n when sudo is set, sending their output
// to stderr.
func run(ctx context.Context, dir string, sudo bool, args ...string) error {
	name, cmdArgs := args[0], args[1:]
	if sudo {
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

// downloadFile downloads url into dir. dir must be a fresh directory created
// for this run: the downloader reuses any file already at the target path, so a
// shared directory like os.TempDir() would let another user plant the installer.
func downloadFile(ctx context.Context, dir, url string) (string, error) {
	dldr, err := download.NewDownloader(ctx, 10*time.Minute, dir)
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
