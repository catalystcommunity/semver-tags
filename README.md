# semver-tags

Create semantic version Git tags from conventional commits. You can create one
tag series for the full repository or separate tag series for selected paths.
The command also creates release-note and version outputs.

## Features

- Analyze a repository or a set of paths and generate semantic version tags.
- Group more than one directory under one tag with `--dir_group`.
- Give a release target a public name that does not depend on a path name.
- Set outputs for other GitHub Actions steps.
- Create release notes from commit subjects.

## Usage

Run `semver-tags run` in a Git work tree. The command creates and pushes tags
unless you use `--dry_run`.

```sh
semver-tags run [flags]
semver-tags run --help
```

By default, the command uses the `origin` remote and pushes the `main` branch
with the tags. It prints a JSON object by default. Use `--output_json=false`
if you do not want this output.

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
types. Each flag is repeatable. Each flag also accepts comma-separated values.
A configured value replaces the default list for that level. The command
always adds `fix` to the patch list and `feat` to the minor list.

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

## Version Identifiers

Use `--pre_release_string` to add a pre-release identifier. The first tag for
an identifier has the `.1` suffix. If the last version has the same
identifier, the command increases this suffix.

Use `--build_string` to add build information to a new version. For example,
the value `build7` adds `+build7` to the tag.

## Configuration File

The command reads `.semver-tags.yaml` from the current directory. Use
`--config` to select a different file. Most keys use the flag name. The short
version keys use underscores, and named targets use the `targets` key.

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

A command-line value replaces an environment or file value.

## Environment Variables

Each setting also reads an environment variable. The variable name is the
setting name in capital letters. Use underscores in the variable name. A
command-line value replaces an environment value.

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

The command writes one JSON object when `output_json` is true. All values in
this object are strings. Most fields contain one comma-separated value for
each release target. The order is `directories`, then `dir_group`, and then
`targets`.

```sh
semver-tags run --dry_run --output_json --dir_group "services/api,libs/shared"
```

The JSON object has these fields:

| Field | Content |
| --- | --- |
| `New_release_published` | `true` when the calculated version changed. In a dry run, no tag is published. |
| `New_release_version` | The new version without the `v` prefix or package name. |
| `New_release_major_version` | The major part of the new version. |
| `New_release_minor_version` | The minor part of the new version. |
| `New_release_patch_version` | The patch part of the new version. |
| `New_release_git_head` | The commit for the new tag. |
| `New_release_notes` | Commit subjects. A comma and newline separate release targets. |
| `New_release_notes_json` | A JSON object that contains an array of commit subjects for each package. |
| `Dry_run` | The dry-run state. |
| `Release_package` | The package or target name. This value is empty for a full-repository release. |
| `New_release_git_tag` | The calculated full tag. |
| `Last_release_version` | The previous version without the `v` prefix or package name. |
| `Last_release_git_head` | The commit for the previous tag. |
| `Last_release_git_tag` | The previous full tag. |

Use `--github_action` to write the same values as GitHub Actions outputs. The
output names use lowercase letters. For example, the command writes
`new_release_git_tag` and `new_release_notes_json`.

The release notes include all commit subjects in the selected Git history.
The commit type controls the version change, but it does not filter the
release notes. Use `New_release_notes_json` when you must reliably separate
the notes for multiple targets.

## Push Behavior

The command pushes only the tags it makes in this run. If no version changes,
the command pushes nothing.

By default, the command also pushes the branch that `--branch` names. Set
`--branch ""` to push only the tags. Use this in a CI job that checked out a
commit instead of a branch.

The command uses an atomic push by default. Thus, the remote accepts all new
tags and the selected branch, or it accepts none of them. Use `--atomic=false`
when the remote does not support atomic pushes.

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

This project replaces the tag calculation that we previously did with
Semantic Release. Plugin and ESM changes made that workflow difficult to use
in GitHub Actions. This command has a smaller scope. It creates tags and
outputs. A separate CI step can use those outputs to publish a release.

## Status

This project is experimental. Evaluate it before you use it in production.

## LICENSE

Apache 2.0

## Contributing

Contact the maintainers before you start a contribution. This step helps you
confirm that the proposed change is suitable. You can also fork the project.
If you publish the fork, use a different command and repository name.
