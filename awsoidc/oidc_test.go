package awsoidc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	stypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"gotest.tools/v3/assert"
)

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"replaces spaces and punctuation", "my job/step 1:run", "my-job-step-1-run"},
		{"preserves valid ASCII chars", "MyJob_session=1,role@org-prod", "MyJob_session=1,role@org-prod"},
		{"truncates to 64 runes", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"replaces non-ASCII letters", "deploy-тест", "deploy-----"},
		{"truncates multi-byte at rune boundary", "あ" + strings.Repeat("a", 64), strings.Repeat("-", 1) + strings.Repeat("a", 63)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSessionName(tt.input)
			assert.Equal(t, got, tt.want)
			assert.Assert(t, len([]rune(got)) <= 64, "result exceeds 64 runes: %d", len([]rune(got)))
		})
	}
}

func TestGetOIDCToken_Success(t *testing.T) {
	stubDir := writeStubCircleCI(t, "eyJhbGciOiJSUzI1NiJ9.test.token")
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))

	token, err := GetOIDCToken(context.Background(), "org-uuid-123")
	assert.NilError(t, err)
	assert.Equal(t, token, "eyJhbGciOiJSUzI1NiJ9.test.token")
}

func TestGetOIDCToken_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir — circleci not found
	start := time.Now()
	_, err := GetOIDCToken(context.Background(), "org-uuid-123")
	assert.ErrorContains(t, err, "the circleci CLI is needed to mint the token and isn't on PATH")
	assert.Assert(t, time.Since(start) < time.Second, "a missing CLI should not be retried")
}

func TestGetOIDCToken_IncludesStderr(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Error: OIDC tokens are only available inside a job' >&2\nexit 1\n"
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "circleci"), []byte(script), 0750))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	_, err := GetOIDCToken(context.Background(), "org-uuid-123")
	assert.Error(t, err, "get OIDC token after 3 attempts: exit status 1: Error: OIDC tokens are only available inside a job")
}

func TestAssumeRole_Success(t *testing.T) {
	keyID := "ASIA..."
	secret := "sekrit"
	token := "session-token"

	creds, err := assumeRole(context.Background(), &fakeSTSClient{
		keyID:  keyID,
		secret: secret,
		token:  token,
	}, AssumeRoleOptions{
		RoleARN:          "arn:aws:iam::123:role/MyRole",
		RoleSessionName:  "test-session",
		WebIdentityToken: "oidc-token",
		DurationSeconds:  3600,
	})
	assert.NilError(t, err)
	assert.Equal(t, creds.AccessKeyID, keyID)
	assert.Equal(t, creds.SecretAccessKey, secret)
	assert.Equal(t, creds.SessionToken, token)
}

func TestAssumeRole_PropagatesError(t *testing.T) {
	_, err := assumeRole(context.Background(), &fakeSTSClient{
		err: fmt.Errorf("STS unavailable"),
	}, AssumeRoleOptions{
		RoleARN:         "arn:...",
		RoleSessionName: "s",
	})
	assert.ErrorContains(t, err, "STS unavailable")
}

// fakeSTSClient satisfies the stsClient interface for tests.
type fakeSTSClient struct {
	keyID, secret, token string
	err                  error
}

func (f *fakeSTSClient) AssumeRoleWithWebIdentity(_ context.Context, _ *sts.AssumeRoleWithWebIdentityInput, _ ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &sts.AssumeRoleWithWebIdentityOutput{
		Credentials: &stypes.Credentials{ //nolint:exhaustruct
			AccessKeyId:     &f.keyID,
			SecretAccessKey: &f.secret,
			SessionToken:    &f.token,
		},
	}, nil
}

// writeStubCircleCI writes a stub circleci script that outputs the given token.
func writeStubCircleCI(t *testing.T, token string) string {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\n", token)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "circleci"), []byte(script), 0750))
	return dir
}
