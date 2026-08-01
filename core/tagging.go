package core

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/catalystcommunity/app-utils-go/logging"
)

// Config holds every setting of one tagging run.
type Config struct {
	DryRun           bool
	GithubAction     bool
	OutputJson       bool
	Atomic           bool
	PreReleaseString string
	BuildString      string
	Remote           string
	Branch           string
	AllowedTypes     []string
	Directories      []string
	DirGroups        []string
	Targets          []TargetConfig
}

// DoTagging works out the next version of each directory group, makes the
// tags, and writes the outputs.
func DoTagging(config Config) error {
	// Make sure we're in a git repo with a git command or this is pointless
	if !IsGitRepo() {
		return errors.New("current directory is not a git repo, nothing to do")
	}

	// We deal in full paths for consistency, so we need to know what to prepend to subdirectories
	gitRoot, err := GetGitRootDir()
	if err != nil {
		return err
	}

	results, err := ParseReleaseTargets(config.Directories, config.DirGroups, config.Targets, gitRoot)
	if err != nil {
		return err
	}
	// With no directory given, the whole repo is one unnamed package
	if len(results) == 0 {
		results = append(results, DirectoryVersionInfo{FullPath: gitRoot})
	}

	head, err := headCommit()
	if err != nil {
		return err
	}

	run := &tagger{config: config, head: head}
	for idx := range results {
		// Get the latest tag and hash that applies for this directory
		results[idx].LastVersion, err = run.latestVersion(results[idx])
		if err != nil {
			return err
		}
		// Now analyze the commit history for that directory and only that
		// directory, and calculate the next version
		if err := run.analyzeCommits(&results[idx]); err != nil {
			return err
		}
	}

	// Process what tags we should be making
	var newTags []string
	for _, result := range results {
		if result.NextVersion == nil ||
			result.LastVersion.Version.FormattedString() == result.NextVersion.Version.FormattedString() {
			// This hasn't changed, so we don't need to do anything
			logging.Log.Info(fmt.Sprintf("No new version for: %s", result.Printable()))
			continue
		}

		// We have a nextVersion, so build the tag with the optional package name
		tag := tagFor(result.NextVersion)

		// If not in dry-run, tag stuff for each thing that changed
		if config.DryRun {
			logging.Log.Info(fmt.Sprintf("We would be tagging a new version: %s", tag))
		} else {
			logging.Log.Info(fmt.Sprintf("Tagging new version: %s", tag))
			if err := createTag(tag); err != nil {
				return err
			}
		}
		newTags = append(newTags, tag)
	}

	outputs, err := GenerateOutputs(results, config.DryRun)
	if err != nil {
		return err
	}

	// We only push tags this run made, and a dry run makes none.
	// All tags should be there, so push! This prevents tags being pushed if
	// there were errors. Ex. cmd: git push --atomic origin main tagOne tagTwo
	if !config.DryRun && len(newTags) > 0 {
		if err := pushTags(config.Remote, config.Branch, config.Atomic, newTags); err != nil {
			return err
		}
	}

	// If in githubactions, output each output, comma separated for each directory
	if config.GithubAction {
		SetGithubActionOutputs(outputs)
	}

	if config.OutputJson {
		outputJson, err := json.Marshal(outputs)
		if err != nil {
			return err
		}
		fmt.Print(string(outputJson))
	}

	return nil
}
