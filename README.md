# setup-aws

Installs the AWS CLI v2 for the rest of the job, and — when you ask it to —
configures AWS credentials in `~/.aws/credentials` and `~/.aws/config`.

Declare the function in a top-level `functions:` block, then call it as a step.
Each flag in the [table below](#flags) is a `with:` key, written without its
leading `--`:

```yaml
functions:
  setup-aws: github.com/circleci-functions/setup-aws@<version>

jobs:
  deploy:
    docker:
      - image: cimg/base:current
    steps:
      - setup-aws:
          with:
            version: 2.13.0
      - run: aws --version
```

`circleci function add setup-aws` writes the `functions:` entry for you.

Where the `functions:` block isn't enabled, such as on CircleCI Server, run it
with `circleci run` and the function's full name. Everything after `--` is
passed to the function:

```yaml
      - run: circleci run github.com/circleci-functions/setup-aws@<version> -- --version 2.13.0
```

Later steps pick up `aws` automatically. The install is skipped when the
requested version is already present (pass `--override-installed` to force a
reinstall).

By default `AWS_PAGER=""` is appended to `$BASH_ENV` so AWS CLI commands never
block waiting for pager input. Pass `--enable-aws-pager` to opt out.

## Choosing a version

`--version` takes an exact AWS CLI v2 version or `latest`:

```yaml
      - setup-aws:
          with:
            version: 2.13.0
```

On macOS you can install through Homebrew instead of the official installer with
`--use-brew`.

## Configuring credentials

Add `--configure-credentials` to configure AWS credentials after installing.
Without it, setup-aws only installs the CLI.

### Static access keys

By default the access key and secret are read from `$AWS_ACCESS_KEY_ID` and
`$AWS_SECRET_ACCESS_KEY`, and the region from `$AWS_DEFAULT_REGION`:

```yaml
      - setup-aws:
          with:
            configure-credentials: true
            region: us-east-1
```

Static keys are only written to `~/.aws/credentials`. Unlike assumed-role
credentials, they aren't exported to `$BASH_ENV`.

### OIDC role assumption

Set `--role-arn` to mint a CircleCI OIDC token and call
`sts:AssumeRoleWithWebIdentity` for temporary credentials. The assumed-role
credentials are also exported to `$BASH_ENV` (turn that off with
`--set-aws-env-vars=false`):

```yaml
      - setup-aws:
          with:
            configure-credentials: true
            role-arn: arn:aws:iam::123456789012:role/Deploy
            region: us-east-1
```

Before this works, AWS needs to trust CircleCI's tokens. Create an IAM OIDC
identity provider for `https://oidc.circleci.com/org/<organization-id>` with
your organization ID as the audience, and give the role a trust policy that
lets that provider call `sts:AssumeRoleWithWebIdentity`. setup-aws mints the
token with the `circleci` CLI, which every job has, and uses
`$CIRCLE_ORGANIZATION_ID` as the audience. See
[Using OpenID Connect tokens in jobs](https://circleci.com/docs/guides/permissions-authentication/openid-connect-tokens/).

`--role-session-name` defaults to `$CIRCLE_JOB`.

### Profile-based role assumption

Set `--source-profile` to write `role_arn` and `source_profile` into the named
profile instead of assuming the role here. The AWS CLI then assumes the role
itself, using the source profile's base credentials, on each call:

```yaml
      - setup-aws:
          with:
            configure-credentials: true
            profile-name: deploy
            role-arn: arn:aws:iam::123456789012:role/Deploy
            source-profile: default
```

setup-aws doesn't create the source profile. It must already hold
credentials, for example from an earlier setup-aws step that configured static
keys.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--version` | `latest` | AWS CLI version to install (e.g. `2.13.0`, `latest`) |
| `--override-installed` | false | Force reinstall even if already installed |
| `--enable-aws-pager` | false | Leave the AWS CLI output pager on instead of disabling it |
| `--install-dir` | `/usr/local/aws-cli` | Installation directory. Linux installer only; the macOS package always installs here |
| `--binary-dir` | `/usr/local/bin` | Directory for the `aws` symlink. Linux installer only; the macOS package always links here |
| `--use-brew` | false | Install via Homebrew (macOS only). Homebrew installs its current version and ignores `--version` |
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
