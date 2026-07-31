package core

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/semver-tags/core/semver"
)

// VersionInfo holds one version of one package, and the commit it points at.
type VersionInfo struct {
	Package    string
	Version    *semver.Semver
	CommitHash string
}

func (v *VersionInfo) Printable() string {
	retVal := "VersionInfo:\n"
	retVal += fmt.Sprintf("Package: '%s'\n", v.Package)
	if v.Version != nil {
		retVal += fmt.Sprintf("Version: %v\n", *v.Version)
	} else {
		retVal += "Version: nil\n"
	}
	retVal += fmt.Sprintf("CommitHash: %s\n", v.CommitHash)
	return retVal
}

// bumpForType gives the version part that each conventional commit type
// changes. A type that is not in this list does not change the version.
var bumpForType = map[string]semver.CommitType{
	"build":    semver.Patch,
	"chore":    semver.Patch,
	"ci":       semver.Patch,
	"docs":     semver.Patch,
	"feat":     semver.Minor,
	"fix":      semver.Patch,
	"perf":     semver.Patch,
	"refactor": semver.Patch,
	"revert":   semver.Patch,
	"style":    semver.Patch,
	"test":     semver.Patch,
}

// DefaultAllowedTypes gives the conventional commit types that change a
// version when the caller does not give a list.
func DefaultAllowedTypes() []string {
	types := make([]string, 0, len(bumpForType))
	for name := range bumpForType {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}

// AnalyzeCommitMessage gives the version part that one commit subject changes.
// A type that is not in allowedTypes changes nothing. An empty allowedTypes
// means the default list, so a wrong setting can not stop every release.
func AnalyzeCommitMessage(message string, allowedTypes []string) semver.CommitType {
	typeAndScope, _, found := strings.Cut(message, ":")
	if !found {
		return semver.NotConventional
	}

	// A breaking change is major whatever the allowed types are
	if strings.TrimSpace(typeAndScope) == "BREAKING CHANGE" {
		return semver.Major
	}

	breaking := strings.HasSuffix(typeAndScope, "!")
	commitType, _, _ := strings.Cut(strings.TrimSuffix(typeAndScope, "!"), "(")
	commitType = strings.TrimSpace(strings.TrimSuffix(commitType, "!"))

	bump, known := bumpForType[commitType]
	if !known {
		return semver.NotConventional
	}
	if len(allowedTypes) == 0 {
		allowedTypes = DefaultAllowedTypes()
	}
	if !slices.Contains(allowedTypes, commitType) {
		return semver.NotConventional
	}

	if breaking {
		return semver.Major
	}
	return bump
}

// ParseVersionInfo reads one "tag,commit" line into a version. The tag is
// "vX.Y.Z" for the whole repository, or "package/vX.Y.Z" for one package.
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

// tagger runs one tagging pass. It holds the repository tags, so it reads them
// only one time for all of the directory groups.
type tagger struct {
	config     Config
	head       string
	tags       []*VersionInfo
	tagsLoaded bool
}

// loadTags reads every tag one time. A tag that is not a version, such as
// "nightly", is skipped instead of stopping the run.
func (t *tagger) loadTags() error {
	if t.tagsLoaded {
		return nil
	}

	lines, err := repositoryTagLines()
	if err != nil {
		return err
	}

	for _, line := range lines {
		logging.Log.Info(fmt.Sprintf("Tag line found: %s", line))
		version, err := ParseVersionInfo(line)
		if err != nil {
			logging.Log.Info(fmt.Sprintf("Skipping tag that is not a version: %s", line))
			continue
		}
		t.tags = append(t.tags, version)
	}

	t.tagsLoaded = true
	return nil
}

// latestVersion gives the highest released version of one group. It uses
// semantic version precedence, not the commit date, so a tag on an old commit
// can not hide a higher version.
func (t *tagger) latestVersion(group DirectoryVersionInfo) (*VersionInfo, error) {
	if err := t.loadTags(); err != nil {
		return nil, err
	}

	packageName := group.PackageName()
	var highest *VersionInfo
	for _, tag := range t.tags {
		// The full path match keeps a tag that names a directory path
		if tag.Package != packageName && tag.Package != group.Directory {
			continue
		}
		if highest == nil || tag.Version.Compare(highest.Version) > 0 {
			highest = tag
		}
	}

	if highest != nil {
		return &VersionInfo{
			Package:    packageName,
			Version:    highest.Version.Clone(),
			CommitHash: highest.CommitHash,
		}, nil
	}

	// None found, so provide the last version as 0.1.0 and the first parentless commit we find
	commit, err := firstCommit()
	if err != nil {
		return nil, err
	}
	return &VersionInfo{
		Package:    packageName,
		Version:    semver.NewSemver(0, 1, 0),
		CommitHash: commit,
	}, nil
}

// analyzeCommits reads the commits of one group since its last version, then
// works out the next version and the release notes.
func (t *tagger) analyzeCommits(group *DirectoryVersionInfo) error {
	nextVersion := group.LastVersion.Version.Clone()
	commitPaths := group.CommitPaths()

	logging.Log.Info(fmt.Sprintf(
		"Analyzing Commits for package: %s in %v",
		group.LastVersion.Package,
		commitPaths,
	))
	subjects, err := commitSubjects(group.LastVersion.CommitHash, commitPaths)
	if err != nil {
		return err
	}

	highest := semver.NotConventional
	releaseNotes := []string{}
	for _, subject := range subjects {
		logging.Log.Info(fmt.Sprintf("Analyzing Commit: %s", subject))
		commitType := AnalyzeCommitMessage(subject, t.config.AllowedTypes)
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
		}
		releaseNotes = append(releaseNotes, subject)
	}

	// If no change is needed, this will be a noOp
	nextVersion.BumpVersion(highest, t.config.PreReleaseString, t.config.BuildString)

	// This only happens after no errors
	group.NextVersion = &VersionInfo{
		Package:    group.LastVersion.Package,
		Version:    nextVersion,
		CommitHash: t.head,
	}
	group.ReleaseNotes = releaseNotes
	return nil
}
