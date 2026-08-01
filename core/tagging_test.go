package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// TaggingSuite builds a small repository for each test. The repository holds
// two services and one library that both services share.
type TaggingSuite struct {
	suite.Suite
	repoDir     string
	previousDir string
	commitCount int
	lastOutput  string
}

func TestTaggingSuite(t *testing.T) {
	suite.Run(t, new(TaggingSuite))
}

func (s *TaggingSuite) SetupTest() {
	previousDir, err := os.Getwd()
	require.NoError(s.T(), err)
	s.previousDir = previousDir
	s.commitCount = 0
	s.lastOutput = ""

	// macOS puts the temporary directory behind a symlink, and git reports the
	// resolved path, so resolve it here to keep the two paths comparable.
	repoDir, err := filepath.EvalSymlinks(s.T().TempDir())
	require.NoError(s.T(), err)
	s.repoDir = repoDir

	require.NoError(s.T(), os.Chdir(s.repoDir))
	s.git("init", "-q", "-b", "main", ".")
	s.git("config", "user.email", "test@example.com")
	s.git("config", "user.name", "Test")

	for _, dir := range []string{"services/api", "services/worker", "libs/shared"} {
		require.NoError(s.T(), os.MkdirAll(filepath.Join(s.repoDir, dir), 0o755))
		s.write(filepath.Join(dir, "file.txt"), "start")
	}
	s.commit("feat: initial layout")
	s.git("tag", "api/v1.0.0")
	s.git("tag", "worker/v2.0.0")
}

func (s *TaggingSuite) TearDownTest() {
	require.NoError(s.T(), os.Chdir(s.previousDir))
}

func (s *TaggingSuite) git(args ...string) {
	command := exec.Command("git", args...)
	command.Dir = s.repoDir
	output, err := command.CombinedOutput()
	require.NoError(s.T(), err, "git %v failed: %s", args, string(output))
}

func (s *TaggingSuite) write(relativePath string, content string) {
	fullPath := filepath.Join(s.repoDir, relativePath)
	require.NoError(s.T(), os.WriteFile(fullPath, []byte(content+"\n"), 0o644))
}

// commit gives every commit a later date, so a test that depends on commit
// order does not depend on how fast the test runs.
func (s *TaggingSuite) commit(message string) {
	s.commitCount++
	date := fmt.Sprintf("2026-01-01T00:%02d:00", s.commitCount)
	command := exec.Command("git", "commit", "-q", "-m", message)
	command.Dir = s.repoDir
	command.Env = append(
		os.Environ(),
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)
	s.git("add", "-A")
	output, err := command.CombinedOutput()
	require.NoError(s.T(), err, "git commit failed: %s", string(output))
}

func (s *TaggingSuite) headCommit() string {
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = s.repoDir
	output, err := command.Output()
	require.NoError(s.T(), err)
	return string(output[:40])
}

// tagDryRun runs the tagging logic without writing tags and returns the
// outputs the command prints as JSON.
func (s *TaggingSuite) tagDryRun(directories []string, dirGroups []string) Outputs {
	return s.runTagging(Config{
		DryRun:      true,
		OutputJson:  true,
		Atomic:      true,
		Remote:      "origin",
		Branch:      "main",
		Directories: directories,
		DirGroups:   dirGroups,
	})
}

func (s *TaggingSuite) targetDryRun(targets []TargetConfig) Outputs {
	return s.runTagging(Config{
		DryRun:     true,
		OutputJson: true,
		Atomic:     true,
		Remote:     "origin",
		Branch:     "main",
		Targets:    targets,
	})
}

func (s *TaggingSuite) runTagging(config Config) Outputs {
	previousStdout := os.Stdout
	capturePath := filepath.Join(s.T().TempDir(), "outputs.json")
	captureFile, err := os.Create(capturePath)
	require.NoError(s.T(), err)
	os.Stdout = captureFile

	taggingErr := DoTagging(config)

	os.Stdout = previousStdout
	require.NoError(s.T(), captureFile.Close())
	require.NoError(s.T(), taggingErr)

	content, err := os.ReadFile(capturePath)
	require.NoError(s.T(), err)
	s.lastOutput = string(content)
	jsonStart := strings.LastIndex(s.lastOutput, `{"New_release_published"`)
	require.NotEqual(s.T(), -1, jsonStart)

	outputs := Outputs{}
	require.NoError(s.T(), json.Unmarshal([]byte(s.lastOutput[jsonStart:]), &outputs))
	return outputs
}

func (s *TaggingSuite) TestSharedDirectoryReleasesEveryGroup() {
	s.write("libs/shared/file.txt", "library change")
	s.commit("fix: shared library bug")

	outputs := s.tagDryRun(nil, []string{
		"services/api,libs/shared",
		"services/worker,libs/shared",
	})

	assert.Equal(s.T(), "api/v1.0.1,worker/v2.0.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "true,true", outputs.NewReleasePublished)
	// The library itself does not get a tag, because no group starts with it
	assert.Equal(s.T(), "api,worker", outputs.ReleasePackage)
}

func (s *TaggingSuite) TestOwnDirectoryReleasesOnlyItsGroup() {
	s.write("services/api/file.txt", "api change")
	s.commit("feat: new api endpoint")

	outputs := s.tagDryRun(nil, []string{
		"services/api,libs/shared",
		"services/worker,libs/shared",
	})

	assert.Equal(s.T(), "api/v1.1.0,worker/v2.0.0", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "true,false", outputs.NewReleasePublished)
}

func (s *TaggingSuite) TestGroupCountsCommitsFromEveryDirectoryOnce() {
	s.write("services/api/file.txt", "api change")
	s.write("libs/shared/file.txt", "library change")
	s.commit("fix: change both directories")

	outputs := s.tagDryRun(nil, []string{"services/api,libs/shared"})

	assert.Equal(s.T(), "api/v1.0.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "fix: change both directories", outputs.NewReleaseNotes)
}

func (s *TaggingSuite) TestSeparateDirectoriesKeepSeparateTags() {
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.tagDryRun([]string{"services/api", "services/worker"}, nil)

	assert.Equal(s.T(), "api/v1.0.1,worker/v2.0.0", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "true,false", outputs.NewReleasePublished)
}

// A directory keeps its own tag when a group also lists it, which is how a
// shared library can still be released on its own.
func (s *TaggingSuite) TestDirectoriesRunBeforeGroups() {
	s.write("libs/shared/file.txt", "library change")
	s.commit("fix: shared library bug")

	outputs := s.tagDryRun(
		[]string{"libs/shared"},
		[]string{"services/api,libs/shared"},
	)

	assert.Equal(s.T(), "shared/v0.1.1,api/v1.0.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "shared,api", outputs.ReleasePackage)
}

func (s *TaggingSuite) TestWholeRepositoryKeepsOneUnnamedTag() {
	s.write("libs/shared/file.txt", "library change")
	s.commit("fix: shared library bug")

	outputs := s.tagDryRun(nil, nil)

	assert.Equal(s.T(), "v0.1.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "", outputs.ReleasePackage)
}

func (s *TaggingSuite) TestNamedTargetUsesItsNameWithOnePath() {
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api"},
	}})

	assert.Equal(s.T(), "public-api/v0.1.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "public-api", outputs.ReleasePackage)
	assert.Contains(s.T(), outputs.NewReleaseNotesJson, `"package_public-api"`)
}

func (s *TaggingSuite) TestNamedTargetUsesEveryPath() {
	s.write("libs/shared/file.txt", "shared change")
	s.commit("feat: shared change")

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api", "libs/shared"},
	}})

	assert.Equal(s.T(), "public-api/v0.2.0", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "true", outputs.NewReleasePublished)
}

func (s *TaggingSuite) TestNamedTargetsCanShareAPathAndKeepOrder() {
	s.write("libs/shared/file.txt", "shared change")
	s.commit("fix: shared change")

	outputs := s.targetDryRun([]TargetConfig{
		{Name: "worker-public", Paths: []string{"services/worker", "libs/shared"}},
		{Name: "api-public", Paths: []string{"services/api", "libs/shared"}},
	})

	assert.Equal(s.T(), "worker-public,api-public", outputs.ReleasePackage)
	assert.Equal(
		s.T(),
		"worker-public/v0.1.1,api-public/v0.1.1",
		outputs.NewReleaseGitTag,
	)
}

func (s *TaggingSuite) TestLegacyFormsRunBeforeNamedTargets() {
	s.write("libs/shared/file.txt", "shared change")
	s.commit("fix: shared change")

	outputs := s.runTagging(Config{
		DryRun:      true,
		OutputJson:  true,
		Directories: []string{"libs/shared"},
		DirGroups:   []string{"services/api,libs/shared"},
		Targets: []TargetConfig{{
			Name:  "worker-public",
			Paths: []string{"services/worker", "libs/shared"},
		}},
	})

	assert.Equal(s.T(), "shared,api,worker-public", outputs.ReleasePackage)
}

// This configuration uses only interfaces that v0.5.0 supports. Its output
// values are a compatibility contract for named-target changes.
func (s *TaggingSuite) TestLegacyV050ConfigurationKeepsItsOutputs() {
	s.write("libs/shared/file.txt", "shared change")
	s.commit("fix: shared change")

	outputs := s.runTagging(Config{
		DryRun:      true,
		OutputJson:  true,
		Directories: []string{"services/worker"},
		DirGroups:   []string{"services/api,libs/shared"},
	})

	assert.Equal(s.T(), "false,true", outputs.NewReleasePublished)
	assert.Equal(s.T(), "worker,api", outputs.ReleasePackage)
	assert.Equal(s.T(), "worker/v2.0.0,api/v1.0.1", outputs.NewReleaseGitTag)
	assert.Equal(s.T(), "worker/v2.0.0,api/v1.0.0", outputs.LastReleaseGitTag)
	assert.Equal(s.T(), ",\nfix: shared change", outputs.NewReleaseNotes)
}

func (s *TaggingSuite) TestChangeOutsideNamedTargetKeepsUnchangedOutputs() {
	s.git("tag", "public-api/v3.2.1")
	s.write("services/worker/file.txt", "worker change")
	s.commit("fix: worker change")

	outputs := s.runTagging(Config{
		OutputJson: true,
		Targets: []TargetConfig{{
			Name:  "public-api",
			Paths: []string{"services/api"},
		}},
	})

	assert.Equal(s.T(), "false", outputs.NewReleasePublished)
	assert.Equal(s.T(), "public-api/v3.2.1", outputs.LastReleaseGitTag)
	assert.Equal(s.T(), outputs.LastReleaseGitTag, outputs.NewReleaseGitTag)
	command := exec.Command("git", "tag", "--list", "public-api/v3.2.2")
	command.Dir = s.repoDir
	content, err := command.Output()
	require.NoError(s.T(), err)
	assert.Empty(s.T(), string(content))
}

func (s *TaggingSuite) TestNamedTargetReadsOnlyItsExplicitTagName() {
	s.git("tag", "services/api/v9.0.0")
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api"},
	}})

	assert.Equal(s.T(), "public-api/v0.1.0", outputs.LastReleaseGitTag)
	assert.Equal(s.T(), "public-api/v0.1.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestLegacyTargetStillReadsAFullPathTag() {
	s.git("tag", "services/api/v4.0.0")
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "api/v4.0.0", outputs.LastReleaseGitTag)
	assert.Equal(s.T(), "api/v4.0.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestNamedPathsAreRelativeToTheGitRoot() {
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")
	require.NoError(s.T(), os.Chdir(filepath.Join(s.repoDir, "libs")))

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api"},
	}})

	assert.Equal(s.T(), "public-api/v0.1.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestNamedTargetCanUseAFilePath() {
	s.write("README.md", "readme change")
	s.commit("docs: update readme")

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "documentation",
		Paths: []string{"README.md"},
	}})

	assert.Equal(s.T(), "documentation/v0.1.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestNamedTargetDotPathUsesTheWholeRepository() {
	s.write("services/worker/file.txt", "worker change")
	s.commit("fix: worker change")

	outputs := s.targetDryRun([]TargetConfig{{
		Name:  "all-source",
		Paths: []string{"."},
	}})

	assert.Equal(s.T(), "all-source/v0.1.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestMultipleNamedTagsUseOneAtomicPush() {
	remoteDir := filepath.Join(s.T().TempDir(), "remote.git")
	command := exec.Command("git", "init", "-q", "--bare", remoteDir)
	require.NoError(s.T(), command.Run())
	s.git("remote", "add", "origin", remoteDir)

	s.write("services/api/file.txt", "api change")
	s.write("services/worker/file.txt", "worker change")
	s.commit("fix: service change")

	outputs := s.runTagging(Config{
		OutputJson: true,
		Atomic:     true,
		Remote:     "origin",
		Branch:     "",
		Targets: []TargetConfig{
			{Name: "public-api", Paths: []string{"services/api"}},
			{Name: "public-worker", Paths: []string{"services/worker"}},
		},
	})

	assert.Equal(
		s.T(),
		"public-api/v0.1.1,public-worker/v0.1.1",
		outputs.NewReleaseGitTag,
	)
	for _, tag := range []string{"public-api/v0.1.1", "public-worker/v0.1.1"} {
		check := exec.Command("git", "--git-dir", remoteDir, "rev-parse", "refs/tags/"+tag)
		content, err := check.Output()
		require.NoError(s.T(), err)
		assert.Equal(s.T(), s.headCommit(), string(content[:40]))
	}
	for _, tag := range []string{"public-api/v0.1", "public-api/v0"} {
		check := exec.Command("git", "--git-dir", remoteDir, "show-ref", "--verify", "refs/tags/"+tag)
		assert.Error(s.T(), check.Run())
	}
}

func (s *TaggingSuite) TestShortVersionsMoveMajorAndMinorTags() {
	firstHead := s.headCommit()
	s.git("tag", "api/v1.3.6")
	s.git("tag", "api/v1.3")
	s.git("tag", "api/v1")

	remoteDir := filepath.Join(s.T().TempDir(), "remote.git")
	command := exec.Command("git", "init", "-q", "--bare", remoteDir)
	require.NoError(s.T(), command.Run())
	s.git("remote", "add", "origin", remoteDir)
	s.git("push", "-q", "origin", "api/v1.3.6", "api/v1.3", "api/v1")

	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")
	newHead := s.headCommit()
	require.NotEqual(s.T(), firstHead, newHead)

	outputs := s.runTagging(Config{
		OutputJson:    true,
		Atomic:        true,
		Remote:        "origin",
		Branch:        "",
		ShortVersions: true,
		Directories:   []string{"services/api"},
	})

	assert.Equal(s.T(), "api/v1.3.7", outputs.NewReleaseGitTag)
	for _, tag := range []string{"api/v1.3.7", "api/v1.3", "api/v1"} {
		check := exec.Command("git", "--git-dir", remoteDir, "rev-parse", "refs/tags/"+tag)
		content, err := check.Output()
		require.NoError(s.T(), err)
		assert.Equal(s.T(), newHead, string(content[:40]))
	}
}

func (s *TaggingSuite) TestShortVersionFlagsCannotConflict() {
	err := DoTagging(Config{ShortVersions: true, SkipShortVersions: true})

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "cannot both be true")
}

func (s *TaggingSuite) TestDefaultShortVersionBehaviorWritesMigrationWarning() {
	s.targetDryRun([]TargetConfig{{Name: "public-api", Paths: []string{"services/api"}}})

	assert.Contains(s.T(), s.lastOutput, "Short version tags will become the default")
	assert.Contains(s.T(), s.lastOutput, "--skip-short-versions")
}

func (s *TaggingSuite) TestSkipShortVersionsSuppressesMigrationWarning() {
	s.runTagging(Config{
		DryRun:            true,
		OutputJson:        true,
		SkipShortVersions: true,
		Targets: []TargetConfig{{
			Name: "public-api", Paths: []string{"services/api"},
		}},
	})

	assert.NotContains(s.T(), s.lastOutput, "Short version tags will become the default")
}

func (s *TaggingSuite) TestErrorLogLevelSuppressesMigrationWarning() {
	previousLevel := logging.Log.GetLevel()
	logging.Log.SetLevel(logrus.ErrorLevel)
	s.T().Cleanup(func() { logging.Log.SetLevel(previousLevel) })

	s.targetDryRun([]TargetConfig{{Name: "public-api", Paths: []string{"services/api"}}})

	assert.NotContains(s.T(), s.lastOutput, "Short version tags will become the default")
}

// A --directories value keeps its old meaning. It is one literal path, even
// when it holds a comma, and it never becomes a group.
func (s *TaggingSuite) TestDirectoriesValueStaysOneLiteralPath() {
	groups, err := ParseDirectoryGroups(
		[]string{"services/api,libs/shared"},
		nil,
		s.repoDir,
	)

	require.NoError(s.T(), err)
	require.Len(s.T(), groups, 1)
	assert.Equal(s.T(), "services/api,libs/shared", groups[0].Directory)
	assert.Equal(s.T(), []string{"services/api,libs/shared"}, groups[0].CommitPaths())
}

func (s *TaggingSuite) TestFirstDirectoryNamesTheTag() {
	groups, err := ParseDirectoryGroups(
		nil,
		[]string{"services/api,libs/shared"},
		s.repoDir,
	)

	require.NoError(s.T(), err)
	require.Len(s.T(), groups, 1)
	assert.Equal(s.T(), "services/api", groups[0].Directory)
	assert.Equal(s.T(), "api", groups[0].PackageName())
	assert.Equal(s.T(), []string{"services/api", "libs/shared"}, groups[0].CommitPaths())
}

func (s *TaggingSuite) TestGroupDropsRepeatedDirectory() {
	groups, err := ParseDirectoryGroups(
		nil,
		[]string{" services/api , libs/shared , services/api "},
		s.repoDir,
	)

	require.NoError(s.T(), err)
	require.Len(s.T(), groups, 1)
	assert.Equal(s.T(), []string{"services/api", "libs/shared"}, groups[0].CommitPaths())
}

func (s *TaggingSuite) TestGitRootGroupUsesTheRepositoryName() {
	groups, err := ParseDirectoryGroups([]string{s.repoDir}, nil, s.repoDir)

	require.NoError(s.T(), err)
	require.Len(s.T(), groups, 1)
	assert.True(s.T(), groups[0].UseRoot)
	assert.Equal(s.T(), filepath.Base(s.repoDir), groups[0].PackageName())
	assert.Equal(s.T(), []string{"./"}, groups[0].CommitPaths())
}

func (s *TaggingSuite) TestTwoDirectoriesCanNotShareOneTagName() {
	_, err := ParseDirectoryGroups([]string{"services/api", "libs/api"}, nil, s.repoDir)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), `both tag the package "api"`)
}

func (s *TaggingSuite) TestDirectoryAndGroupCanNotShareOneTagName() {
	_, err := ParseDirectoryGroups(
		[]string{"services/api"},
		[]string{"libs/api,libs/shared"},
		s.repoDir,
	)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), `both tag the package "api"`)
}

func (s *TaggingSuite) TestEmptyGroupIsAnError() {
	_, err := ParseDirectoryGroups(nil, []string{" , "}, s.repoDir)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "does not name a directory")
}

func (s *TaggingSuite) TestDuplicateNamedTargetsAreAnError() {
	_, err := ParseReleaseTargets(nil, nil, []TargetConfig{
		{Name: "public-api", Paths: []string{"services/api"}},
		{Name: "public-api", Paths: []string{"libs/shared"}},
	}, s.repoDir)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), `both tag the package "public-api"`)
}

func (s *TaggingSuite) TestNamedTargetAndDirectoryCanNotShareAName() {
	_, err := ParseReleaseTargets(
		[]string{"services/api"},
		nil,
		[]TargetConfig{{Name: "api", Paths: []string{"libs/shared"}}},
		s.repoDir,
	)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), `both tag the package "api"`)
}

func (s *TaggingSuite) TestNamedTargetAndDirectoryGroupCanNotShareAName() {
	_, err := ParseReleaseTargets(
		nil,
		[]string{"services/api,libs/shared"},
		[]TargetConfig{{Name: "api", Paths: []string{"services/worker"}}},
		s.repoDir,
	)

	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), `both tag the package "api"`)
}

func (s *TaggingSuite) TestNamedTargetValidation() {
	tests := []struct {
		name   string
		target TargetConfig
		text   string
	}{
		{name: "empty name", target: TargetConfig{Paths: []string{"services/api"}}, text: "name must not be empty"},
		{name: "empty paths", target: TargetConfig{Name: "api"}, text: "at least one path"},
		{name: "empty path", target: TargetConfig{Name: "api", Paths: []string{""}}, text: "path must not be empty"},
		{name: "absolute path", target: TargetConfig{Name: "api", Paths: []string{"/services/api"}}, text: "relative to the Git root"},
		{name: "outside path", target: TargetConfig{Name: "api", Paths: []string{"../api"}}, text: "stay in the Git repository"},
		{name: "backslash path", target: TargetConfig{Name: "api", Paths: []string{`services\api`}}, text: "forward slashes"},
		{name: "comma name", target: TargetConfig{Name: "api,worker", Paths: []string{"services/api"}}, text: "not a safe Git tag prefix"},
		{name: "equals name", target: TargetConfig{Name: "api=worker", Paths: []string{"services/api"}}, text: "not a safe Git tag prefix"},
		{name: "slash name", target: TargetConfig{Name: "scope/api", Paths: []string{"services/api"}}, text: "not a safe Git tag prefix"},
		{name: "dot sequence", target: TargetConfig{Name: "api..next", Paths: []string{"services/api"}}, text: "not a safe Git tag prefix"},
		{name: "lock suffix", target: TargetConfig{Name: "api.lock", Paths: []string{"services/api"}}, text: "not a safe Git tag prefix"},
	}

	for _, test := range tests {
		s.Run(test.name, func() {
			_, err := ParseReleaseTargets(nil, nil, []TargetConfig{test.target}, s.repoDir)
			require.Error(s.T(), err)
			assert.Contains(s.T(), err.Error(), test.text)
		})
	}
}

func (s *TaggingSuite) TestNamedTargetNormalizesAndRemovesRepeatedPaths() {
	groups, err := ParseReleaseTargets(nil, nil, []TargetConfig{{
		Name:  "public-api",
		Paths: []string{"services/api", "services/./api"},
	}}, s.repoDir)

	require.NoError(s.T(), err)
	require.Len(s.T(), groups, 1)
	assert.Equal(s.T(), []string{"services/api"}, groups[0].Directories)
}

func TestParseTargetSpecifications(t *testing.T) {
	targets, err := ParseTargetSpecifications([]string{
		"public-api=services/api,libs/shared",
		"worker=services/worker",
	})

	require.NoError(t, err)
	assert.Equal(t, []TargetConfig{
		{Name: "public-api", Paths: []string{"services/api", "libs/shared"}},
		{Name: "worker", Paths: []string{"services/worker"}},
	}, targets)
}

func TestParseTargetSpecificationsRequiresEquals(t *testing.T) {
	_, err := ParseTargetSpecifications([]string{"public-api"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name=path[,path...]")
}

// A tag that is not a version, such as "nightly", must not stop the run.
func (s *TaggingSuite) TestNonVersionTagIsSkipped() {
	s.git("tag", "nightly")
	s.git("tag", "latest")
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "api/v1.0.1", outputs.NewReleaseGitTag)
}

// The highest version wins even when a lower version sits on a newer commit.
func (s *TaggingSuite) TestLatestVersionUsesSemverNotCommitDate() {
	// api/v2.0.0 goes on the first commit, which is older than api/v1.0.0
	firstCommit := func() string {
		command := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
		command.Dir = s.repoDir
		output, err := command.Output()
		require.NoError(s.T(), err)
		return string(output[:40])
	}
	s.write("services/api/file.txt", "api change")
	s.commit("fix: later change")
	s.git("tag", "api/v2.0.0", firstCommit())

	s.write("services/api/file.txt", "another api change")
	s.commit("fix: newest change")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "api/v2.0.0", outputs.LastReleaseGitTag)
	assert.Equal(s.T(), "api/v2.0.1", outputs.NewReleaseGitTag)
}

// The new head is the commit that the new tag points at.
func (s *TaggingSuite) TestNewReleaseGitHeadIsTheTaggedCommit() {
	s.write("services/api/file.txt", "api change")
	s.commit("fix: api change")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), s.headCommit(), outputs.NewReleaseGitHead)
}

// A type outside the allowed list does not change the version.
func (s *TaggingSuite) TestAllowedTypesLimitsWhatReleases() {
	s.write("services/api/file.txt", "api change")
	s.commit("feat: new api endpoint")

	outputs := s.runTagging(Config{
		DryRun:       true,
		OutputJson:   true,
		Atomic:       true,
		Remote:       "origin",
		Branch:       "main",
		AllowedTypes: []string{"fix"},
		Directories:  []string{"services/api"},
	})

	assert.Equal(s.T(), "false", outputs.NewReleasePublished)
	assert.Equal(s.T(), "api/v1.0.0", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestArbitraryConfiguredTypeReleases() {
	s.write("services/api/file.txt", "api change")
	s.commit("holiday: bring joy")

	outputs := s.runTagging(Config{
		DryRun:      true,
		OutputJson:  true,
		PatchTypes:  []string{"holiday"},
		Directories: []string{"services/api"},
	})

	assert.Equal(s.T(), "true", outputs.NewReleasePublished)
	assert.Equal(s.T(), "api/v1.0.1", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestBreakingChangeFooterReleasesMajorVersion() {
	s.write("services/api/file.txt", "api change")
	s.commit("holiday: change the api\n\nBREAKING CHANGE: old clients cannot connect")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "api/v2.0.0", outputs.NewReleaseGitTag)
}

func (s *TaggingSuite) TestCiAndPerfCommitsRelease() {
	s.write("services/api/file.txt", "api change")
	s.commit("ci: change the pipeline")
	s.write("services/api/file.txt", "another api change")
	s.commit("perf: make it fast")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "true", outputs.NewReleasePublished)
	assert.Equal(s.T(), "api/v1.0.1", outputs.NewReleaseGitTag)
}
