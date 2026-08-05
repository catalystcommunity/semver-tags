package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/sirupsen/logrus"
)

const shortVersionWarning = "WARNING: Short version tags will become the default in the next major version. " +
	"Use --short-versions to enable them now or --skip-short-versions to keep the current behavior " +
	"without this warning. --skip-short-versions will be removed after the next major version."

// Config holds every setting of one tagging run.
type Config struct {
	DryRun            bool
	GithubAction      bool
	OutputJson        bool
	Atomic            bool
	PreReleaseString  string
	BuildString       string
	Remote            string
	Branch            string
	AllowedTypes      []string
	PatchTypes        []string
	MinorTypes        []string
	MajorTypes        []string
	ShortVersions     bool
	SkipShortVersions bool
	Directories       []string
	DirGroups         []string
	Targets           []TargetConfig
}

// DoTagging works out the next version of each directory group, makes the
// tags, and writes the outputs.
func DoTagging(config Config) error {
	if config.ShortVersions && config.SkipShortVersions {
		return errors.New("short_versions and skip_short_versions cannot both be true")
	}
	if !config.ShortVersions && !config.SkipShortVersions &&
		logging.Log.IsLevelEnabled(logrus.WarnLevel) {
		if _, err := fmt.Fprintln(os.Stdout, shortVersionWarning); err != nil {
			return fmt.Errorf("can not write the short-version migration warning: %w", err)
		}
	}
	rules, err := newBumpRules(config)
	if err != nil {
		return err
	}

	if !IsGitRepo() {
		return errors.New("current directory is not a git repo, nothing to do")
	}

	gitRoot, err := GetGitRootDir()
	if err != nil {
		return err
	}

	results, err := ParseReleaseTargets(config.Directories, config.DirGroups, config.Targets, gitRoot)
	if err != nil {
		return err
	}
	// An empty target list selects the full repository.
	if len(results) == 0 {
		results = append(results, DirectoryVersionInfo{FullPath: gitRoot})
	}

	head, err := headCommit()
	if err != nil {
		return err
	}

	run := &tagger{config: config, rules: rules, head: head}
	for idx := range results {
		results[idx].LastVersion, err = run.latestVersion(results[idx])
		if err != nil {
			return err
		}
		if err := run.analyzeCommits(&results[idx]); err != nil {
			return err
		}
	}

	var newTags []string
	var shortTags []string
	for _, result := range results {
		if result.NextVersion == nil ||
			result.LastVersion.Version.FormattedString() == result.NextVersion.Version.FormattedString() {
			logging.Log.Info(fmt.Sprintf("No new version for: %s", result.Printable()))
			continue
		}

		tag := tagFor(result.NextVersion)

		if config.DryRun {
			logging.Log.Info(fmt.Sprintf("We would be tagging a new version: %s", tag))
		} else {
			logging.Log.Info(fmt.Sprintf("Tagging new version: %s", tag))
			if err := createTag(tag); err != nil {
				return err
			}
		}
		newTags = append(newTags, tag)

		if config.ShortVersions {
			for _, shortTag := range shortTagsFor(result.NextVersion) {
				if config.DryRun {
					logging.Log.Info(fmt.Sprintf("We would be updating a short version tag: %s", shortTag))
				} else {
					logging.Log.Info(fmt.Sprintf("Updating short version tag: %s", shortTag))
					if err := updateTag(shortTag); err != nil {
						return err
					}
				}
				shortTags = append(shortTags, shortTag)
			}
		}
	}

	outputs, err := GenerateOutputs(results, config.DryRun)
	if err != nil {
		return err
	}

	// Push after all local tags exist so that an earlier error cannot publish a
	// partial result.
	if !config.DryRun && len(newTags) > 0 {
		if err := pushTags(config.Remote, config.Branch, config.Atomic, newTags, shortTags); err != nil {
			return err
		}
	}

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
