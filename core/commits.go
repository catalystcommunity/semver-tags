package core

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/catalystcommunity/app-utils-go/logging"
	"github.com/catalystcommunity/semver-tags/core/semver"
)

const BreakingChangeType = "BREAKING CHANGE"

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

// defaultPatchTypes gives the commit types that make a patch release when the
// caller does not configure the patch list.
var defaultPatchTypes = []string{
	"build", "chore", "ci", "docs", "fix", "perf", "refactor", "revert", "style", "test",
}

var defaultMinorTypes = []string{"feat"}

// DefaultPatchTypes gives the commit types that make a patch release by
// default.
func DefaultPatchTypes() []string {
	return slices.Clone(defaultPatchTypes)
}

// DefaultMinorTypes gives the commit types that make a minor release by
// default.
func DefaultMinorTypes() []string {
	return slices.Clone(defaultMinorTypes)
}

// DefaultMajorTypes gives the commit types that make a major release by
// default. A breaking marker makes a major release separately from this list.
func DefaultMajorTypes() []string {
	return []string{}
}

// DefaultAllowedTypes gives the conventional commit types that change a
// version when the caller does not give a list.
func DefaultAllowedTypes() []string {
	types := append(DefaultPatchTypes(), DefaultMinorTypes()...)
	types = append(types, BreakingChangeType)
	sort.Strings(types)
	return types
}

type bumpRules struct {
	levels  map[string]semver.CommitType
	allowed map[string]struct{}
}

func normalizeTypeList(values []string) []string {
	var normalized []string
	for _, value := range values {
		for _, member := range strings.Split(value, ",") {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if strings.EqualFold(member, BreakingChangeType) ||
				strings.EqualFold(member, "BREAKING-CHANGE") {
				member = BreakingChangeType
			} else {
				member = strings.ToLower(member)
			}
			if !slices.Contains(normalized, member) {
				normalized = append(normalized, member)
			}
		}
	}
	return normalized
}

func newBumpRules(config Config) (bumpRules, error) {
	patchTypes := normalizeTypeList(config.PatchTypes)
	if len(patchTypes) == 0 {
		patchTypes = DefaultPatchTypes()
	}
	minorTypes := normalizeTypeList(config.MinorTypes)
	if len(minorTypes) == 0 {
		minorTypes = DefaultMinorTypes()
	}
	majorTypes := normalizeTypeList(config.MajorTypes)

	if slices.Contains(minorTypes, "fix") || slices.Contains(majorTypes, "fix") {
		return bumpRules{}, fmt.Errorf("commit type %q must make a patch release", "fix")
	}
	if slices.Contains(patchTypes, "feat") || slices.Contains(majorTypes, "feat") {
		return bumpRules{}, fmt.Errorf("commit type %q must make a minor release", "feat")
	}
	if !slices.Contains(patchTypes, "fix") {
		patchTypes = append(patchTypes, "fix")
	}
	if !slices.Contains(minorTypes, "feat") {
		minorTypes = append(minorTypes, "feat")
	}

	rules := bumpRules{levels: map[string]semver.CommitType{}}
	addTypes := func(values []string, level semver.CommitType) error {
		for _, value := range values {
			if value == BreakingChangeType {
				return fmt.Errorf("%q is a breaking marker and cannot be a configured commit type", value)
			}
			if previous, found := rules.levels[value]; found && previous != level {
				return fmt.Errorf("commit type %q is configured for more than one version level", value)
			}
			rules.levels[value] = level
		}
		return nil
	}
	if err := addTypes(patchTypes, semver.Patch); err != nil {
		return bumpRules{}, err
	}
	if err := addTypes(minorTypes, semver.Minor); err != nil {
		return bumpRules{}, err
	}
	if err := addTypes(majorTypes, semver.Major); err != nil {
		return bumpRules{}, err
	}

	allowedTypes := normalizeTypeList(config.AllowedTypes)
	if len(allowedTypes) == 0 {
		for value := range rules.levels {
			allowedTypes = append(allowedTypes, value)
		}
		allowedTypes = append(allowedTypes, BreakingChangeType)
	}
	rules.allowed = make(map[string]struct{}, len(allowedTypes))
	for _, value := range allowedTypes {
		rules.allowed[value] = struct{}{}
	}
	return rules, nil
}

func hasBreakingChange(message string) bool {
	subject := strings.SplitN(message, "\n", 2)[0]
	typeAndScope, _, found := strings.Cut(subject, ":")
	if found && strings.HasSuffix(strings.TrimSpace(typeAndScope), "!") {
		return true
	}
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(line, "BREAKING CHANGE: ") ||
			strings.HasPrefix(line, "BREAKING-CHANGE: ") {
			return true
		}
	}
	return false
}

func analyzeCommitMessage(message string, rules bumpRules) semver.CommitType {
	if hasBreakingChange(message) {
		if _, allowed := rules.allowed[BreakingChangeType]; !allowed {
			return semver.NotConventional
		}
		return semver.Major
	}

	subject := strings.SplitN(message, "\n", 2)[0]
	typeAndScope, _, found := strings.Cut(subject, ":")
	if !found {
		return semver.NotConventional
	}
	commitType, _, _ := strings.Cut(strings.TrimSpace(typeAndScope), "(")
	commitType = strings.ToLower(strings.TrimSpace(commitType))
	if _, allowed := rules.allowed[commitType]; !allowed {
		return semver.NotConventional
	}
	return rules.levels[commitType]
}

// AnalyzeCommitMessage gives the version part that one commit subject changes.
// A type that is not in allowedTypes changes nothing. An empty allowedTypes
// permits every default type and a breaking marker.
func AnalyzeCommitMessage(message string, allowedTypes []string) semver.CommitType {
	rules, err := newBumpRules(Config{AllowedTypes: allowedTypes})
	if err != nil {
		return semver.NotConventional
	}
	return analyzeCommitMessage(message, rules)
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
	rules      bumpRules
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
		matches := tag.Package == packageName
		for _, alias := range group.TagAliases {
			matches = matches || tag.Package == alias
		}
		if !matches {
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
	commits, err := commitMessages(group.LastVersion.CommitHash, commitPaths)
	if err != nil {
		return err
	}

	highest := semver.NotConventional
	releaseNotes := []string{}
	for _, commit := range commits {
		logging.Log.Info(fmt.Sprintf("Analyzing Commit: %s", commit.Subject))
		commitType := analyzeCommitMessage(commit.Message, t.rules)
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
		releaseNotes = append(releaseNotes, commit.Subject)
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
