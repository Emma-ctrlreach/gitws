# gitws

`gitws` is a small Bubble Tea TUI to scan a workspace root, list Git repositories, and jump into `lazygit` for the selected repo.

## Features

- scans `~/code` by default
- supports overriding the root with `GITWS_ROOT`
- lists directories containing a `.git`, including nested repos and submodules under the scan root
- shows branch name, dirty/clean state, modified file count, and ahead/behind
- filter repos with `/`
- toggle `dirty only` with `d`
- refresh the scan with `r`
- open `lazygit` with `enter` or `l`
- open `opencode` with `o`
- show a detail panel for the selected repo
- preview code diff in the detail panel
- preview `.claude/features/JOURNAL_<slug>.md` in the detail panel
- open `.claude/features/JOURNAL_<slug>.md` with `J`
- uses globally installed `lazygit` and `opencode` from `PATH`
- can be used globally from any current working directory
- supports `--root /path` or `gitws /path` for ad-hoc scans
- inside tmux, can open tools in popup or split mode
- inside tmux, can open tools in `popup`, `split`, or `window` mode
- supports tmux sizing/position config via environment variables
- persists tmux settings in `~/.config/gitws/config.json`

## Installation locale

Option simple:

```bash
./install.sh
```

Ce script:
- lance `go install ./cmd/gitws`
- détecte le dossier de binaires Go
- t’indique quoi ajouter à `~/.zshrc` si ce dossier n’est pas dans le `PATH`
- rappelle les commandes d’utilisation

Désinstallation:

```bash
./uninstall.sh
```

Option manuelle:

```bash
make install
```

`make install` fait simplement:

```bash
go install ./cmd/gitws
```

Si le binaire n’est pas trouvé ensuite, ajoute le dossier Go bin à ton `PATH`.
En général:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Puis recharge ton shell:

```bash
source ~/.zshrc
```

## Utilisation

Une fois installé, `gitws` fonctionne depuis n’importe quel dossier.

Cas par défaut:

```bash
gitws
```

Dans ce mode, la racine scannée est résolue dans cet ordre:
1. `--root /chemin`
2. argument positionnel `/chemin`
3. variable `GITWS_ROOT`
4. défaut `~/code`

Exemples:

```bash
gitws --help
gitws --root /path/to/code
gitws /path/to/code
GITWS_ROOT=/path/to/code gitws
```

Au démarrage, `gitws` signale immédiatement si une dépendance manque dans le `PATH`.
Aujourd’hui, les checks portent sur:
- `lazygit`
- `opencode`
- `osascript` pour l’ouverture OpenCode sur macOS

## Développement

```bash
go mod tidy
go run ./cmd/gitws
```

Project commands:

```bash
make fmt
make test
make build
make install
```

## Controls

- `j` or arrow down: move selection down
- `k` or arrow up: move selection up
- `/`: start filtering
- `enter` or `l`: open `lazygit` in selected repo
- `o`: open `opencode` for selected repo
- `J`: open `.claude/Journal-feature.md` for selected repo
- `s`: open/close Settings panel
- `p`: toggle tmux mode between `popup`, `split`, and `window`
- `d`: toggle dirty-only mode
- `r`: refresh repositories
- `q`: quit

## Tmux

When `gitws` runs inside tmux:
- `enter` / `l` opens `lazygit` using tmux
- `o` opens `opencode` using tmux
- `J` opens the journal in a terminal editor using `${EDITOR:-vi}`
- `p` toggles the current tmux integration mode between `popup`, `split`, and `window`
- the selected tmux mode is persisted in `~/.config/gitws/config.json`
- `s` opens a Settings panel to edit tmux options directly from the TUI

Environment variables:

```bash
GITWS_TMUX_MODE=split
GITWS_TMUX_POPUP_WIDTH=90%
GITWS_TMUX_POPUP_HEIGHT=90%
GITWS_TMUX_POPUP_X=C
GITWS_TMUX_POPUP_Y=C
GITWS_TMUX_SPLIT_DIRECTION=right
GITWS_TMUX_SPLIT_SIZE=50%
```

Supported values:
- `GITWS_TMUX_MODE`: `popup`, `split`, or `window`
- `GITWS_TMUX_SPLIT_DIRECTION`: `right` or `down`
- popup width/height/position values are passed directly to tmux

Priority for tmux mode:
1. `GITWS_TMUX_MODE`
2. persisted value in `~/.config/gitws/config.json`
3. default `split`

Journal path convention:

```bash
$HOME/code/<repo>/.claude/features/JOURNAL_<slug>.md
```

Current implementation resolves it from the selected repo path plus the current branch slug.
Example:

```text
feat/bm-yt -> JOURNAL_bm-yt.md
```

The computed slug is displayed in the detail panel.
If the primary journal path does not exist, `gitws` tries fallbacks for non-feature branches:
- suffix slug after the first branch segment
- full normalized branch slug
- last branch segment slug
- if exactly one `JOURNAL_*.md` exists in `.claude/features`, it is used as a final fallback

## Notes

- repositories are sorted with dirty repos first, then by relative path
- detached HEAD is displayed as `detached`
- repos that fail `git status` are skipped during scanning
- the detail panel switches between side-by-side and stacked layout depending on terminal width
- `J` shows an error if the computed journal file does not exist in the selected repo
- the detail panel shows the computed journal slug explicitly
- `lazygit` and `opencode` must be installed globally and available in `PATH`
- inside tmux, `enter`/`l` opens lazygit using the current tmux mode (`popup`, `split`, or `window`)
- outside tmux, `enter`/`l` falls back to opening lazygit in the current terminal session
- inside tmux, `o` uses the current tmux mode; outside tmux it falls back to macOS Terminal via `osascript`
- inside tmux, `J` uses the current tmux mode and `${EDITOR:-vi}`
- Settings panel supports inline edit/reset of tmux config with persistence in `~/.config/gitws/config.json`
- diff preview is computed during scan from staged and unstaged `git diff --no-color`
- `gitws --help` affiche l’aide CLI et l’ordre de résolution de la racine
