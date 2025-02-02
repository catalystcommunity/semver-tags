package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/semver-tags/core/semver"

	gha "github.com/sethvargo/go-githubactions"
)

type Outputs struct {
	New_release_published     string
	New_release_version       string
	New_release_major_version string
	New_release_minor_version string
	New_release_patch_version string
	New_release_git_head      string
	New_release_notes         string
	New_release_notes_json    string
	Dry_run                   string
	Release_package           string
	New_release_git_tag       string
	Last_release_version      string
	Last_release_git_head     string
	Last_release_git_tag      string
}

type VersionInfo struct {
	Package    string
	Version    *semver.Semver
	CommitHash string
}

func (v *VersionInfo) Printable() string {
	var retVal string = "VersionInfo:\n"
	retVal += fmt.Sprintf("Package: '%s'\n", v.Package)
	retVal += fmt.Sprintf("Version: %v\n", *v.Version)
	retVal += fmt.Sprintf("CommitHash: %s\n", v.CommitHash)
	return retVal
}

type DirectoryVersionInfo struct {
	Directory    string
	FullPath     string
	LastVersion  *VersionInfo
	NextVersion  *VersionInfo
	ReleaseNotes []string
	UseRoot      bool
}

func (d *DirectoryVersionInfo) Printable() string {
	var retVal string = "DirectoryVersionInfo:\n"
	retVal += fmt.Sprintf("Directory: %s\n", d.Directory)
	retVal += fmt.Sprintf("FullPath: %s\n", d.FullPath)
	if d.LastVersion != nil {
		retVal += fmt.Sprintf("LastVersion: %s\n", d.LastVersion.Printable())
	} else {
		retVal += "LastVersion: nil\n"
	}
	if d.NextVersion != nil {
		retVal += fmt.Sprintf("NextVersion: %s\n", d.NextVersion.Printable())
	} else {
		retVal += "NextVersion: nil\n"
	}
	retVal += fmt.Sprintf("ReleaseNotes: %v\n", d.ReleaseNotes)
	return retVal
}

var latestTagInfo []*VersionInfo

func AnalyzeCommitMessage(message string) semver.CommitType {
	if !strings.Contains(message, ":") {
		return semver.NotConventional
	}

	split := strings.SplitN(message, ":", 2)
	if len(split) < 2 {
		return semver.NotConventional
	}

	typeAndScope := split[0]
	typeSplit := strings.SplitN(typeAndScope, "(", 2)

	commitType := typeSplit[0]

	if strings.HasSuffix(commitType, "!") || strings.HasSuffix(typeAndScope, "!") {
		return semver.Major
	}

	switch commitType {
	case "fix", "chore", "docs", "style", "refactor", "test", "revert":
		return semver.Patch
	case "feat":
		return semver.Minor
	case "BREAKING CHANGE":
		return semver.Major
	default:
		return semver.NotConventional
	}
}

func ParseVersionInfo(line string) (*VersionInfo, error) {
	split := strings.Split(line, ",")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid format")
	}

	parts := strings.Split(split[0], "/")
	// the last part is the version, which we can clip the v off of
	versionPart := strings.TrimPrefix(parts[len(parts)-1], "v")
	// everything else is the package name
	packageName := strings.Join(parts[:len(parts)-1], "/")

	// If there's a PreRelease string, it will be after the first -
	versionComponents := strings.SplitN(versionPart, "-", 2)
	version := versionComponents[0]

	var preRelease, build string
	if len(versionComponents) > 1 {
		preRelease = versionComponents[1]
	}

	// If there is a build string, we'll see it in the PreRelease now after the +
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
		Package: packageName,
		Version: &semver.Semver{
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

func GetGitRootDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()

	if err != nil {
		return "", err
	}

	return strings.Trim(string(output), "\n"), nil
}

func GetLatestVersion(dir DirectoryVersionInfo, preRelease string) (*VersionInfo, error) {
	var retVal *VersionInfo

	// If we don't have tag info, get it for the first run
	if len(latestTagInfo) == 0 {
		cmd := exec.Command("git", "for-each-ref", "--sort=-committerdate", "--format", "%(refname:short),%(objectname)", "refs/tags")
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("can not get git tags: %s\n%s", err, string(output))
		}

		for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
			logging.Log.Info(fmt.Sprintf("Tag line found: %s", line))
			if len(line) == 0 {
				continue
			}
			v, err := ParseVersionInfo(line)
			if err != nil {
				return nil, fmt.Errorf("can not parse tag line: %s-h%s", line, err)
			}

			latestTagInfo = append(latestTagInfo, v)
		}
	}

	// Now no matter what we should have latestTagInfo from the start of the run
	// So we can just find the first that applies
	pathParts := strings.Split(strings.TrimRight(dir.Directory, "/"), "/")
	packageName := pathParts[len(pathParts)-1]
	if dir.UseRoot {
		packageName = dir.Directory
	}
	for _, tag := range latestTagInfo {
		// This is redundant in the case of UseRoot, which affects git command paths
		if tag.Package == packageName || tag.Package == dir.Directory {
			retVal = &VersionInfo{
				Package:    packageName,
				Version:    tag.Version.Clone(),
				CommitHash: tag.CommitHash,
			}
			return retVal, nil
		}
	}

	// None found, so provide the last version as 0.1.0 and the first parentless commit we find
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("can not get a parentless commit, so no root to determine: %s\n%s", err, string(output))
	}
	retVal = &VersionInfo{
		Package:    packageName,
		Version:    semver.NewSemver(0, 1, 0),
		CommitHash: strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")[0],
	}
	return retVal, nil
}

func AnalyzeCommits(dir *DirectoryVersionInfo, preRelease string, build string) error {
	nextVersion := dir.LastVersion.Version.Clone()
	packagePath := strings.TrimRight(dir.Directory, "/")
	if dir.UseRoot {
		packagePath = "./"
	}
	if packagePath == "" {
		packagePath = strings.TrimRight(dir.FullPath, "/")
	}

	logging.Log.Info(fmt.Sprintf("Analyzing Commits for package: %s", dir.LastVersion.Package))
	cmd := exec.Command("git", "log", "--pretty=format:%s", fmt.Sprintf("%s..HEAD", dir.LastVersion.CommitHash), "--", packagePath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("can not get git commits: %s\n%s", err, string(output))
	}

	highest := semver.NotConventional
	release_notes_items := []string{}

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
		case semver.NotConventional:
			logging.Log.Info("Not a conventional commit")
		case semver.Patch:
			logging.Log.Info("Found Patch commit")
		case semver.Minor:
			logging.Log.Info("Found Minor commit")
		case semver.Major:
			logging.Log.Info("Found Major commit")
		default:
			return fmt.Errorf("could not parse commit message: %s", string(line))
		}
		release_notes_items = append(release_notes_items, fmt.Sprintf("%s", line))
	}

	// If no change is needed, this will be a noOp
	nextVersion.BumpVersion(highest, preRelease, build)

	// This only happens after no errors
	dir.NextVersion = &VersionInfo{Version: nextVersion, Package: dir.LastVersion.Package}
	dir.ReleaseNotes = release_notes_items
	return nil
}

func EscapeStringForJSON(input string) (string, error) {
	escaped, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	// Convert []byte to string and remove the extra quotes added by Marshal
	return string(escaped[1 : len(escaped)-1]), nil
}

func GenerateOutputs(results []DirectoryVersionInfo, dry_run bool) Outputs {
	retVal := Outputs{}

	// We're building a json string object for parsing release notes with whatever
	retVal.New_release_notes_json = `{"new_release_notes_escaped":{`
	for _, result := range results {
		if result.NextVersion.Version.FormattedString() == result.LastVersion.Version.FormattedString() {
			retVal.New_release_published += "false,"
		} else {
			retVal.New_release_published += "true,"
		}

		retVal.Release_package += result.NextVersion.Package + ","

		retVal.New_release_version += fmt.Sprintf("%d.%d.%d", result.NextVersion.Version.Major, result.NextVersion.Version.Minor, result.NextVersion.Version.Patch) + ","
		retVal.New_release_major_version += fmt.Sprintf("%d", result.NextVersion.Version.Major) + ","
		retVal.New_release_minor_version += fmt.Sprintf("%d", result.NextVersion.Version.Minor) + ","
		retVal.New_release_patch_version += fmt.Sprintf("%d", result.NextVersion.Version.Patch) + ","
		retVal.New_release_git_head += result.NextVersion.CommitHash + ","
		retVal.New_release_notes += strings.Join(result.ReleaseNotes, "\n") + ",\n"
		result_escaped_release_notes := ""
		for _, note := range result.ReleaseNotes {
			escaped_note, err := EscapeStringForJSON(note)
			if err != nil {
				logging.Log.Error(fmt.Sprintf("Error escaping release note. \n%s\nRelease note: %s", err, note))
				continue
			}
			result_escaped_release_notes += "\"" + escaped_note + "\","
		}
		result_escaped_release_notes = strings.TrimRight(result_escaped_release_notes, ",")
		retVal.New_release_notes_json += `"package_` + result.NextVersion.Package + `":[` + result_escaped_release_notes + "],"
		retVal.Dry_run += strconv.FormatBool(dry_run) + ","
		prepended_package := result.NextVersion.Package
		if result.NextVersion.Package != "" {
			prepended_package += "/"
		}
		retVal.New_release_git_tag += prepended_package + result.NextVersion.Version.FormattedString() + ","
		retVal.Last_release_version += fmt.Sprintf("%d.%d.%d", result.LastVersion.Version.Major, result.LastVersion.Version.Minor, result.LastVersion.Version.Patch) + ","
		retVal.Last_release_git_head += result.LastVersion.CommitHash + ","
		retVal.Last_release_git_tag += prepended_package + result.LastVersion.Version.FormattedString() + ","
	}
	// Shave off the last comma from the set of package notes
	retVal.New_release_notes_json = strings.TrimRight(retVal.New_release_notes_json, ",")
	retVal.New_release_notes_json += `}}`
	re := regexp.MustCompile(`\r?\n`)
	retVal.New_release_notes_json = re.ReplaceAllString(retVal.New_release_notes_json, "\\n")

	// Clean up trailing items
	retVal.New_release_published = strings.TrimRight(retVal.New_release_published, ",")
	retVal.New_release_version = strings.TrimRight(retVal.New_release_version, ",")
	retVal.New_release_major_version = strings.TrimRight(retVal.New_release_major_version, ",")
	retVal.New_release_minor_version = strings.TrimRight(retVal.New_release_minor_version, ",")
	retVal.New_release_patch_version = strings.TrimRight(retVal.New_release_patch_version, ",")
	retVal.New_release_git_head = strings.TrimRight(retVal.New_release_git_head, ",")
	retVal.New_release_notes = strings.TrimRight(retVal.New_release_notes, ",\n")
	retVal.Dry_run = strings.TrimRight(retVal.Dry_run, ",")
	retVal.Release_package = strings.TrimRight(retVal.Release_package, ",\n")
	retVal.New_release_git_tag = strings.TrimRight(retVal.New_release_git_tag, ",")
	retVal.Last_release_version = strings.TrimRight(retVal.Last_release_version, ",")
	retVal.Last_release_git_head = strings.TrimRight(retVal.Last_release_git_head, ",")
	retVal.Last_release_git_tag = strings.TrimRight(retVal.Last_release_git_tag, ",")

	return retVal
}

func SetGithubActionOutputs(results Outputs) {
	gha.SetOutput("new_release_published", results.New_release_published)
	gha.SetOutput("new_release_version", results.New_release_version)
	gha.SetOutput("new_release_major_version", results.New_release_major_version)
	gha.SetOutput("new_release_minor_version", results.New_release_minor_version)
	gha.SetOutput("new_release_patch_version", results.New_release_patch_version)
	gha.SetOutput("new_release_git_head", results.New_release_git_head)
	gha.SetOutput("new_release_notes", results.New_release_notes)
	gha.SetOutput("new_release_notes_json", results.New_release_notes_json)
	gha.SetOutput("dry_run", results.Dry_run)
	gha.SetOutput("release_package", results.Release_package)
	gha.SetOutput("new_release_git_tag", results.New_release_git_tag)
	gha.SetOutput("last_release_version", results.Last_release_version)
	gha.SetOutput("last_release_git_head", results.Last_release_git_head)
	gha.SetOutput("last_release_git_tag", results.Last_release_git_tag)
}

func DoTagging(
	DryRun bool,
	GithubAction bool,
	OutputJson bool,
	Atomic bool,
	PreReleaseString string,
	BuildString string,
	Remote string,
	Branch string,
	Directories []string,
) error {
	// Make sure we're in a git repo with a git command or this is pointless
	if !IsGitRepo() {
		return errors.New("current directory is not a git repo, nothing to do")
	}

	// We deal in full paths for consistency, so we need to know what to prepend to subdirectories
	gitRoot, rootDirErr := GetGitRootDir()
	if rootDirErr != nil {
		return rootDirErr
	}

	var results []DirectoryVersionInfo

	gitRootPath, _ := filepath.Abs(gitRoot)
	// We need to know if we're in full repo mode or subdirectory mode
	isFullRepo := len(Directories) == 0
	// This will never run if we're in fullRepo mode
	for _, dir := range Directories {
		useRoot := false
		fullPath := gitRoot
		sanedir := strings.Trim(dir, string(os.PathSeparator))
		dirPath, _ := filepath.Abs(dir)
		if gitRootPath == dirPath {
			useRoot = true
			sanedir = path.Base(gitRootPath)
			fullPath = path.Dir(gitRoot)
		}
		results = append(results, DirectoryVersionInfo{
			Directory: sanedir,
			FullPath:  fullPath,
			UseRoot:   useRoot,
		})
	}
	if isFullRepo {
		results = append(results, DirectoryVersionInfo{Directory: "", FullPath: gitRoot})
	}

	var lastVersionError error = nil
	for idx, _ := range results {
		// Get the latest tag and hash that applies for this directory
		results[idx].LastVersion, lastVersionError = GetLatestVersion(results[idx], PreReleaseString)
		if lastVersionError != nil {
			return lastVersionError
		}
		// Now analyze the commit history for that directory and only that directory
		// Also, calculate the next version
		commitsErr := AnalyzeCommits(&results[idx], PreReleaseString, BuildString)

		if commitsErr != nil {
			return commitsErr
		}
	}

	// Process what tags we should be making
	for _, result := range results {
		if result.NextVersion == nil || result.LastVersion.Version.FormattedString() == result.NextVersion.Version.FormattedString() {
			// This hasn't changed, so we don't need to do anything
			logging.Log.Info(fmt.Sprintf("No new version for: %s", result.Printable()))
			continue
		}
		// We have a nextVersion, so build the tag with the optional package name
		tag := strings.TrimLeft(result.NextVersion.Package+"/"+result.NextVersion.Version.FormattedString(), "/")

		// If not in dry-run, tag stuff for each thing that changed
		if DryRun {
			logging.Log.Info(fmt.Sprintf("We would be tagging a new version: %s", tag))
		} else {
			logging.Log.Info(fmt.Sprintf("Tagging new version: %s", tag))
			cmd := exec.Command("git", "tag", tag)
			output, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("error tagging: %s\n%s", err, string(output))
			}
		}
	}

	Outputs := GenerateOutputs(results, DryRun)
	// We don't need to push tags if this is a dry run
	if !DryRun {
		tags := strings.Split(Outputs.New_release_git_tag, ",")
		cmdArgs := []string{"push"}
		if Atomic {
			cmdArgs = append(cmdArgs, "--atomic")
		}
		cmdArgs = append(cmdArgs, []string{Remote, Branch}...)
		cmdArgs = append(cmdArgs, tags...)
		// All tags should be there, so push! This prevents tags being pushed if there were errors
		// Ex. cmd: git push --atomic origin main tagOne tagTwo
		cmd := exec.Command("git", cmdArgs...)

		cmdOutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("error getting stdout pipe: %s", err)
		}
		cmdErrPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("error getting stderr pipe: %s", err)
		}

		cmd.Start()
		output, err := io.ReadAll(cmdOutPipe)
		if err != nil {
			return fmt.Errorf("error reading cmd stdout: %s", err)
		}
		cmdErrOut, err := io.ReadAll(cmdErrPipe)
		if err != nil {
			return fmt.Errorf("error reading cmd stderr: %s", err)
		}

		err = cmd.Wait()
		if err != nil {
			return fmt.Errorf("error pushing tags: %s\n%s\n%s", err, string(output), string(cmdErrOut))
		}
	}

	// If in githubactions, output each output, comma separated for each directory
	if GithubAction {
		SetGithubActionOutputs(Outputs)
	}

	if OutputJson {
		outputJson, err := json.Marshal(Outputs)
		if err != nil {
			return err
		}
		fmt.Print(string(outputJson))
	}

	return nil
}
