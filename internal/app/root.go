package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type CLIConfig struct {
	Root string
}

func ParseCLI(args []string, stdout io.Writer, stderr io.Writer) (CLIConfig, error) {
	fs := flag.NewFlagSet("gitws", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stdout, "gitws scans a workspace and opens Git repos in a Bubble Tea TUI.\n\n")
		fmt.Fprintf(stdout, "Usage:\n")
		fmt.Fprintf(stdout, "  gitws [--root PATH]\n")
		fmt.Fprintf(stdout, "  gitws [PATH]\n\n")
		fmt.Fprintf(stdout, "Root resolution order:\n")
		fmt.Fprintf(stdout, "  1. --root PATH\n")
		fmt.Fprintf(stdout, "  2. positional PATH\n")
		fmt.Fprintf(stdout, "  3. GITWS_ROOT\n")
		fmt.Fprintf(stdout, "  4. ~/code\n\n")
		fmt.Fprintf(stdout, "Options:\n")
		fmt.Fprintf(stdout, "  -root string\n")
		fmt.Fprintf(stdout, "        workspace root to scan\n")
	}

	var rootFlag string
	fs.StringVar(&rootFlag, "root", "", "workspace root to scan")

	if err := fs.Parse(args); err != nil {
		return CLIConfig{}, err
	}

	root := rootFlag
	if root == "" && fs.NArg() > 0 {
		root = fs.Arg(0)
	}
	if root == "" {
		root = os.Getenv("GITWS_ROOT")
	}
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return CLIConfig{}, fmt.Errorf("resolve home dir: %w", err)
		}
		root = filepath.Join(home, "code")
	}

	resolved, err := filepath.Abs(root)
	if err != nil {
		return CLIConfig{}, fmt.Errorf("resolve root %q: %w", root, err)
	}

	return CLIConfig{Root: resolved}, nil
}
