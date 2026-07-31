// Command setup-aws installs the AWS CLI v2 and optionally configures AWS
// credentials (access key or OIDC-based role assumption) in ~/.aws/credentials
// and ~/.aws/config, and exports environment variables to $BASH_ENV.
//
// Supported platforms: linux, linux/alpine, darwin.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-aws/awsconfigure"
	"github.com/circleci-functions/setup-aws/awsinstall"
	"github.com/circleci-functions/setup-aws/awsoidc"
)

type cli struct {
	Install      installCmd      `cmd:"" default:"withargs" help:"Install the AWS CLI v2."`
	Setup        setupCmd        `cmd:"" help:"Install and configure AWS credentials."`
	RoleARNSetup roleARNSetupCmd `cmd:"role-arn-setup" help:"Add a role ARN to an existing named profile."`
}

type installCmd struct {
	Version           string `help:"AWS CLI version to install (e.g. 2.13.0, latest)." default:"latest"`
	OverrideInstalled bool   `name:"override-installed" help:"Force reinstall even if already installed."`
	EnableAWSPager    bool   `name:"enable-aws-pager" help:"Skip disabling the AWS CLI output pager."`
	InstallDir        string `name:"install-dir" help:"Installation directory for the AWS CLI." default:"/usr/local/aws-cli"`
	BinaryDir         string `name:"binary-dir" help:"Directory for the aws binary symlink." default:"/usr/local/bin"`
	UseBrew           bool   `name:"use-brew" help:"Install via Homebrew (macOS only)."`
	// hidden override for tests
	BaseURL string `name:"base-url" help:"Override download base URL." hidden:""`
}

func (c *installCmd) Run(ctx context.Context) error {
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
	return nil
}

type roleARNSetupCmd struct {
	ProfileName   string `name:"profile-name" help:"Profile to configure." required:""`
	RoleARN       string `name:"role-arn" help:"Role ARN for the profile." required:""`
	SourceProfile string `name:"source-profile" help:"Source profile with base credentials." default:"default"`
}

func (c *roleARNSetupCmd) Run(ctx context.Context) error {
	return awsconfigure.ConfigureRoleARN(ctx, c.ProfileName, c.RoleARN, c.SourceProfile)
}

type setupCmd struct {
	// install options
	Version           string `help:"AWS CLI version to install." default:"latest"`
	OverrideInstalled bool   `name:"override-installed" help:"Force reinstall even if already installed."`
	EnableAWSPager    bool   `name:"enable-aws-pager" help:"Skip disabling the AWS CLI output pager."`
	InstallDir        string `name:"install-dir" help:"Installation directory for the AWS CLI." default:"/usr/local/aws-cli"`
	BinaryDir         string `name:"binary-dir" help:"Directory for the aws binary symlink." default:"/usr/local/bin"`
	UseBrew           bool   `name:"use-brew" help:"Install via Homebrew (macOS only)."`
	DisableAWSInstall bool   `name:"disable-aws-install" help:"Skip the AWS CLI install step."`
	BaseURL           string `name:"base-url" help:"Override download base URL." hidden:""`
	// credential options
	ProfileName            string `name:"profile-name" help:"AWS credentials profile to configure." default:"default"`
	AWSAccessKeyID         string `name:"aws-access-key-id" help:"AWS access key ID." env:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey     string `name:"aws-secret-access-key" help:"AWS secret access key." env:"AWS_SECRET_ACCESS_KEY"`
	Region                 string `name:"region" help:"AWS region." env:"AWS_DEFAULT_REGION"`
	ConfigureDefaultRegion bool   `name:"configure-default-region" help:"Set the default region in ~/.aws/config." default:"true"`
	ConfigureProfileRegion bool   `name:"configure-profile-region" help:"Set the region for the named profile." default:"true"`
	// OIDC role assumption
	RoleARN         string `name:"role-arn" help:"Role ARN to assume via OIDC. Requires role-session-name."`
	RoleSessionName string `name:"role-session-name" help:"Session name for the assumed role." env:"CIRCLE_JOB"`
	SessionDuration int    `name:"session-duration" help:"Session duration in seconds." default:"3600"`
	SetAWSEnvVars   bool   `name:"set-aws-env-vars" help:"Export assumed-role credentials to $BASH_ENV." default:"true"`
}

func (c *setupCmd) Run(ctx context.Context) error {
	if !c.DisableAWSInstall {
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
	}

	// Resolve the credentials to use for configuration. Static key/secret are
	// the default; OIDC role assumption overrides all three when role-arn is set.
	accessKeyID := c.AWSAccessKeyID
	secretAccessKey := c.AWSSecretAccessKey
	var sessionToken string

	if c.RoleARN != "" {
		if c.RoleSessionName == "" {
			return fmt.Errorf("--role-session-name is required when --role-arn is set")
		}
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

func (c *setupCmd) assumeRole(ctx context.Context) (awsoidc.Credentials, error) {
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
		kong.Description("Install the AWS CLI v2 and configure AWS credentials.\n\nExample:\n  circleci run setup-aws@<version> -- --version 2.13.0"),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	return kCtx.Run()
}
