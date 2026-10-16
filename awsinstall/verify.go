package awsinstall

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
)

// signingKey is the AWS CLI team's PGP public key (fingerprint
// FB5D B77F D5C1 18B8 0511 ADA8 A631 0ACC 4672 475C), as published in the AWS
// CLI install guide. The Linux installer must carry a detached signature by it.
//
//go:embed aws-cli.asc
var signingKey string

// darwinSigner is the Apple Developer ID that signs the AWS CLI macOS package.
const darwinSigner = "Developer ID Installer: AMZN Mobile LLC (94KV3E626L)"

// verifyPGP checks that sigPath is a valid detached signature of path by
// signingKey.
func verifyPGP(path, sigPath string) error {
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(signingKey))
	if err != nil {
		return fmt.Errorf("read AWS CLI signing key: %w", err)
	}
	signed, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}
	defer signed.Close()
	sig, err := os.Open(sigPath) //nolint:gosec
	if err != nil {
		return err
	}
	defer sig.Close()

	if _, err := openpgp.CheckDetachedSignature(keyring, signed, sig, nil); err != nil {
		if errors.Is(err, pgperrors.ErrKeyExpired) {
			return fmt.Errorf("verify %s: the AWS CLI signing key built into setup-aws has expired; update setup-aws", filepath.Base(path))
		}
		return fmt.Errorf("verify %s against the AWS CLI signing key: %w", filepath.Base(path), err)
	}
	return nil
}

// verifyPkg checks with pkgutil that the macOS package is signed by Amazon's
// Apple Developer ID through a certificate chain Apple trusts.
func verifyPkg(ctx context.Context, pkgPath string) error {
	name := filepath.Base(pkgPath)
	out, err := exec.CommandContext(ctx, "pkgutil", "--check-signature", pkgPath).CombinedOutput() //nolint:gosec
	if err != nil {
		return fmt.Errorf("verify %s: pkgutil --check-signature: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	report := string(out)
	if !strings.Contains(report, "Status: signed by a developer certificate issued by Apple for distribution") ||
		!strings.Contains(report, "1. "+darwinSigner) {
		return fmt.Errorf("verify %s: not signed by %q:\n%s", name, darwinSigner, strings.TrimSpace(report))
	}
	return nil
}
