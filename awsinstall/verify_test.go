package awsinstall

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"gotest.tools/v3/assert"
)

func TestSigningKey_IsTheAWSCLITeamKey(t *testing.T) {
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(signingKey))
	assert.NilError(t, err)
	assert.Equal(t, len(keyring), 1)
	assert.Equal(t, fmt.Sprintf("%X", keyring[0].PrimaryKey.Fingerprint), "FB5DB77FD5C118B80511ADA8A6310ACC4672475C")
}

// TestSigningKey_NotExpiringSoon fails well before the embedded key expires,
// so it is refreshed from the AWS CLI install guide before installs break.
func TestSigningKey_NotExpiringSoon(t *testing.T) {
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(signingKey))
	assert.NilError(t, err)
	_, ok := keyring[0].SigningKey(time.Now().AddDate(0, 0, 60))
	assert.Assert(t, ok, "the AWS CLI signing key expires within 60 days; refresh awsinstall/aws-cli.asc from the AWS CLI install guide")
}

func TestVerifyPGP(t *testing.T) {
	key := useTestSigningKey(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "awscli.zip")
	assert.NilError(t, os.WriteFile(path, []byte("installer"), 0600))

	t.Run("valid signature", func(t *testing.T) {
		sig := filepath.Join(dir, "good.sig")
		assert.NilError(t, os.WriteFile(sig, detachSign(t, key, []byte("installer")), 0600))
		assert.NilError(t, verifyPGP(path, sig))
	})
	t.Run("signature of other content", func(t *testing.T) {
		sig := filepath.Join(dir, "bad.sig")
		assert.NilError(t, os.WriteFile(sig, detachSign(t, key, []byte("tampered")), 0600))
		assert.ErrorContains(t, verifyPGP(path, sig), "against the AWS CLI signing key")
	})
	t.Run("signed by another key", func(t *testing.T) {
		other, err := openpgp.NewEntity("other", "", "other@example.com", nil)
		assert.NilError(t, err)
		sig := filepath.Join(dir, "other.sig")
		assert.NilError(t, os.WriteFile(sig, detachSign(t, other, []byte("installer")), 0600))
		assert.ErrorContains(t, verifyPGP(path, sig), "against the AWS CLI signing key")
	})
	t.Run("missing signature", func(t *testing.T) {
		sig := filepath.Join(dir, "empty.sig")
		assert.NilError(t, os.WriteFile(sig, nil, 0600))
		assert.ErrorContains(t, verifyPGP(path, sig), "against the AWS CLI signing key")
	})
}

func TestVerifyPkg(t *testing.T) {
	signed := "Package \"AWSCLIV2.pkg\":\n" +
		"   Status: signed by a developer certificate issued by Apple for distribution\n" +
		"   Certificate Chain:\n" +
		"    1. " + darwinSigner + "\n" +
		"    2. Developer ID Certification Authority\n"
	tests := []struct {
		name    string
		output  string
		exit    int
		wantErr string
	}{
		{name: "signed by Amazon", output: signed},
		{
			name:    "signed by someone else",
			output:  strings.ReplaceAll(signed, darwinSigner, "Developer ID Installer: Someone Else (ABCDE12345)"),
			wantErr: "not signed by",
		},
		{
			name:    "untrusted certificate",
			output:  strings.ReplaceAll(signed, "a developer certificate issued by Apple for distribution", "untrusted certificate"),
			wantErr: "not signed by",
		},
		{name: "unsigned", output: "Package \"AWSCLIV2.pkg\":\n   Status: no signature\n", exit: 1, wantErr: "Status: no signature"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", writeStubPkgutil(t, tt.output, tt.exit)+string(os.PathListSeparator)+os.Getenv("PATH"))
			err := verifyPkg(context.Background(), "/tmp/AWSCLIV2.pkg")
			if tt.wantErr == "" {
				assert.NilError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// useTestSigningKey makes a fresh key the trusted signing key for the rest of
// the test and returns it.
func useTestSigningKey(t *testing.T) *openpgp.Entity {
	t.Helper()
	key, err := openpgp.NewEntity("test", "", "test@example.com", nil)
	assert.NilError(t, err)
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	assert.NilError(t, err)
	assert.NilError(t, key.Serialize(w))
	assert.NilError(t, w.Close())

	orig := signingKey
	signingKey = buf.String()
	t.Cleanup(func() { signingKey = orig })
	return key
}

// detachSign returns a binary detached signature of data by key.
func detachSign(t *testing.T, key *openpgp.Entity, data []byte) []byte {
	t.Helper()
	var sig bytes.Buffer
	assert.NilError(t, openpgp.DetachSign(&sig, key, bytes.NewReader(data), nil))
	return sig.Bytes()
}

// writeStubPkgutil writes a pkgutil stub that prints output and exits with
// code, and returns its directory.
func writeStubPkgutil(t *testing.T, output string, code int) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\ncat <<'EOF'\n%s\nEOF\nexit %d\n", output, code)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "pkgutil"), []byte(script), 0750))
	return dir
}
