package core

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	gha "github.com/sethvargo/go-githubactions"
)

// Outputs holds one value for each directory group, separated by commas, in
// the order of the groups. The JSON names are the contract that other tools
// read, so the tags keep them even though the Go names follow Go style.
type Outputs struct {
	NewReleasePublished    string `json:"New_release_published"`
	NewReleaseVersion      string `json:"New_release_version"`
	NewReleaseMajorVersion string `json:"New_release_major_version"`
	NewReleaseMinorVersion string `json:"New_release_minor_version"`
	NewReleasePatchVersion string `json:"New_release_patch_version"`
	NewReleaseGitHead      string `json:"New_release_git_head"`
	NewReleaseNotes        string `json:"New_release_notes"`
	NewReleaseNotesJson    string `json:"New_release_notes_json"`
	DryRun                 string `json:"Dry_run"`
	ReleasePackage         string `json:"Release_package"`
	NewReleaseGitTag       string `json:"New_release_git_tag"`
	LastReleaseVersion     string `json:"Last_release_version"`
	LastReleaseGitHead     string `json:"Last_release_git_head"`
	LastReleaseGitTag      string `json:"Last_release_git_tag"`
}

// joinValues makes the separated output of one field. It removes the
// separators at the end, so a group with an empty value does not leave one.
func joinValues(values []string, separator string) string {
	joined := ""
	for _, value := range values {
		joined += value + separator
	}
	return strings.TrimRight(joined, separator)
}

// tagFor makes the tag text of one version. The package name is a prefix, and
// the whole repository has no package name.
func tagFor(version *VersionInfo) string {
	if version.Package == "" {
		return version.Version.FormattedString()
	}
	return version.Package + "/" + version.Version.FormattedString()
}

// shortTagsFor gives the mutable major and minor tags for one full release
// tag. These tags point to the same commit as the full release tag.
func shortTagsFor(version *VersionInfo) []string {
	prefix := ""
	if version.Package != "" {
		prefix = version.Package + "/"
	}
	return []string{
		fmt.Sprintf("%sv%d.%d", prefix, version.Version.Major, version.Version.Minor),
		fmt.Sprintf("%sv%d", prefix, version.Version.Major),
	}
}

// releaseNotesJson makes a JSON object with the notes of each package. It
// keeps the order of the groups, which a JSON object of a Go map would not.
func releaseNotesJson(results []DirectoryVersionInfo) (string, error) {
	var builder strings.Builder
	builder.WriteString(`{"new_release_notes_escaped":{`)

	for index, result := range results {
		if index > 0 {
			builder.WriteString(",")
		}

		key, err := json.Marshal("package_" + result.NextVersion.Package)
		if err != nil {
			return "", fmt.Errorf("can not write the package name of a release note: %w", err)
		}
		notes := result.ReleaseNotes
		if notes == nil {
			notes = []string{}
		}
		value, err := json.Marshal(notes)
		if err != nil {
			return "", fmt.Errorf("can not write the release notes: %w", err)
		}

		builder.Write(key)
		builder.WriteString(":")
		builder.Write(value)
	}

	builder.WriteString("}}")
	return builder.String(), nil
}

// GenerateOutputs makes the outputs for every directory group of one run.
func GenerateOutputs(results []DirectoryVersionInfo, dryRun bool) (Outputs, error) {
	count := len(results)
	published := make([]string, 0, count)
	packages := make([]string, 0, count)
	versions := make([]string, 0, count)
	majorVersions := make([]string, 0, count)
	minorVersions := make([]string, 0, count)
	patchVersions := make([]string, 0, count)
	newHeads := make([]string, 0, count)
	notes := make([]string, 0, count)
	dryRuns := make([]string, 0, count)
	newTags := make([]string, 0, count)
	lastVersions := make([]string, 0, count)
	lastHeads := make([]string, 0, count)
	lastTags := make([]string, 0, count)

	for _, result := range results {
		next := result.NextVersion
		last := result.LastVersion

		changed := next.Version.FormattedString() != last.Version.FormattedString()
		published = append(published, strconv.FormatBool(changed))

		packages = append(packages, next.Package)
		versions = append(versions, fmt.Sprintf(
			"%d.%d.%d", next.Version.Major, next.Version.Minor, next.Version.Patch,
		))
		majorVersions = append(majorVersions, fmt.Sprintf("%d", next.Version.Major))
		minorVersions = append(minorVersions, fmt.Sprintf("%d", next.Version.Minor))
		patchVersions = append(patchVersions, fmt.Sprintf("%d", next.Version.Patch))
		newHeads = append(newHeads, next.CommitHash)
		notes = append(notes, strings.Join(result.ReleaseNotes, "\n"))
		dryRuns = append(dryRuns, strconv.FormatBool(dryRun))
		newTags = append(newTags, tagFor(next))
		lastVersions = append(lastVersions, fmt.Sprintf(
			"%d.%d.%d", last.Version.Major, last.Version.Minor, last.Version.Patch,
		))
		lastHeads = append(lastHeads, last.CommitHash)
		lastTags = append(lastTags, tagFor(last))
	}

	notesJson, err := releaseNotesJson(results)
	if err != nil {
		return Outputs{}, err
	}

	return Outputs{
		NewReleasePublished:    joinValues(published, ","),
		NewReleaseVersion:      joinValues(versions, ","),
		NewReleaseMajorVersion: joinValues(majorVersions, ","),
		NewReleaseMinorVersion: joinValues(minorVersions, ","),
		NewReleasePatchVersion: joinValues(patchVersions, ","),
		NewReleaseGitHead:      joinValues(newHeads, ","),
		NewReleaseNotes:        joinValues(notes, ",\n"),
		NewReleaseNotesJson:    notesJson,
		DryRun:                 joinValues(dryRuns, ","),
		ReleasePackage:         joinValues(packages, ","),
		NewReleaseGitTag:       joinValues(newTags, ","),
		LastReleaseVersion:     joinValues(lastVersions, ","),
		LastReleaseGitHead:     joinValues(lastHeads, ","),
		LastReleaseGitTag:      joinValues(lastTags, ","),
	}, nil
}

// SetGithubActionOutputs writes the outputs for later github action steps.
func SetGithubActionOutputs(results Outputs) {
	gha.SetOutput("new_release_published", results.NewReleasePublished)
	gha.SetOutput("new_release_version", results.NewReleaseVersion)
	gha.SetOutput("new_release_major_version", results.NewReleaseMajorVersion)
	gha.SetOutput("new_release_minor_version", results.NewReleaseMinorVersion)
	gha.SetOutput("new_release_patch_version", results.NewReleasePatchVersion)
	gha.SetOutput("new_release_git_head", results.NewReleaseGitHead)
	gha.SetOutput("new_release_notes", results.NewReleaseNotes)
	gha.SetOutput("new_release_notes_json", results.NewReleaseNotesJson)
	gha.SetOutput("dry_run", results.DryRun)
	gha.SetOutput("release_package", results.ReleasePackage)
	gha.SetOutput("new_release_git_tag", results.NewReleaseGitTag)
	gha.SetOutput("last_release_version", results.LastReleaseVersion)
	gha.SetOutput("last_release_git_head", results.LastReleaseGitHead)
	gha.SetOutput("last_release_git_tag", results.LastReleaseGitTag)
}
