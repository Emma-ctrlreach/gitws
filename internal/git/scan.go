package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RepoStatus struct {
	Name          string
	Path          string
	RelPath       string
	Branch        string
	Dirty         bool
	ModifiedCount int
	Ahead         int
	Behind        int
	DiffPreview   string
}

func Scan(root string) ([]RepoStatus, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", root)
	}

	repos := make([]RepoStatus, 0, 32)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}

		gitPath := filepath.Join(path, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			status, err := inspectRepo(root, path)
			if err == nil {
				repos = append(repos, status)
			}
			// Keep walking so a repo at the scan root does not hide nested repos or submodules.
			return nil
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return repos, nil
}

func inspectRepo(root, repoPath string) (RepoStatus, error) {
	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain=v2", "--branch")
	out, err := cmd.Output()
	if err != nil {
		return RepoStatus{}, fmt.Errorf("git status for %q: %w", repoPath, err)
	}

	relPath, err := filepath.Rel(root, repoPath)
	if err != nil {
		relPath = repoPath
	}

	status := RepoStatus{
		Name:    filepath.Base(repoPath),
		Path:    repoPath,
		RelPath: relPath,
		Branch:  "detached",
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			status.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			var ahead, behind int
			fmt.Sscanf(strings.TrimPrefix(line, "# branch.ab "), "+%d -%d", &ahead, &behind)
			status.Ahead = ahead
			status.Behind = behind
		case strings.HasPrefix(line, "#"):
			continue
		case strings.TrimSpace(line) != "":
			status.ModifiedCount++
		}
	}
	status.Dirty = status.ModifiedCount > 0
	status.DiffPreview = diffPreview(repoPath)

	if err := scanner.Err(); err != nil {
		return RepoStatus{}, fmt.Errorf("scan git output for %q: %w", repoPath, err)
	}

	return status, nil
}

func diffPreview(repoPath string) string {
	parts := make([]string, 0, 2)

	if out := runGitText(repoPath, "diff", "--no-color", "--cached"); out != "" {
		parts = append(parts, "staged\n"+out)
	}
	if out := runGitText(repoPath, "diff", "--no-color"); out != "" {
		parts = append(parts, "unstaged\n"+out)
	}

	if len(parts) == 0 {
		return "No patch preview available; working tree may be clean or contain only untracked files"
	}

	return strings.Join(parts, "\n\n")
}

func runGitText(repoPath string, args ...string) string {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
