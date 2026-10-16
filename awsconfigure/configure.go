// Package awsconfigure configures AWS credentials by invoking the aws CLI.
package awsconfigure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Options controls which credentials and region values are written.
type Options struct {
	Profile                string
	AccessKeyID            string
	SecretAccessKey        string
	SessionToken           string // written only when non-empty
	Region                 string
	ConfigureDefaultRegion bool
	ConfigureProfileRegion bool
}

// Configure writes credentials to ~/.aws/credentials and region settings to
// ~/.aws/config for the given profile by calling `aws configure set`.
func Configure(ctx context.Context, opts Options) error {
	profile := opts.Profile
	if profile == "" {
		profile = "default"
	}

	steps := [][]string{
		{"aws_access_key_id", opts.AccessKeyID},
		{"aws_secret_access_key", opts.SecretAccessKey},
	}
	if opts.SessionToken != "" {
		steps = append(steps, []string{"aws_session_token", opts.SessionToken})
	}
	for _, s := range steps {
		if err := awsConfigure(ctx, s[0], s[1], profile); err != nil {
			return err
		}
	}

	return ConfigureRegion(ctx, profile, opts.Region, opts.ConfigureDefaultRegion, opts.ConfigureProfileRegion)
}

// ConfigureRegion writes region to ~/.aws/config as the default region and,
// separately, as the region of profile. It does nothing when region is empty.
func ConfigureRegion(ctx context.Context, profile, region string, setDefault, setProfile bool) error {
	if region == "" {
		return nil
	}
	if setDefault {
		if err := awsConfigure(ctx, "default.region", region, ""); err != nil {
			return err
		}
	}
	if setProfile {
		if err := awsConfigure(ctx, "region", region, profile); err != nil {
			return err
		}
	}
	return nil
}

// ConfigureRoleARN adds a role_arn and source_profile to a named profile in
// ~/.aws/config. The profile must already exist (created by Configure).
func ConfigureRoleARN(ctx context.Context, profile, roleARN, sourceProfile string) error {
	if err := awsConfigureProfile(ctx, profile, "role_arn", roleARN); err != nil {
		return err
	}
	return awsConfigureProfile(ctx, profile, "source_profile", sourceProfile)
}

// awsConfigure runs `aws configure set key value [--profile profile]`.
// When profile is empty, --profile is omitted (used for default.region).
func awsConfigure(ctx context.Context, key, value, profile string) error {
	args := []string{"configure", "set", key, value}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	cmd := exec.CommandContext(ctx, "aws", args...) //nolint:gosec
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws configure set %s: %w", key, err)
	}
	return nil
}

// awsConfigureProfile runs `aws configure set profile.PROFILE.key value`.
func awsConfigureProfile(ctx context.Context, profile, key, value string) error {
	fullKey := fmt.Sprintf("profile.%s.%s", profile, key)
	cmd := exec.CommandContext(ctx, "aws", "configure", "set", fullKey, value) //nolint:gosec
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws configure set %s: %w", fullKey, err)
	}
	return nil
}
