package core

import (
	"testing"

	"github.com/catalystcommunity/semver-tags/core/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeCommitMessageBumpLevels(t *testing.T) {
	allowed := DefaultAllowedTypes()

	cases := map[string]semver.CommitType{
		"feat: add a thing":            semver.Minor,
		"feat(api): add a thing":       semver.Minor,
		"fix: repair a thing":          semver.Patch,
		"chore: tidy up":               semver.Patch,
		"docs: write it down":          semver.Patch,
		"style: reformat":              semver.Patch,
		"refactor: move it":            semver.Patch,
		"test: cover it":               semver.Patch,
		"revert: undo it":              semver.Patch,
		"build: change the build":      semver.Patch,
		"ci: change the pipeline":      semver.Patch,
		"perf: make it fast":           semver.Patch,
		"feat!: break it":              semver.Major,
		"feat(api)!: break it":         semver.Major,
		"fix!: break it":               semver.Major,
		"BREAKING CHANGE: break it":    semver.Major,
		"just a message":               semver.NotConventional,
		"unknown: not a known type":    semver.NotConventional,
		"Merge pull request #1 from x": semver.NotConventional,
	}

	for message, expected := range cases {
		assert.Equal(t, expected, AnalyzeCommitMessage(message, allowed), message)
	}
}

func TestAnalyzeCommitMessageHonorsAllowedTypes(t *testing.T) {
	allowed := []string{"fix"}

	assert.Equal(t, semver.Patch, AnalyzeCommitMessage("fix: repair a thing", allowed))
	assert.Equal(t, semver.NotConventional, AnalyzeCommitMessage("feat: add a thing", allowed))
	assert.Equal(t, semver.NotConventional, AnalyzeCommitMessage("ci: change it", allowed))
}

// A breaking change is major even when its type is not allowed, because
// dropping it would hide an incompatible change.
func TestAnalyzeCommitMessageAlwaysTakesBreakingChange(t *testing.T) {
	assert.Equal(
		t,
		semver.Major,
		AnalyzeCommitMessage("BREAKING CHANGE: break it", []string{"fix"}),
	)
}

// An empty list means the default list, so a wrong setting can not silently
// stop every release.
func TestAnalyzeCommitMessageWithNoAllowedTypesUsesTheDefault(t *testing.T) {
	assert.Equal(t, semver.Minor, AnalyzeCommitMessage("feat: add a thing", nil))
}

func TestDefaultAllowedTypesIsSortedAndComplete(t *testing.T) {
	types := DefaultAllowedTypes()

	assert.Equal(t, []string{
		"build", "chore", "ci", "docs", "feat",
		"fix", "perf", "refactor", "revert", "style", "test",
	}, types)
}

func TestParseVersionInfo(t *testing.T) {
	info, err := ParseVersionInfo("api/v1.2.3,abc123")
	require.NoError(t, err)
	assert.Equal(t, "api", info.Package)
	assert.Equal(t, "v1.2.3", info.Version.FormattedString())
	assert.Equal(t, "abc123", info.CommitHash)
}

func TestParseVersionInfoWithoutPackage(t *testing.T) {
	info, err := ParseVersionInfo("v1.2.3,abc123")
	require.NoError(t, err)
	assert.Equal(t, "", info.Package)
	assert.Equal(t, "v1.2.3", info.Version.FormattedString())
}

func TestParseVersionInfoWithPreReleaseAndBuild(t *testing.T) {
	info, err := ParseVersionInfo("api/v1.2.3-rc.1+build7,abc123")
	require.NoError(t, err)
	assert.Equal(t, "rc.1", info.Version.PreRelease)
	assert.Equal(t, "build7", info.Version.Build)
}

func TestParseVersionInfoRejectsANonVersionTag(t *testing.T) {
	_, err := ParseVersionInfo("nightly,abc123")
	assert.Error(t, err)
}
