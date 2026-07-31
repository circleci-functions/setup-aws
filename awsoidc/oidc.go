// Package awsoidc handles OIDC-based role assumption for AWS.
package awsoidc

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Credentials holds temporary AWS credentials returned by AssumeRole.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// AssumeRoleOptions controls the STS AssumeRoleWithWebIdentity call.
type AssumeRoleOptions struct {
	RoleARN          string
	RoleSessionName  string
	WebIdentityToken string
	DurationSeconds  int32
	Region           string
}

// SanitizeSessionName replaces any character not in the ASCII set
// [A-Za-z0-9=,.@_-] with a hyphen and truncates to 64 characters,
// matching AWS STS session name requirements.
func SanitizeSessionName(name string) string {
	runes := []rune(name)
	for i, r := range runes {
		if !isAllowedSessionRune(r) {
			runes[i] = '-'
		}
	}
	if len(runes) > 64 {
		runes = runes[:64]
	}
	return string(runes)
}

func isAllowedSessionRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		strings.ContainsRune("=,.@_-", r)
}

// GetOIDCToken mints a CircleCI OIDC token via `circleci run oidc get`.
// It retries up to 3 times with a 1-second delay between attempts.
func GetOIDCToken(ctx context.Context, orgID string) (string, error) {
	claims := fmt.Sprintf(`{"aud":%q}`, orgID)
	var lastErr error
	for i := range 3 {
		if i > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Second):
			}
		}
		out, err := exec.CommandContext(ctx, "circleci", "run", "oidc", "get", "--claims", claims).Output() //nolint:gosec
		if err != nil {
			lastErr = err
			continue
		}
		if token := strings.TrimSpace(string(out)); token != "" {
			return token, nil
		}
		lastErr = fmt.Errorf("empty token returned")
	}
	return "", fmt.Errorf("get OIDC token after 3 attempts: %w", lastErr)
}

// stsClient is the subset of sts.Client used by assumeRole. Defined as an
// interface so tests can inject a fake.
type stsClient interface {
	AssumeRoleWithWebIdentity(context.Context, *sts.AssumeRoleWithWebIdentityInput, ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error)
}

// AssumeRole calls STS AssumeRoleWithWebIdentity and returns temporary credentials.
func AssumeRole(ctx context.Context, opts AssumeRoleOptions) (Credentials, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(opts.Region))
	if err != nil {
		return Credentials{}, fmt.Errorf("load AWS config: %w", err)
	}
	return assumeRole(ctx, sts.NewFromConfig(cfg), opts)
}

func assumeRole(ctx context.Context, client stsClient, opts AssumeRoleOptions) (Credentials, error) {
	duration := opts.DurationSeconds
	resp, err := client.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &opts.RoleARN,
		RoleSessionName:  &opts.RoleSessionName,
		WebIdentityToken: &opts.WebIdentityToken,
		DurationSeconds:  &duration,
	})
	if err != nil {
		return Credentials{}, fmt.Errorf("assume role with web identity: %w", err)
	}
	creds := resp.Credentials
	if creds == nil {
		return Credentials{}, fmt.Errorf("STS returned empty credentials")
	}
	return Credentials{
		AccessKeyID:     toString(creds.AccessKeyId),
		SecretAccessKey: toString(creds.SecretAccessKey),
		SessionToken:    toString(creds.SessionToken),
	}, nil
}

func toString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
