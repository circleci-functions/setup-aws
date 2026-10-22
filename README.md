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
reinstall). For `latest`, any AWS CLI v2 already on `PATH` counts; a v1 is
replaced with v2.

By default `AWS_PAGER=""` is appended to `$BASH_ENV`, once per job, so AWS CLI
commands never block waiting for pager input. Pass `--enable-aws-pager` to opt
out. If `$BASH_ENV` isn't set, setup-aws warns and leaves the pager on.

## Choosing a version

`--version` takes `latest` or a full AWS CLI v2 version. Anything else, such as
`2.13`, is rejected before installing. An exact version skips the install only
when that exact version is already installed:

```yaml
- run: circleci run setup-aws@<version> -- --version 2.13.0
```

On Alpine the official installer can't run (it needs glibc), so setup-aws runs
`apk add aws-cli` and gets whatever version the package repository has.
`--version`, `--install-dir`, `--binary-dir` and `--use-brew` are ignored there,
with a warning when you set them.

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

The access key, secret and session token are written straight to
`~/.aws/credentials` (or `$AWS_SHARED_CREDENTIALS_FILE`) at mode 0600, so they
never appear on a command line. Region and role settings go through
`aws configure set`.

### Static access keys

By default the access key and secret are read from `$AWS_ACCESS_KEY_ID` and
`$AWS_SECRET_ACCESS_KEY`, and the region from `$AWS_DEFAULT_REGION`. If either
key is empty, setup-aws fails before installing rather than writing an empty
profile:

```yaml
- run:
    name: Set up AWS
    command: circleci run setup-aws@<version> -- --configure-credentials --region us-east-1
```

### OIDC role assumption

Set `--role-arn` to mint a CircleCI OIDC token and call
`sts:AssumeRoleWithWebIdentity` for temporary credentials. The assumed-role
credentials are also exported to `$BASH_ENV` (turn that off with
`--set-aws-env-vars=false`). setup-aws fails if `$BASH_ENV` isn't set, rather
than leaving later steps without them:

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

The exported credentials are plain text in `$BASH_ENV`, which setup-aws keeps
at mode 0600. CircleCI only masks project and context secrets, not values
minted during the job, so a later `env` or `set -x` prints them. Pass
`--set-aws-env-vars=false` if later steps only need `~/.aws/credentials`.

### Profile-based role assumption

Set `--source-profile` to write `role_arn`, `source_profile` and `--region` into
the named profile instead of assuming the role here. The AWS CLI then assumes
the role itself, using the source profile's base credentials, on each call. The
key, session and `--set-aws-env-vars` flags don't apply in this mode, and
setup-aws warns if you pass them:

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
| `--source-profile` | (empty) | Source profile for `--role-arn`; writes `role_arn`/`source_profile`/`region` instead of assuming via OIDC |

Credential flags apply only when `--configure-credentials` is set. setup-aws
warns about any you pass without it. Values that only come from env vars, such
as `$AWS_DEFAULT_REGION`, don't trigger the warning.

## Platform support

Linux (amd64, arm64), Alpine Linux, and macOS. Windows is not yet supported.

On Linux and macOS the official installer runs as root. When the step isn't
already root, setup-aws uses `sudo -n` and fails if sudo is missing or asks for
a password. `--use-brew` on macOS doesn't need root.
