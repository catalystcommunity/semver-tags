package core

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
}

func TestTaggingSuite(t *testing.T) {
	suite.Run(t, new(TaggingSuite))
}

func (s *TaggingSuite) SetupTest() {
	previousDir, err := os.Getwd()
	require.NoError(s.T(), err)
	s.previousDir = previousDir
	s.commitCount = 0

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

	outputs := Outputs{}
	require.NoError(s.T(), json.Unmarshal(content, &outputs))
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

func (s *TaggingSuite) TestCiAndPerfCommitsRelease() {
	s.write("services/api/file.txt", "api change")
	s.commit("ci: change the pipeline")
	s.write("services/api/file.txt", "another api change")
	s.commit("perf: make it fast")

	outputs := s.tagDryRun([]string{"services/api"}, nil)

	assert.Equal(s.T(), "true", outputs.NewReleasePublished)
	assert.Equal(s.T(), "api/v1.0.1", outputs.NewReleaseGitTag)
}
