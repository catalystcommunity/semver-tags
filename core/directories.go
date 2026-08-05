package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// TargetConfig separates the public release name from the paths that affect
// the release.
type TargetConfig struct {
	Name  string   `mapstructure:"name" yaml:"name"`
	Paths []string `mapstructure:"paths" yaml:"paths"`
}

// DirectoryVersionInfo holds one release target. Package is its public name.
// Directories holds the Git paths that affect it. Directory and TagAliases
// keep the legacy directory-name behavior.
type DirectoryVersionInfo struct {
	Directory    string
	Directories  []string
	Package      string
	TagAliases   []string
	FullPath     string
	LastVersion  *VersionInfo
	NextVersion  *VersionInfo
	ReleaseNotes []string
	RootRelative bool
}

// PackageName gives the package part of the tag. Parsed targets store this
// value explicitly. The fallback keeps callers that construct legacy values.
func (d *DirectoryVersionInfo) PackageName() string {
	if d.Package != "" {
		return d.Package
	}
	trimmed := strings.TrimRight(d.Directory, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

// CommitPaths gives the git paths to limit the commit history to. In whole
// repo mode the group has no directories, so the full path is the limit.
func (d *DirectoryVersionInfo) CommitPaths() []string {
	if len(d.Directories) > 0 {
		if d.RootRelative {
			paths := make([]string, 0, len(d.Directories))
			for _, directory := range d.Directories {
				if directory == "." {
					paths = append(paths, ":(top)")
					continue
				}
				paths = append(paths, ":(top,literal)"+directory)
			}
			return paths
		}
		return d.Directories
	}
	return []string{strings.TrimRight(d.FullPath, "/")}
}

func (d *DirectoryVersionInfo) Printable() string {
	retVal := "DirectoryVersionInfo:\n"
	if d.RootRelative {
		retVal += fmt.Sprintf("Package: %s\n", d.PackageName())
	}
	retVal += fmt.Sprintf("Directory: %s\n", d.Directory)
	retVal += fmt.Sprintf("Directories: %v\n", d.Directories)
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

func splitDirectoryGroup(value string) []string {
	var members []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		members = append(members, part)
	}
	return members
}

// ParseTargetSpecifications reads the compact form that the command-line and
// environment interfaces use. A comma separates paths. This form does not
// have an escaping syntax.
func ParseTargetSpecifications(values []string) ([]TargetConfig, error) {
	targets := make([]TargetConfig, 0, len(values))
	for _, value := range values {
		name, pathsText, found := strings.Cut(value, "=")
		if !found {
			return nil, fmt.Errorf("target %q must have the form name=path[,path...]", value)
		}

		paths := strings.Split(pathsText, ",")
		for index := range paths {
			paths[index] = strings.TrimSpace(paths[index])
		}
		targets = append(targets, TargetConfig{
			Name:  strings.TrimSpace(name),
			Paths: paths,
		})
	}
	return targets, nil
}

// appendNewPath adds a path only when the list does not hold it, because a
// repeated git path makes a duplicate release note.
func appendNewPath(paths []string, candidate string) []string {
	for _, existing := range paths {
		if existing == candidate {
			return paths
		}
	}
	return append(paths, candidate)
}

// commitPathFor gives the git path for one directory of a group. The git root
// becomes "./" because git limits the history from the current directory.
func commitPathFor(dir string, gitRootPath string) (string, error) {
	dirPath, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("can not resolve directory %s: %w", dir, err)
	}
	if dirPath == gitRootPath {
		return "./", nil
	}
	return strings.Trim(dir, string(os.PathSeparator)), nil
}

// newDirectoryGroup makes one tag group from the directories it holds. The
// first directory names the tag, and a commit in any of them changes that tag.
func newDirectoryGroup(
	members []string,
	gitRoot string,
	gitRootPath string,
) (DirectoryVersionInfo, error) {
	group := DirectoryVersionInfo{FullPath: gitRoot}

	primaryPath, err := filepath.Abs(members[0])
	if err != nil {
		return group, fmt.Errorf("can not resolve directory %s: %w", members[0], err)
	}
	if primaryPath == gitRootPath {
		group.Directory = path.Base(gitRootPath)
		group.FullPath = path.Dir(gitRoot)
	} else {
		group.Directory = strings.Trim(members[0], string(os.PathSeparator))
	}

	for _, member := range members {
		commitPath, err := commitPathFor(member, gitRootPath)
		if err != nil {
			return group, err
		}
		group.Directories = appendNewPath(group.Directories, commitPath)
	}
	group.Package = group.PackageName()
	if group.Directory != group.Package {
		group.TagAliases = []string{group.Directory}
	}

	return group, nil
}

var targetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func validateTargetName(name string) error {
	if name == "" {
		return fmt.Errorf("target name must not be empty")
	}
	if !targetNamePattern.MatchString(name) ||
		strings.Contains(name, "..") ||
		strings.HasSuffix(name, ".") ||
		strings.HasSuffix(strings.ToLower(name), ".lock") {
		return fmt.Errorf(
			"target name %q is not a safe Git tag prefix; use letters, digits, dots, underscores, and hyphens",
			name,
		)
	}
	if _, err := runGit("check-ref-format", "refs/tags/"+name+"/v0.0.0"); err != nil {
		return fmt.Errorf("target name %q is not a valid Git tag prefix", name)
	}
	return nil
}

// normalizeTargetPath makes one named-target path relative to the Git root.
// It does not check if the path exists because deleted paths can affect a
// release.
func normalizeTargetPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("target path must not be empty")
	}
	if strings.Contains(value, `\`) {
		return "", fmt.Errorf("target path %q must use forward slashes", value)
	}
	if filepath.IsAbs(value) || path.IsAbs(value) {
		return "", fmt.Errorf("target path %q must be relative to the Git root", value)
	}

	normalized := path.Clean(value)
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("target path %q must stay in the Git repository", value)
	}
	return normalized, nil
}

func newNamedTarget(target TargetConfig, gitRoot string) (DirectoryVersionInfo, error) {
	group := DirectoryVersionInfo{
		Package:      target.Name,
		FullPath:     gitRoot,
		RootRelative: true,
	}
	if err := validateTargetName(target.Name); err != nil {
		return group, err
	}
	if len(target.Paths) == 0 {
		return group, fmt.Errorf("target %q must have at least one path", target.Name)
	}

	for _, value := range target.Paths {
		normalized, err := normalizeTargetPath(value)
		if err != nil {
			return group, fmt.Errorf("target %q: %w", target.Name, err)
		}
		group.Directories = appendNewPath(group.Directories, normalized)
	}
	return group, nil
}

// ParseDirectoryGroups makes the tag groups for one run. Each --directories
// value is one directory with its own tag. Each --dir_group value is a comma
// separated list of directories that share one tag, so a change to a shared
// directory releases each group that lists it. The first directory of a group
// names the tag. The groups keep their order, with --directories values first.
func ParseDirectoryGroups(
	directories []string,
	dirGroups []string,
	gitRoot string,
) ([]DirectoryVersionInfo, error) {
	gitRootPath, err := filepath.Abs(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("can not resolve the git root %s: %w", gitRoot, err)
	}

	var groups []DirectoryVersionInfo
	// Reject duplicate package names before they create colliding tags.
	groupForPackage := map[string]string{}

	addGroup := func(value string, members []string) error {
		if len(members) == 0 {
			return fmt.Errorf("directory group %q does not name a directory", value)
		}

		group, err := newDirectoryGroup(members, gitRoot, gitRootPath)
		if err != nil {
			return err
		}

		packageName := group.PackageName()
		if previous, found := groupForPackage[packageName]; found {
			return fmt.Errorf(
				"%q and %q both tag the package %q, so give one of them a different first directory",
				previous, value, packageName,
			)
		}
		groupForPackage[packageName] = value

		groups = append(groups, group)
		return nil
	}

	// Keep the legacy behavior: commas in --directories are literal.
	for _, value := range directories {
		if err := addGroup(value, []string{value}); err != nil {
			return nil, err
		}
	}

	for _, value := range dirGroups {
		if err := addGroup(value, splitDirectoryGroup(value)); err != nil {
			return nil, err
		}
	}

	return groups, nil
}

// ParseReleaseTargets makes every release target for one run. Legacy
// directories keep their current order and behavior. Named targets follow
// all legacy targets in their configured order.
func ParseReleaseTargets(
	directories []string,
	dirGroups []string,
	targets []TargetConfig,
	gitRoot string,
) ([]DirectoryVersionInfo, error) {
	groups, err := ParseDirectoryGroups(directories, dirGroups, gitRoot)
	if err != nil {
		return nil, err
	}

	groupForPackage := make(map[string]string, len(groups)+len(targets))
	for _, group := range groups {
		groupForPackage[group.PackageName()] = "a legacy directory or directory group"
	}

	for index, target := range targets {
		group, err := newNamedTarget(target, gitRoot)
		if err != nil {
			return nil, err
		}
		if previous, found := groupForPackage[group.PackageName()]; found {
			return nil, fmt.Errorf(
				"%s and target %q at position %d both tag the package %q",
				previous, target.Name, index+1, group.PackageName(),
			)
		}
		groupForPackage[group.PackageName()] = fmt.Sprintf(
			"target %q at position %d", target.Name, index+1,
		)
		groups = append(groups, group)
	}

	return groups, nil
}
