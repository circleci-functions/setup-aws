# Changelog

All notable changes to setup-aws will be documented in this file. Each release's section is added
by its release PR (see [RELEASE.md](RELEASE.md)) and becomes its GitHub release notes.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-10-23

### What's Changed
* Stop passing a GitHub token to CI to download the toolkit by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/37


**Full Changelog**: https://github.com/circleci-functions/setup-aws/compare/v0.3.0...v0.4.0

## [0.3.0] - 2026-10-22

### What's Changed
* Update module golang.org/x/text to v0.41.0 [SECURITY] by @renovate[bot] in https://github.com/circleci-functions/setup-aws/pull/12
* Update module golang.org/x/mod to v0.40.0 [SECURITY] by @renovate[bot] in https://github.com/circleci-functions/setup-aws/pull/11
* Update toolkit to v0.3.0 by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/34
* Harden the installer and keep credentials out of argv and readable files by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/30
* Match the installed AWS CLI version exactly and replace v1 by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/31
* Check flags before installing and say what is ignored or missing by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/32
* Lead the README with the functions block and fill its gaps by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/33
* Update module golang.org/x/crypto to v0.56.0 [SECURITY] by @renovate[bot] in https://github.com/circleci-functions/setup-aws/pull/36

### New Contributors
* @renovate[bot] made their first contribution in https://github.com/circleci-functions/setup-aws/pull/12

**Full Changelog**: https://github.com/circleci-functions/setup-aws/compare/v0.2.0...v0.3.0

## [0.2.0] - 2026-10-13

### What's Changed
* Release by merging a release PR by @stiyyagura0901 in https://github.com/circleci-functions/setup-aws/pull/9


**Full Changelog**: https://github.com/circleci-functions/setup-aws/compare/v0.1.1...v0.2.0

## [0.1.1] - 2026-10-07

The last release made by pushing a tag, published as `v0.1.1-f8b628f`. Earlier releases are in
the repo's [tags](https://github.com/circleci-functions/setup-aws/tags).
