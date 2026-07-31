# setup-aws

Install the AWS CLI v2 and configure AWS credentials in `~/.aws/credentials`
and `~/.aws/config`, with optional OIDC-based role assumption.

## Commands

### `install`

Download and install the AWS CLI v2 for the current platform. Skipped when
the requested version is already present (unless `--override-installed`).

```yaml
steps:
  - run:
      name: Install AWS CLI
      command: circleci run setup-aws@<version> -- install --version 2.13.0
```

| Flag | Default | Description |
|------|---------|-------------|
| `--version` | `latest` | AWS CLI version to install (e.g. `2.13.0`, `latest`) |
| `--override-installed` | false | Force reinstall even if already installed |
| `--enable-aws-pager` | false | Skip disabling the AWS CLI output pager |
| `--install-dir` | `/usr/local/aws-cli` | Installation directory |
| `--binary-dir` | `/usr/local/bin` | Directory for the `aws` symlink |
| `--use-brew` | false | Install via Homebrew (macOS only) |

By default, `AWS_PAGER=""` is appended to `$BASH_ENV` so AWS CLI commands
never block waiting for pager input. Pass `--enable-aws-pager` to opt out.

### `setup`

Install the AWS CLI and configure credentials. Supports both static access
keys and OIDC-based role assumption.

```yaml
steps:
  - run:
      name: Set up AWS
      command: circleci run setup-aws@<version> -- setup
```

All `install` flags are also accepted. Additional flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--disable-aws-install` | false | Skip the install step (if already installed) |
| `--profile-name` | `default` | AWS credentials profile to configure |
| `--aws-access-key-id` | `$AWS_ACCESS_KEY_ID` | AWS access key ID |
| `--aws-secret-access-key` | `$AWS_SECRET_ACCESS_KEY` | AWS secret access key |
| `--region` | `$AWS_DEFAULT_REGION` | AWS region |
| `--configure-default-region` | true | Set the default region in `~/.aws/config` |
| `--configure-profile-region` | true | Set the region for the named profile |
| `--role-arn` | (empty) | Role ARN to assume via OIDC |
| `--role-session-name` | `$CIRCLE_JOB` | Session name for the assumed role |
| `--session-duration` | `3600` | Session duration in seconds |
| `--set-aws-env-vars` | true | Export assumed-role credentials to `$BASH_ENV` |

When `--role-arn` is set, an OIDC token is minted and
`sts:AssumeRoleWithWebIdentity` is called to obtain temporary credentials.

### `role-arn-setup`

Add a role ARN to an existing named profile in `~/.aws/config`. Requires the
`setup` command to have been run first to create the base credentials.

```yaml
steps:
  - run:
      name: Configure role ARN
      command: circleci run setup-aws@<version> -- role-arn-setup --profile-name deploy --role-arn arn:aws:iam::123:role/Deploy
```

| Flag | Default | Description |
|------|---------|-------------|
| `--profile-name` | (required) | Profile to configure |
| `--role-arn` | (required) | Role ARN for the profile |
| `--source-profile` | `default` | Source profile with base credentials |

## Platform support

Linux (amd64, arm64), Alpine Linux, and macOS. Windows is not yet supported.
