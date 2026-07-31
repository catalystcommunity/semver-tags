package core

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DirectoryVersionInfo holds one tag group. Directory is the first directory
// of the group, and it names the tag. Directories holds the git paths of every
// directory in the group, because a commit in any of them changes this tag.
type DirectoryVersionInfo struct {
	Directory    string
	Directories  []string
	FullPath     string
	LastVersion  *VersionInfo
	NextVersion  *VersionInfo
	ReleaseNotes []string
	UseRoot      bool
}

// PackageName gives the package part of the tag for this group. It is the last
// path element of the first directory, and it is empty for whole repo mode.
func (d *DirectoryVersionInfo) PackageName() string {
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
		return d.Directories
	}
	return []string{strings.TrimRight(d.FullPath, "/")}
}

func (d *DirectoryVersionInfo) Printable() string {
	retVal := "DirectoryVersionInfo:\n"
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

// splitDirectoryGroup splits one --dir_group value into its directories.
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
		group.UseRoot = true
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
	// Two groups that make the same tag would collide, so refuse that early.
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

	// A --directories value stays one literal path, the way it always was
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
