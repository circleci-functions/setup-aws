// Command setup-aws installs the AWS CLI v2 and, when --configure-credentials
// is set, configures AWS credentials (static access key or OIDC-based role
// assumption) in ~/.aws/credentials and ~/.aws/config, exporting environment
// variables to $BASH_ENV.
//
// Supported platforms: linux, linux/alpine, darwin.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-aws/awsconfigure"
	"github.com/circleci-functions/setup-aws/awsinstall"
	"github.com/circleci-functions/setup-aws/awsoidc"
)

type cli struct {
	// install options
	Version           string `help:"AWS CLI version to install (e.g. 2.13.0, latest)." default:"latest" example:"2.13.0"`
	OverrideInstalled bool   `name:"override-installed" help:"Force reinstall even if already installed."`
	EnableAWSPager    bool   `name:"enable-aws-pager" help:"Skip disabling the AWS CLI output pager."`
	InstallDir        string `name:"install-dir" help:"Installation directory for the AWS CLI." default:"/usr/local/aws-cli"`
	BinaryDir         string `name:"binary-dir" help:"Directory for the aws binary symlink." default:"/usr/local/bin"`
	UseBrew           bool   `name:"use-brew" help:"Install via Homebrew (macOS only)."`
	// hidden override for tests
	BaseURL string `name:"base-url" help:"Override download base URL." hidden:""`

	// ConfigureCredentials gates all credential and region configuration below.
	// When false, setup-aws only installs the AWS CLI.
	ConfigureCredentials bool `name:"configure-credentials" help:"Configure AWS credentials after installing the CLI."`

	// credential options (used when --configure-credentials is set)
	ProfileName            string `name:"profile-name" help:"AWS credentials profile to configure." default:"default"`
	AWSAccessKeyID         string `name:"aws-access-key-id" help:"AWS access key ID." env:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey     string `name:"aws-secret-access-key" help:"AWS secret access key." env:"AWS_SECRET_ACCESS_KEY"`
	Region                 string `name:"region" help:"AWS region." env:"AWS_DEFAULT_REGION"`
	ConfigureDefaultRegion bool   `name:"configure-default-region" help:"Set the default region in ~/.aws/config." default:"true"`
	ConfigureProfileRegion bool   `name:"configure-profile-region" help:"Set the region for the named profile." default:"true"`

	// OIDC role assumption (used when --configure-credentials is set)
	RoleARN         string `name:"role-arn" help:"Role ARN to assume via OIDC. Requires role-session-name."`
	RoleSessionName string `name:"role-session-name" help:"Session name for the assumed role." env:"CIRCLE_JOB"`
	SessionDuration int    `name:"session-duration" help:"Session duration in seconds." default:"3600"`
	SetAWSEnvVars   bool   `name:"set-aws-env-vars" help:"Export assumed-role credentials to $BASH_ENV." default:"true"`

	// Profile-based role assumption (used when --configure-credentials is set).
	// Setting --source-profile selects this mode: rather than assuming the role
	// via OIDC, --role-arn, --source-profile and --region are written to the
	// named profile so the AWS CLI assumes the role itself on each call.
	SourceProfile string `name:"source-profile" help:"Source profile for --role-arn. Setting this writes role_arn/source_profile/region to the profile instead of assuming the role via OIDC."`
}

// Validate checks flag combinations. Kong runs it before Run, so a bad
// combination fails before anything is installed or written.
func (c *cli) Validate(kctx *kong.Context) error {
	c.warnIgnoredFlags(passedFlags(kctx))
	if !c.ConfigureCredentials {
		return nil
	}
	if c.SourceProfile != "" {
		if c.RoleARN == "" {
			return fmt.Errorf("--role-arn is required when --source-profile is set")
		}
		return nil
	}
	if c.RoleARN != "" && c.RoleSessionName == "" {
		return fmt.Errorf("--role-session-name is required when --role-arn is set")
	}
	return nil
}

func (c *cli) Run(ctx context.Context) error {
	if err := awsinstall.Ensure(ctx, awsinstall.EnsureOptions{
		Version:           c.Version,
		OverrideInstalled: c.OverrideInstalled,
		InstallDir:        c.InstallDir,
		BinaryDir:         c.BinaryDir,
		UseBrew:           c.UseBrew,
		BaseURL:           c.BaseURL,
	}); err != nil {
		return err
	}

	if !c.EnableAWSPager && os.Getenv("AWS_PAGER") == "" {
		if err := disableAWSPager(); err != nil {
			return fmt.Errorf("disable aws pager: %w", err)
		}
	}

	if !c.ConfigureCredentials {
		return nil
	}
	return c.configureCredentials(ctx)
}

func (c *cli) configureCredentials(ctx context.Context) error {
	// Profile-based role assumption: write role_arn/source_profile to the named
	// profile so the AWS CLI assumes the role itself, rather than assuming it
	// here via OIDC.
	if c.SourceProfile != "" {
		if err := awsconfigure.ConfigureRoleARN(ctx, c.ProfileName, c.RoleARN, c.SourceProfile); err != nil {
			return err
		}
		return awsconfigure.ConfigureRegion(ctx, c.ProfileName, c.Region, c.ConfigureDefaultRegion, c.ConfigureProfileRegion)
	}

	// Resolve the credentials to use for configuration. Static key/secret are
	// the default; OIDC role assumption overrides all three when role-arn is set.
	accessKeyID := c.AWSAccessKeyID
	secretAccessKey := c.AWSSecretAccessKey
	var sessionToken string

	if c.RoleARN != "" {
		creds, err := c.assumeRole(ctx)
		if err != nil {
			return err
		}
		accessKeyID = creds.AccessKeyID
		secretAccessKey = creds.SecretAccessKey
		sessionToken = creds.SessionToken
		if c.SetAWSEnvVars {
			if err := exportCredsToEnv(creds); err != nil {
				return fmt.Errorf("export credentials to BASH_ENV: %w", err)
			}
		}
	}

	return awsconfigure.Configure(ctx, awsconfigure.Options{
		Profile:                c.ProfileName,
		AccessKeyID:            accessKeyID,
		SecretAccessKey:        secretAccessKey,
		SessionToken:           sessionToken,
		Region:                 c.Region,
		ConfigureDefaultRegion: c.ConfigureDefaultRegion,
		ConfigureProfileRegion: c.ConfigureProfileRegion,
	})
}

func (c *cli) assumeRole(ctx context.Context) (awsoidc.Credentials, error) {
	orgID := os.Getenv("CIRCLE_ORGANIZATION_ID")
	token, err := awsoidc.GetOIDCToken(ctx, orgID)
	if err != nil {
		return awsoidc.Credentials{}, fmt.Errorf("get OIDC token: %w", err)
	}

	sessionName := awsoidc.SanitizeSessionName(c.RoleSessionName)
	return awsoidc.AssumeRole(ctx, awsoidc.AssumeRoleOptions{
		RoleARN:          c.RoleARN,
		RoleSessionName:  sessionName,
		WebIdentityToken: token,
		DurationSeconds:  int32(c.SessionDuration), //nolint:gosec
		Region:           c.Region,
	})
}

// exportCredsToEnv writes temporary AWS credentials to $BASH_ENV so subsequent
// steps can use them without re-assuming the role.
func exportCredsToEnv(creds awsoidc.Credentials) error {
	bashEnv := os.Getenv("BASH_ENV")
	if bashEnv == "" {
		return nil
	}
	f, err := os.OpenFile(bashEnv, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666) //nolint:gosec
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f,
		"export AWS_ACCESS_KEY_ID=%q\nexport AWS_SECRET_ACCESS_KEY=%q\nexport AWS_SESSION_TOKEN=%q\n",
		creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken,
	)
	return err
}

// disableAWSPager writes export AWS_PAGER="" to $BASH_ENV so subsequent steps
// don't get paged output.
func disableAWSPager() error {
	bashEnv := os.Getenv("BASH_ENV")
	if bashEnv == "" {
		return nil
	}
	f, err := os.OpenFile(bashEnv, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666) //nolint:gosec
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, `export AWS_PAGER=""`)
	return err
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, _ *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-aws"),
		kong.Description("Install the AWS CLI v2 and optionally configure AWS credentials."),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	return kCtx.Run()
}

// credentialFlags are the flags that only apply with --configure-credentials.
var credentialFlags = []string{
	"profile-name", "aws-access-key-id", "aws-secret-access-key", "region",
	"configure-default-region", "configure-profile-region", "role-arn",
	"role-session-name", "session-duration", "set-aws-env-vars", "source-profile",
}

// sourceProfileIgnoredFlags are the credential flags that --source-profile
// makes no-ops, since it only writes role_arn, source_profile and region.
var sourceProfileIgnoredFlags = []string{
	"aws-access-key-id", "aws-secret-access-key", "role-session-name",
	"session-duration", "set-aws-env-vars",
}

// warnIgnoredFlags warns about flags the user passed that this combination
// ignores. It looks only at flags on the command line: the key, region and
// session name flags default from env vars a CircleCI job or context commonly
// sets, and those shouldn't trigger a warning.
func (c *cli) warnIgnoredFlags(passed map[string]bool) {
	var ignored []string
	var reason string
	switch {
	case !c.ConfigureCredentials:
		ignored, reason = filterPassed(credentialFlags, passed), "without --configure-credentials"
	case c.SourceProfile != "":
		ignored, reason = filterPassed(sourceProfileIgnoredFlags, passed), "with --source-profile, which only writes role_arn, source_profile and region"
	}
	if len(ignored) > 0 {
		fmt.Fprintf(os.Stderr, "setup-aws: warning: ignoring %s %s\n", strings.Join(ignored, ", "), reason)
	}
}

// passedFlags returns the names of the flags given on the command line, as
// opposed to those filled in from defaults or env vars.
func passedFlags(kctx *kong.Context) map[string]bool {
	passed := map[string]bool{}
	for _, p := range kctx.Path {
		if p.Flag != nil {
			passed[p.Flag.Name] = true
		}
	}
	return passed
}

// filterPassed returns the entries of names that were passed, as flags.
func filterPassed(names []string, passed map[string]bool) []string {
	var out []string
	for _, n := range names {
		if passed[n] {
			out = append(out, "--"+n)
		}
	}
	return out
}
