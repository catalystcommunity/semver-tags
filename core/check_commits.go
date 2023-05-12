package core

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/catalystsquad/app-utils-go/logging"
	"github.com/catalystsquad/semver-tags/core/semver"

	gha "github.com/sethvargo/go-githubactions"
)

type VersionInfo struct {
	Version    semver.Semver
	CommitHash string
}

type CommitType int

const (
	NotConventional CommitType = iota
	Patch
	Minor
	Major
)

func AnalyzeCommitMessage(message string) CommitType {
	if !strings.Contains(message, ":") {
		return NotConventional
	}

	split := strings.SplitN(message, ":", 2)
	if len(split) < 2 {
		return NotConventional
	}

	typeAndScope := split[0]
	typeSplit := strings.SplitN(typeAndScope, "(", 2)

	commitType := typeSplit[0]

	if strings.HasSuffix(commitType, "!") || strings.HasSuffix(typeAndScope, "!") {
		return Major
	}

	switch commitType {
	case "fix", "chore", "docs", "style", "refactor", "test", "revert":
		return Patch
	case "feat":
		return Minor
	case "BREAKING CHANGE":
		return Major
	default:
		return NotConventional
	}
}

func ParseVersionInfo(line string) (*VersionInfo, error) {
	fmt.Println("foo: " + line)
	line = strings.TrimPrefix(line, "v")
	fmt.Println(line)
	split := strings.Split(line, ",")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid format")
	}

	versionComponents := strings.SplitN(split[0], "-", 2)
	version := versionComponents[0]

	var preRelease, build string
	if len(versionComponents) > 1 {
		preRelease = versionComponents[1]
	}

	buildComponents := strings.SplitN(preRelease, "+", 2)
	if len(buildComponents) > 1 {
		preRelease = buildComponents[0]
		build = buildComponents[1]
	}

	var major, minor, patch uint32
	n, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch)
	if err != nil || n != 3 {
		return nil, fmt.Errorf("error parsing version: count: %d err: %w", n, err)
	}

	info := &VersionInfo{
		Version: semver.Semver{
			Major:      major,
			Minor:      minor,
			Patch:      patch,
			PreRelease: preRelease,
			Build:      build,
		},
		CommitHash: split[1],
	}

	return info, nil
}

func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()

	if err != nil {
		return false
	}

	result := strings.Trim(string(output), "\n")

	return result == "true"
}

func SetGithubActionOutputs(latest *VersionInfo, next *VersionInfo, release_notes string, dry_run bool) {
	if next.Version.FormattedString() == latest.Version.FormattedString() {
		gha.SetOutput("new_release_published", "false")
	} else {
		gha.SetOutput("new_release_published", "true")
	}

	gha.SetOutput("new_release_version", fmt.Sprintf("%d.%d.%d", next.Version.Major, next.Version.Minor, next.Version.Patch))
	gha.SetOutput("new_release_major_version", fmt.Sprintf("%d", next.Version.Major))
	gha.SetOutput("new_release_minor_version", fmt.Sprintf("%d", next.Version.Minor))
	gha.SetOutput("new_release_patch_version", fmt.Sprintf("%d", next.Version.Patch))
	gha.SetOutput("new_release_git_head", next.CommitHash)
	gha.SetOutput("new_release_notes", release_notes)
	gha.SetOutput("dry_run", strconv.FormatBool(dry_run))
	gha.SetOutput("new_release_git_tag", next.Version.FormattedString())
	gha.SetOutput("last_release_version", fmt.Sprintf("%d.%d.%d", latest.Version.Major, latest.Version.Minor, latest.Version.Patch))
	gha.SetOutput("last_release_git_head", latest.CommitHash)
	gha.SetOutput("last_release_git_tag", latest.Version.FormattedString())
}

func Check_commits(DryRun bool, GithubAction bool, OutputJson bool, PreReleaseString string, BuildString string) error {
	// Make sure we're in a git repo with a git command or this is pointless
	if !IsGitRepo() {
		return errors.New("current directory is not a git repo, nothing to do")
	}

	// Get all the tags.
	cmd := exec.Command("git", "for-each-ref", "--count=5", "--sort=-committerdate", "--format", "%(refname:short),%(objectname)", "refs/tags")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("can not get git tags: %s\n%s", err, string(output))
	}

	tags := []*VersionInfo{}
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		logging.Log.Info(fmt.Sprintf("Tag line found: %s", line))
		if len(line) == 0 {
			continue
		}
		v, err := ParseVersionInfo(line)
		if err != nil {
			return fmt.Errorf("can not parse tag line: %s-h%s", line, err)
		}
		tags = append(tags, v)
	}

	// Now analyze the commits to see if we are generating anything.
	latest := tags[0]
	nextVersion := tags[0].Version.Clone()
	cmd = exec.Command("git", "log", "--pretty=format:%s", fmt.Sprintf("%s..HEAD", latest.CommitHash))
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("can not get git commits: %s\n%s", err, string(output))
	}

	highest := NotConventional
	release_notes_items := []string{}
	release_notes := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		logging.Log.Info(fmt.Sprintf("Analyzing Commit: %s", line))
		if len(line) == 0 {
			continue
		}
		commitType := AnalyzeCommitMessage(line)
		if commitType > highest {
			highest = commitType
		}
		switch commitType {
		case NotConventional:
			logging.Log.Info("Not a conventional commit")
		case Patch:
			logging.Log.Info("Found Patch commit")
		case Minor:
			logging.Log.Info("Found Minor commit")
		case Major:
			logging.Log.Info("Found Major commit")
		default:
			return fmt.Errorf("could not parse commit message: %s", string(line))
		}
		release_notes_items = append(release_notes_items, fmt.Sprintf(" * %s", line))
	}
	for _, line := range release_notes_items {
		release_notes += line + "\n"
	}

	switch highest {
	case NotConventional:
		logging.Log.Info("No version to generate.")
	case Patch:
		nextVersion.BumpPatch()
	case Minor:
		nextVersion.BumpMinor()
	case Major:
		nextVersion.BumpMajor()
	default:
		return fmt.Errorf("somehow highest is not a CommitType, which shouldn't be possible: %d", highest)
	}
	// If they want a pre-release, we just need to check if the latest was already the same pre-release
	if PreReleaseString != "" {
		// If it already has the same pre-release, we increment that, rather than adding onto nextVersion
		if strings.HasPrefix(latest.Version.PreRelease, PreReleaseString) {
			nextVersion = latest.Version.Clone()
		}
		nextVersion.IncrementPreRelease()
		if BuildString != "" {
			nextVersion.Build = BuildString
		}
	}

	// Get the latest commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("can not get latest commit hash: %s\n%s", err, string(output))
	}
	next := &VersionInfo{
		*nextVersion,
		strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")[0],
	}

	// Now if we have a new version, do the things. Otherwise, exit
	if next.Version.FormattedString() == latest.Version.FormattedString() {
		gha.SetOutput("new_release_published", "false")
		logging.Log.Info("No release to be made, exiting.")
		return nil
	}

	logging.Log.Info("New release version:", nextVersion.FormattedString())
	if GithubAction {
		SetGithubActionOutputs(latest, next, release_notes, DryRun)
	}

	if !DryRun {
		cmd = exec.Command("git", "tag", nextVersion.FormattedString())
		output, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("error tagging: %s\n%s", err, string(output))
		}
		cmd = exec.Command("git", "push", "origin", nextVersion.FormattedString())
		output, err = cmd.Output()
		if err != nil {
			return fmt.Errorf("error pushing tags: %s\n%s", err, string(output))
		}
	}

	return nil
}
