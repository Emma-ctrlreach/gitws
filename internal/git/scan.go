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
	DiffStats     string
	DiffEntries   []DiffStatEntry
}

type DiffStatEntry struct {
	Section  string
	Added    string
	Deleted  string
	Path     string
	OpenPath string
	Line     int
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
	status.DiffStats, status.DiffEntries = diffStats(repoPath)

	if err := scanner.Err(); err != nil {
		return RepoStatus{}, fmt.Errorf("scan git output for %q: %w", repoPath, err)
	}

	return status, nil
}

func diffStats(repoPath string) (string, []DiffStatEntry) {
	parts := make([]string, 0, 2)
	entries := make([]DiffStatEntry, 0, 16)

	if out := runGitText(repoPath, "diff", "--numstat", "--cached"); out != "" {
		parts = append(parts, "staged\n"+out)
		entries = append(entries, parseDiffStatEntries("staged", out, parseDiffLineHints(repoPath, "--cached"))...)
	}
	if out := runGitText(repoPath, "diff", "--numstat"); out != "" {
		parts = append(parts, "unstaged\n"+out)
		entries = append(entries, parseDiffStatEntries("unstaged", out, parseDiffLineHints(repoPath))...)
	}

	if len(parts) == 0 {
		return "No diff stats available; working tree may be clean or contain only untracked files", nil
	}

	return strings.Join(parts, "\n\n"), entries
}

func parseDiffStatEntries(section string, text string, lineHints map[string]int) []DiffStatEntry {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	entries := make([]DiffStatEntry, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		entries = append(entries, DiffStatEntry{
			Section:  section,
			Added:    parts[0],
			Deleted:  parts[1],
			Path:     parts[2],
			OpenPath: resolveDiffOpenPath(parts[2]),
			Line:     lineHints[parts[2]],
		})
	}
	return entries
}

func resolveDiffOpenPath(path string) string {
	if strings.Contains(path, " => ") {
		left, right, found := strings.Cut(path, " => ")
		if !found {
			return path
		}
		leftBrace := strings.LastIndex(left, "{")
		rightBrace := strings.Index(right, "}")
		if leftBrace >= 0 && rightBrace >= 0 {
			prefix := left[:leftBrace]
			suffix := right[rightBrace+1:]
			middle := right[:rightBrace]
			return prefix + middle + suffix
		}
		return right
	}
	return path
}

func parseDiffLineHints(repoPath string, args ...string) map[string]int {
	cmdArgs := append([]string{"diff", "--no-color", "--unified=0"}, args...)
	out := runGitText(repoPath, cmdArgs...)
	hints := map[string]int{}
	if out == "" {
		return hints
	}
	currentPath := ""
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			currentPath = strings.TrimPrefix(line, "+++ b/")
		case strings.HasPrefix(line, "+++ "):
			currentPath = strings.TrimPrefix(line, "+++ ")
		case strings.HasPrefix(line, "@@") && currentPath != "":
			if _, exists := hints[currentPath]; exists {
				continue
			}
			var oldStart, oldCount, newStart, newCount int
			if _, err := fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oldStart, &oldCount, &newStart, &newCount); err == nil {
				if newStart > 0 {
					hints[currentPath] = newStart
				} else if oldStart > 0 {
					hints[currentPath] = oldStart
				}
				continue
			}
			if _, err := fmt.Sscanf(line, "@@ -%d +%d @@", &oldStart, &newStart); err == nil {
				if newStart > 0 {
					hints[currentPath] = newStart
				} else if oldStart > 0 {
					hints[currentPath] = oldStart
				}
			}
		}
	}
	return hints
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
