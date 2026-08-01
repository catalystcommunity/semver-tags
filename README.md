# semver-tags

Do an analysis of a repo or its subdirs and generate git tags for semantic versioning based on conventional commits. Oh, and release notes generated.

## Features
- Analyze a repository or a set of paths and generate semantic version tags.
- Group more than one directory under one tag with `--dir_group`.
- Give a release target a public name that does not depend on a path name.
- Set outputs for other GitHub Actions steps.
- Generate release notes.

## Directories

Give the `--directories` flag one time for each tag you want. Each flag makes
its own tag. The tag name is the last part of the directory path. The value is
one literal path.

```sh
semver-tags run --directories services/api --directories services/worker
```

This command can make the tags `api/v1.2.3` and `worker/v0.4.1`. A commit that
changes only `services/api` releases only `api`.

## Tag Groups

Use `--dir_group` to give more than one directory the same tag. Put the
directories in one flag and separate them with commas. The first directory in
the list gives the tag its name. Give the flag one time for each tag group.

```sh
semver-tags run \
  --dir_group "services/api,libs/shared" \
  --dir_group "services/worker,libs/shared"
```

The tags stay `api` and `worker`. The directory `libs/shared` does not get its
own tag, because no group starts with it. A commit in `libs/shared` releases
both `api` and `worker`. A commit in only `services/api` releases only `api`.

Because each group makes its own tag, you can start a different job from each
tag event in your CI system.

You can use `--directories` and `--dir_group` in the same run. To release a
shared library on its own as well, name it in a `--directories` flag too.

Every group must make a different tag name. The command stops with an error if
two groups make the same tag name.

## Named Targets

Use a named target when the public release name must not depend on a source
path. A target has one name and one or more paths. A path can name a file or a
directory. A commit in any target path affects that target.

Use `targets` in `.semver-tags.yaml` for the full configuration form:

```yaml
targets:
  - name: public-api
    paths:
      - services/api
      - libs/shared
      - README.md
  - name: public-worker
    paths:
      - services/worker
      - libs/shared
```

This configuration can make the tags `public-api/v1.2.3` and
`public-worker/v2.1.0`. A commit in `libs/shared` affects both targets.

Use the repeatable `--target` flag for the compact command-line form:

```sh
semver-tags run \
  --target "public-api=services/api,libs/shared" \
  --target "public-worker=services/worker,libs/shared"
```

The compact form uses the first `=` to separate the name from the paths. A
comma separates the paths. The compact form has no escape syntax. Use the
configuration file if a path contains a comma.

A target name must start with a letter or digit. It can contain letters,
digits, dots, underscores, and hyphens. It cannot contain `..`, and it cannot
end with a dot or `.lock`. Each target path is relative to the Git root. Use
forward slashes. A path of `.` includes the full repository.

You can use `--directories`, `--dir_group`, and named targets in one run. Each
release name must be unique. The output order is `directories`, `dir_group`,
and then `targets`.

The `directories` and `dir_group` settings remain supported. Use `directories`
for one path when its basename is the required tag name. Use `dir_group` for
multiple paths when the first path basename is the required tag name.

## Commit Types

The command reads [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
A `fix` commit increases the patch version. A `feat` commit increases the minor
version. These two bump levels are fixed.

A `!` after the type or scope increases the major version. A
`BREAKING CHANGE:` or `BREAKING-CHANGE:` footer also increases the major
version. `BREAKING CHANGE` is in the default allowed-type list.

By default, these types make a patch release: `build`, `chore`, `ci`, `docs`,
`fix`, `perf`, `refactor`, `revert`, `style`, and `test`. The `feat` type makes
a minor release. No ordinary type makes a major release by default.

Use `--patch_types`, `--minor_types`, and `--major_types` to configure other
types. Each flag is repeatable and also accepts comma-separated values. A value
replaces the default list for that level. The command always adds `fix` to the
patch list and `feat` to the minor list.

```sh
semver-tags run \
  --patch_types "fix,holiday" \
  --minor_types "feat,meatball" \
  --major_types earthquake
```

This example makes `holiday` a patch change, `meatball` a minor change, and
`earthquake` a major change.

Use `--allowed_types` to limit the configured types that can change a version.
A commit with a type outside this list does not change the version. If you do
not set `allowed_types`, all configured types and `BREAKING CHANGE` are
allowed.

```sh
semver-tags run --allowed_types fix --allowed_types "holiday,BREAKING CHANGE"
```

If you set `allowed_types`, include `BREAKING CHANGE` to permit breaking
markers.

## Configuration File

The command reads `.semver-tags.yaml` from the current directory, or the file
that `--config` names. Each key is a flag name.

```yaml
dry_run: true
patch_types:
  - fix
  - holiday
minor_types:
  - feat
  - meatball
major_types:
  - earthquake
allowed_types:
  - fix
  - feat
  - holiday
  - meatball
  - earthquake
  - BREAKING CHANGE
directories:
  - services/api
dir_group:
  - services/worker,libs/shared
targets:
  - name: public-tools
    paths:
      - tools/cli
      - libs/shared
```

A flag on the command line replaces the value in the file.

## Environment Variables

Each flag also reads an environment variable. The variable name is the flag
name in capital letters. An underscore replaces a hyphen. A flag on the
command line replaces the variable.

```sh
DRY_RUN=true BRANCH=main semver-tags run
```

For `DIRECTORIES`, `DIR_GROUP`, and `TARGETS`, a space separates the values. A
comma keeps its meaning inside one `DIR_GROUP` or `TARGETS` value. These two
commands do the same thing:

```sh
semver-tags run \
  --directories services/cron \
  --dir_group "services/api,libs/shared" \
  --dir_group "services/worker,libs/shared"
```

```sh
DIRECTORIES="services/cron" \
DIR_GROUP="services/api,libs/shared services/worker,libs/shared" \
  semver-tags run
```

Use this form for named targets:

```sh
TARGETS="public-api=services/api,libs/shared public-worker=services/worker,libs/shared" \
  semver-tags run
```

`TARGETS` uses a space between targets, the first `=` between a name and its
paths, and a comma between paths. It has no escape syntax. A path that contains
a space or comma does not work in this variable. Use the structured
configuration-file form for that path.

Command-line `--target` values replace `TARGETS` and file `targets` values.
`TARGETS` values replace file `targets` values.

A path with a space does not work in `DIRECTORIES` or `DIR_GROUP`. Use the
flags for that path.

The commit-type settings also have environment variables. A space separates
their values:

```sh
PATCH_TYPES="fix holiday" \
MINOR_TYPES="feat meatball" \
MAJOR_TYPES="earthquake" \
ALLOWED_TYPES="fix feat holiday meatball earthquake BREAKING-CHANGE" \
  semver-tags run
```

The environment form uses `BREAKING-CHANGE` as an alias because
`BREAKING CHANGE` contains a space.

## Outputs

The command writes one JSON object. Each field holds one value for each group,
separated by commas. The order is every `directories` value first, then every
`dir_group` value, and then every named target.

```sh
semver-tags run --dry_run --output_json --dir_group "services/api,libs/shared"
```

## Push Behavior

The command pushes only the tags it makes in this run. If no version changes,
the command pushes nothing.

By default the command also pushes the branch that `--branch` names. Set
`--branch ""` to push only the tags. Use this in a CI job that checked out a
commit instead of a branch.

## Short Version Tags

Use `--short-versions` to update mutable major and minor tags. A release tag of
`v1.3.7` also updates `v1.3` and `v1`. A package release of `api/v1.3.7` also
updates `api/v1.3` and `api/v1`.

```sh
semver-tags run --short-versions
```

The command force-updates short tags because they move to each new release.
It sends the full tag and the short tags in the same atomic push when
`--atomic` is true. Output fields continue to contain the full version tag.

Short tags are off by default in this major version. They will be on by
default in the next major version. When neither short-version flag is set, the
command writes a migration warning to standard output. Use
`--skip-short-versions` to keep only full tags and suppress this warning. You
can also set `LOG_LEVEL=ERROR` to suppress the warning. Set one of these values
before you parse JSON output.

```sh
semver-tags run --skip-short-versions
```

The `--skip-short-versions` flag will remain available in the next major
version. It will not remain available in later major versions. Do not use
`--short-versions` and `--skip-short-versions` together.

The configuration-file keys are `short_versions` and `skip_short_versions`.
The environment variables are `SHORT_VERSIONS` and `SKIP_SHORT_VERSIONS`.

## Continuous Integration

This repository builds and releases itself with
[Reactorcide](https://github.com/catalystcommunity/reactorcide). The
definitions are in `.reactorcide/`:

* `workflows/pr.yaml` validates the commit format, then builds, tests, lints,
  and dry runs the command for each pull request.
* `workflows/release-tag.yaml` runs after a merge to `main`. It creates a draft
  GitHub release that records the source commit, then pushes the new semver
  tag.
* `workflows/release.yaml` runs on the tag event. It first proves that CI made
  the draft release, then builds one archive for each platform, then publishes
  the release.

The jobs in `jobs/` are thin. The Python plugins in `plugins/` do the work
through the runnerlib lifecycle. There are no shell scripts.

A tag that a person pushes by hand has no draft release, so the release
workflow stops and publishes nothing.

## Development

```sh
go build ./...
go test ./...
gofmt -s -w .
golangci-lint run ./...
```

`.golangci.yml` holds the linter settings that CI uses.

## Why

We used to use Semantic-Release which is fine and dandy. We had problems with plugins and using it in a github action when the commmunity struggled with the shift to ESM imports. We are sure it will work out fine, but we can't wait, so we took the piece that was most important and separated concerns. `semver-tags` won't do anything but generate tags and give us outputs to do other things. e.g. If we want to publish a release, that's a simple thing and we can do that in a separate step based on the outputs provided.

## Status

Currently being experimented with. We're going to use this in production asap, but use at your own risk.

## LICENSE

Apache 2.0

## Contributing

Uh... hit us up somewhere before you do any work. We're happy to accept PRs if they make sense, but we don't want anyone to waste their time on a feature or approach we won't accept. Feel free to fork though, just change the command/repo name.
