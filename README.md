# setup-aws

Installs the AWS CLI v2 for the rest of the job, and — when you ask it to —
configures AWS credentials in `~/.aws/credentials` and `~/.aws/config`.

```yaml
steps:
  - run:
      name: Install AWS CLI
      command: circleci run setup-aws@<version> -- --version 2.13.0
  - run: aws --version
```

Later steps pick up `aws` automatically. The install is skipped when the
requested version is already present (pass `--override-installed` to force a
reinstall).

By default `AWS_PAGER=""` is appended to `$BASH_ENV` so AWS CLI commands never
block waiting for pager input. Pass `--enable-aws-pager` to opt out.

## Choosing a version

`--version` takes an exact AWS CLI v2 version or `latest`:

```yaml
- run: circleci run setup-aws@<version> -- --version 2.13.0
```

On macOS you can install through Homebrew instead of the official installer with
`--use-brew`.

The official installers are verified before they run. On Linux, the zip's PGP
signature must match the AWS CLI team key published in the
[AWS CLI install guide](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html).
On macOS, `pkgutil --check-signature` must show the package signed by Amazon's
Apple Developer ID. Alpine (`apk`) and `--use-brew` installs are left to the
package manager.

## Configuring credentials

Add `--configure-credentials` to configure AWS credentials after installing.
Without it, setup-aws only installs the CLI.

### Static access keys

By default the access key and secret are read from `$AWS_ACCESS_KEY_ID` and
`$AWS_SECRET_ACCESS_KEY`, and the region from `$AWS_DEFAULT_REGION`:

```yaml
- run:
    name: Set up AWS
    command: circleci run setup-aws@<version> -- --configure-credentials --region us-east-1
```

### OIDC role assumption

Set `--role-arn` to mint a CircleCI OIDC token and call
`sts:AssumeRoleWithWebIdentity` for temporary credentials. The assumed-role
credentials are also exported to `$BASH_ENV` (turn that off with
`--set-aws-env-vars=false`):

```yaml
- run:
    name: Assume role via OIDC
    command: >-
      circleci run setup-aws@<version> --
      --configure-credentials
      --role-arn arn:aws:iam::123456789012:role/Deploy
      --region us-east-1
```

`--role-session-name` defaults to `$CIRCLE_JOB`.

### Profile-based role assumption

Set `--source-profile` to write `role_arn` and `source_profile` into the named
profile instead of assuming the role here. The AWS CLI then assumes the role
itself, using the source profile's base credentials, on each call:

```yaml
- run:
    name: Configure deploy profile
    command: >-
      circleci run setup-aws@<version> --
      --configure-credentials
      --profile-name deploy
      --role-arn arn:aws:iam::123456789012:role/Deploy
      --source-profile default
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--version` | `latest` | AWS CLI version to install (e.g. `2.13.0`, `latest`) |
| `--override-installed` | false | Force reinstall even if already installed |
| `--enable-aws-pager` | false | Skip disabling the AWS CLI output pager |
| `--install-dir` | `/usr/local/aws-cli` | Installation directory |
| `--binary-dir` | `/usr/local/bin` | Directory for the `aws` symlink |
| `--use-brew` | false | Install via Homebrew (macOS only) |
| `--configure-credentials` | false | Configure credentials after installing |
| `--profile-name` | `default` | AWS credentials profile to configure |
| `--aws-access-key-id` | `$AWS_ACCESS_KEY_ID` | AWS access key ID |
| `--aws-secret-access-key` | `$AWS_SECRET_ACCESS_KEY` | AWS secret access key |
| `--region` | `$AWS_DEFAULT_REGION` | AWS region |
| `--configure-default-region` | true | Set the default region in `~/.aws/config` |
| `--configure-profile-region` | true | Set the region for the named profile |
| `--role-arn` | (empty) | Role ARN to assume via OIDC, or to write with `--source-profile` |
| `--role-session-name` | `$CIRCLE_JOB` | Session name for the assumed role |
| `--session-duration` | `3600` | Session duration in seconds |
| `--set-aws-env-vars` | true | Export assumed-role credentials to `$BASH_ENV` |
| `--source-profile` | (empty) | Source profile for `--role-arn`; writes `role_arn`/`source_profile` instead of assuming via OIDC |

Credential flags apply only when `--configure-credentials` is set.

## Platform support

Linux (amd64, arm64), Alpine Linux, and macOS. Windows is not yet supported.

On Linux and macOS the official installer runs as root. When the step isn't
already root, setup-aws uses `sudo -n` and fails if sudo is missing or asks for
a password. `--use-brew` on macOS doesn't need root.
