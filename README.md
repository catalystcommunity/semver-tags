# semver-tags

Do an analysis of a repo or its subdirs and generate git tags for semantic versioning based on conventional commits. Oh, and release notes generated.

## Features
* Analyze a repo, or a set of directories in a repo and generate semantic version tags.
* In github action mode, set outputs for use in other github action steps
* Generate releaase notes

## Why

We used to use Semantic-Release which is fine and dandy. We had problems with plugins and using it in a github action when the commmunity struggled with the shift to ESM imports. We are sure it will work out fine, but we can't wait, so we took the piece that was most important and separated concerns. `semver-tags` won't do anything but generate tags and give us outputs to do other things. e.g. If we want to publish a release, that's a simple thing and we can do that in a separate step based on the outputs provided.

## Status

Currently being experimented with. We're going to use this in production asap, but use at your own risk.

## LICENSE

Apache 2.0

## Contributing

Uh... hit us up somewhere before you do any work. We're happy to accept PRs if they make sense, but we don't want anyone to waste their time on a feature or approach we won't accept. Feel free to fork though, just change the command/repo name.