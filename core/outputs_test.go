package core

import (
	"encoding/json"
	"testing"

	"github.com/catalystcommunity/semver-tags/core/semver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func group(
	packageName string,
	lastVersion string,
	nextVersion string,
	notes []string,
) DirectoryVersionInfo {
	parseVersion := func(text string) *semver.Semver {
		info, err := ParseVersionInfo(text + ",hash")
		if err != nil {
			panic(err)
		}
		return info.Version
	}

	return DirectoryVersionInfo{
		Directory:    packageName,
		LastVersion:  &VersionInfo{Package: packageName, Version: parseVersion(lastVersion), CommitHash: "lasthash"},
		NextVersion:  &VersionInfo{Package: packageName, Version: parseVersion(nextVersion), CommitHash: "nexthash"},
		ReleaseNotes: notes,
	}
}

func TestGenerateOutputsForOneGroup(t *testing.T) {
	results := []DirectoryVersionInfo{
		group("api", "v1.0.0", "v1.1.0", []string{"feat: a thing"}),
	}

	outputs, err := GenerateOutputs(results, true)
	require.NoError(t, err)

	assert.Equal(t, "true", outputs.NewReleasePublished)
	assert.Equal(t, "1.1.0", outputs.NewReleaseVersion)
	assert.Equal(t, "1", outputs.NewReleaseMajorVersion)
	assert.Equal(t, "1", outputs.NewReleaseMinorVersion)
	assert.Equal(t, "0", outputs.NewReleasePatchVersion)
	assert.Equal(t, "nexthash", outputs.NewReleaseGitHead)
	assert.Equal(t, "lasthash", outputs.LastReleaseGitHead)
	assert.Equal(t, "api/v1.1.0", outputs.NewReleaseGitTag)
	assert.Equal(t, "api/v1.0.0", outputs.LastReleaseGitTag)
	assert.Equal(t, "api", outputs.ReleasePackage)
	assert.Equal(t, "true", outputs.DryRun)
	assert.Equal(t, "feat: a thing", outputs.NewReleaseNotes)
}

func TestGenerateOutputsJoinsGroupsInOrder(t *testing.T) {
	results := []DirectoryVersionInfo{
		group("api", "v1.0.0", "v1.1.0", []string{"feat: a thing"}),
		group("worker", "v2.0.0", "v2.0.0", nil),
	}

	outputs, err := GenerateOutputs(results, false)
	require.NoError(t, err)

	assert.Equal(t, "true,false", outputs.NewReleasePublished)
	assert.Equal(t, "api/v1.1.0,worker/v2.0.0", outputs.NewReleaseGitTag)
	assert.Equal(t, "api,worker", outputs.ReleasePackage)
	assert.Equal(t, "false,false", outputs.DryRun)
}

// The whole repository has no package name, so its tag has no prefix.
func TestGenerateOutputsForTheWholeRepository(t *testing.T) {
	results := []DirectoryVersionInfo{
		group("", "v1.0.0", "v1.0.1", []string{"fix: a thing"}),
	}

	outputs, err := GenerateOutputs(results, false)
	require.NoError(t, err)

	assert.Equal(t, "v1.0.1", outputs.NewReleaseGitTag)
	assert.Equal(t, "v1.0.0", outputs.LastReleaseGitTag)
	assert.Equal(t, "", outputs.ReleasePackage)
}

func TestReleaseNotesJsonIsValidAndKeepsGroupOrder(t *testing.T) {
	results := []DirectoryVersionInfo{
		group("worker", "v2.0.0", "v2.0.1", []string{`fix: handle a "quote"`}),
		group("api", "v1.0.0", "v1.1.0", []string{"feat: one", "fix: two"}),
	}

	outputs, err := GenerateOutputs(results, false)
	require.NoError(t, err)

	// The text must parse, which proves the quotes are escaped
	var parsed struct {
		Escaped map[string][]string `json:"new_release_notes_escaped"`
	}
	require.NoError(t, json.Unmarshal([]byte(outputs.NewReleaseNotesJson), &parsed))
	assert.Equal(t, []string{`fix: handle a "quote"`}, parsed.Escaped["package_worker"])
	assert.Equal(t, []string{"feat: one", "fix: two"}, parsed.Escaped["package_api"])

	// The worker group comes first, because the groups keep their order
	assert.Equal(
		t,
		`{"new_release_notes_escaped":{"package_worker":["fix: handle a \"quote\""],`+
			`"package_api":["feat: one","fix: two"]}}`,
		outputs.NewReleaseNotesJson,
	)
}

func TestReleaseNotesJsonWritesAnEmptyListForNoNotes(t *testing.T) {
	results := []DirectoryVersionInfo{group("api", "v1.0.0", "v1.0.0", nil)}

	outputs, err := GenerateOutputs(results, false)
	require.NoError(t, err)

	assert.Equal(
		t,
		`{"new_release_notes_escaped":{"package_api":[]}}`,
		outputs.NewReleaseNotesJson,
	)
}

// The JSON names are the contract that other tools read, so they must not
// change when the Go field names do.
func TestOutputsKeepTheirJsonNames(t *testing.T) {
	encoded, err := json.Marshal(Outputs{NewReleaseGitTag: "v1.0.0"})
	require.NoError(t, err)

	var decoded map[string]string
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	for _, name := range []string{
		"New_release_published", "New_release_version", "New_release_major_version",
		"New_release_minor_version", "New_release_patch_version", "New_release_git_head",
		"New_release_notes", "New_release_notes_json", "Dry_run", "Release_package",
		"New_release_git_tag", "Last_release_version", "Last_release_git_head",
		"Last_release_git_tag",
	} {
		assert.Contains(t, decoded, name)
	}
	assert.Len(t, decoded, 14)
	assert.Equal(t, "v1.0.0", decoded["New_release_git_tag"])
}
