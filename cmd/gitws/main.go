package main

import (
	"errors"
	"flag"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/emma/gitws/internal/app"
)

func main() {
	cli, err := app.ParseCLI(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatal(err)
	}

	tmux := app.LoadTmuxConfigForModel()
	warnings := app.DependencyWarnings(tmux)
	m := app.NewModel(cli.Root, warnings, tmux)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
