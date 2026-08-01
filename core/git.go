package core

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// gitError makes an error that keeps the standard error of a failed git
// command. exec.Cmd.Output gives only the standard output, so without this the
// error tells you nothing about the cause.
func gitError(args []string, err error) error {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		detail := strings.TrimSpace(string(exitError.Stderr))
		if detail != "" {
			return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, detail)
		}
	}
	return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

// runGit runs one git command and gives its standard output.
func runGit(args ...string) (string, error) {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", gitError(args, err)
	}
	return string(output), nil
}

// gitLines runs one git command and gives its output as lines. It removes
// empty lines, because git writes a last newline.
func gitLines(args ...string) ([]string, error) {
	output, err := runGit(args...)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// IsGitRepo tells if the current directory is in a git work tree.
func IsGitRepo() bool {
	output, err := runGit("rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false
	}
	return strings.TrimSpace(output) == "true"
}

// GetGitRootDir gives the top directory of the current git repository.
func GetGitRootDir() (string, error) {
	output, err := runGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// headCommit gives the commit that a new tag points at.
func headCommit() (string, error) {
	output, err := runGit("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// firstCommit gives the first commit that has no parent. It is the start point
// for a package that has no tag yet.
func firstCommit() (string, error) {
	lines, err := gitLines("rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", fmt.Errorf("can not get a parentless commit, so no root to determine: %w", err)
	}
	if len(lines) == 0 {
		return "", errors.New("the repository has no commit, so there is no root to determine")
	}
	return lines[0], nil
}

// repositoryTagLines gives one "tag,commit" line for each tag in the
// repository.
func repositoryTagLines() ([]string, error) {
	lines, err := gitLines(
		"for-each-ref",
		"--format", "%(refname:short),%(objectname)",
		"refs/tags",
	)
	if err != nil {
		return nil, fmt.Errorf("can not get git tags: %w", err)
	}
	return lines, nil
}

type commitMessage struct {
	Subject string
	Message string
}

// commitMessages gives the subject and full message of each commit after the
// given commit that changed one of the given paths.
func commitMessages(afterCommit string, paths []string) ([]commitMessage, error) {
	args := []string{"log", "-z", "--pretty=format:%s%x00%B", fmt.Sprintf("%s..HEAD", afterCommit), "--"}
	args = append(args, paths...)
	output, err := runGit(args...)
	if err != nil {
		return nil, fmt.Errorf("can not get git commits: %w", err)
	}

	parts := strings.Split(output, "\x00")
	commits := make([]commitMessage, 0, len(parts)/2)
	for index := 0; index+1 < len(parts); index += 2 {
		subject := strings.TrimSuffix(parts[index], "\n")
		message := strings.TrimSuffix(parts[index+1], "\n")
		if subject == "" && message == "" {
			continue
		}
		commits = append(commits, commitMessage{Subject: subject, Message: message})
	}
	return commits, nil
}

// createTag makes one local tag at HEAD.
func createTag(tag string) error {
	if _, err := runGit("tag", tag); err != nil {
		return fmt.Errorf("error tagging: %w", err)
	}
	return nil
}

// updateTag makes a local tag at HEAD or moves an existing local tag to HEAD.
func updateTag(tag string) error {
	if _, err := runGit("tag", "--force", tag); err != nil {
		return fmt.Errorf("error updating tag: %w", err)
	}
	return nil
}

// pushTags sends the new tags to the remote. An empty branch pushes only the
// tags, which is what a job that checked out a commit instead of a branch
// needs. The atomic option makes the remote take all of the tags or none.
func pushTags(
	remote string,
	branch string,
	atomic bool,
	tags []string,
	forceTags []string,
) error {
	args := []string{"push"}
	if atomic {
		args = append(args, "--atomic")
	}
	args = append(args, remote)
	if branch != "" {
		args = append(args, branch)
	}
	args = append(args, tags...)
	for _, tag := range forceTags {
		args = append(args, "+refs/tags/"+tag+":refs/tags/"+tag)
	}

	if _, err := runGit(args...); err != nil {
		return fmt.Errorf("error pushing tags: %w", err)
	}
	return nil
}
