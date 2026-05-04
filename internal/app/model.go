package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emma/gitws/internal/git"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cleanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	dirtyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	branchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
	labelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	settingStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60"))
	focusStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(0, 1)
)

type scanResultMsg struct {
	repos []git.RepoStatus
	err   error
}

type externalCommandFinishedMsg struct {
	err error
}

type model struct {
	root              string
	warnings          []string
	tmux              tmuxConfig
	repos             []git.RepoStatus
	filtered          []git.RepoStatus
	selected          int
	focus             string
	scrollMemory      map[string]panelScrollState
	width             int
	height            int
	settingsOpen      bool
	settingsIndex     int
	descriptionScroll int
	journalScroll     int
	diffScroll        int
	loading           bool
	dirtyOnly         bool
	filtering         bool
	filter            textinput.Model
	err               error
}

type panelScrollState struct {
	Description int
	Journal     int
	Diff        int
	Focus       string
}

type tmuxConfig struct {
	Active         bool
	Mode           string
	PopupWidth     string
	PopupHeight    string
	PopupX         string
	PopupY         string
	SplitDirection string
	SplitSize      string
}

type settingsItem struct {
	label string
	value string
}

type journalResolution struct {
	Path         string
	ComputedSlug string
	ResolvedSlug string
	FileName     string
	Source       string
}

func NewModel(root string, warnings []string, tmux tmuxConfig) model {
	input := textinput.New()
	input.Placeholder = "Filter repos"
	input.CharLimit = 256
	input.Prompt = "/ "
	input.Width = 40

	return model{
		root:         root,
		warnings:     warnings,
		tmux:         tmux,
		focus:        "list",
		scrollMemory: map[string]panelScrollState{},
		loading:      true,
		filter:       input,
	}
}

func (m model) Init() tea.Cmd {
	return scanReposCmd(m.root)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.filtering {
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()

		switch key := msg.(type) {
		case tea.KeyMsg:
			switch key.String() {
			case "esc":
				m.filtering = false
				m.filter.Blur()
				return m, nil
			case "enter":
				m.filtering = false
				m.filter.Blur()
				return m, nil
			}
		}

		return m, cmd
	}

	if m.settingsOpen {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			return m, nil
		case tea.KeyMsg:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			if msg.String() == "shift+tab" {
				m.cycleFocus(-1)
				return m, nil
			}
			if msg.String() == "tab" {
				m.cycleFocus(1)
				return m, nil
			}
			if err := m.handleSettingsKey(msg.String()); err != nil {
				m.err = err
			}
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case scanResultMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.repos = msg.repos
			m.applyFilter()
		}
		return m, nil
	case externalCommandFinishedMsg:
		m.err = msg.err
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			m.loading = true
			m.err = nil
			return m, scanReposCmd(m.root)
		case "d":
			m.dirtyOnly = !m.dirtyOnly
			m.applyFilter()
			return m, nil
		case "s":
			m.settingsOpen = !m.settingsOpen
			if m.settingsOpen {
				m.focus = "settings"
			} else if m.focus == "settings" {
				m.focus = "list"
			}
			return m, nil
		case "tab":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil
		case "p":
			if m.tmux.Active {
				m.tmux.toggleMode()
				if err := m.persistTmuxConfig(); err != nil {
					m.err = fmt.Errorf("persist tmux mode: %w", err)
					return m, nil
				}
			}
			return m, nil
		case "/":
			m.filtering = true
			m.filter.Focus()
			return m, textinput.Blink
		case "up", "k":
			if m.focus == "list" {
				m.setSelected(m.selected - 1)
			} else {
				m.scrollFocused(-1)
			}
			return m, nil
		case "down", "j":
			if m.focus == "list" {
				m.setSelected(m.selected + 1)
			} else {
				m.scrollFocused(1)
			}
			return m, nil
		case "pgup", "ctrl+u":
			m.scrollFocused(-5)
			return m, nil
		case "pgdown", "ctrl+d":
			m.scrollFocused(5)
			return m, nil
		case "o":
			if len(m.filtered) == 0 {
				return m, nil
			}
			cmd, err := openOpenCodeCmd(m.filtered[m.selected].Path, m.tmux)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.err = nil
			return m, runExternalCmd(cmd)
		case "J":
			if len(m.filtered) == 0 {
				return m, nil
			}
			journalPath := journalPathForRepo(m.filtered[m.selected])
			if _, err := os.Stat(journalPath); err != nil {
				m.err = fmt.Errorf("journal not found: %s", journalPath)
				return m, nil
			}
			cmd, err := openJournalCmd(m.filtered[m.selected].Path, journalPath, m.tmux)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.err = nil
			return m, runExternalCmd(cmd)
		case "enter", "l":
			if len(m.filtered) == 0 {
				return m, nil
			}
			cmd, err := openLazygitCmd(m.filtered[m.selected].Path, m.tmux)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.err = nil
			return m, runExternalCmd(cmd)
		}
	}

	return m, nil
}

func (m *model) applyFilter() {
	currentPath := m.selectedRepoPath()
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	filtered := make([]git.RepoStatus, 0, len(m.repos))

	for _, repo := range m.repos {
		if m.dirtyOnly && !repo.Dirty {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(repo.Name + " " + repo.RelPath + " " + repo.Branch)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, repo)
	}

	m.filtered = filtered
	if len(m.filtered) == 0 {
		m.selected = 0
		m.resetPanelScrolls()
		return
	}
	if currentPath != "" {
		for i, repo := range m.filtered {
			if repo.Path == currentPath {
				m.selected = i
				m.restoreScrollsForSelection()
				return
			}
		}
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	m.restoreScrollsForSelection()
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("gitws"))
	b.WriteString("  ")
	b.WriteString(mutedStyle.Render(m.root))
	b.WriteString("\n")
	b.WriteString(m.statusLine())
	b.WriteString("\n\n")

	if len(m.warnings) > 0 {
		for _, warning := range m.warnings {
			b.WriteString(dirtyStyle.Render("warning: "))
			b.WriteString(mutedStyle.Render(warning))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.filtering {
		b.WriteString(m.filter.View())
		b.WriteString("\n\n")
	} else if value := strings.TrimSpace(m.filter.Value()); value != "" {
		b.WriteString(mutedStyle.Render("filter: " + value))
		b.WriteString("\n\n")
	}

	if m.loading {
		b.WriteString("Scanning repositories...")
		b.WriteString("\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(errorStyle.Render(m.err.Error()))
		b.WriteString("\n\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(mutedStyle.Render("No repositories found for current filter."))
		b.WriteString("\n")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(m.helpLine()))
		return b.String()
	}

	contentHeight := m.height - lipgloss.Height(b.String()) - 2
	if contentHeight < 6 {
		contentHeight = 6
	}
	b.WriteString(m.renderContent(contentHeight))

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.helpLine()))
	return b.String()
}

func (m model) statusLine() string {
	parts := []string{fmt.Sprintf("%d repos", len(m.repos))}
	if m.tmux.Active {
		parts = append(parts, fmt.Sprintf("tmux:%s", m.tmux.Mode))
	}
	if m.dirtyOnly {
		parts = append(parts, dirtyStyle.Render("dirty-only"))
	}
	if len(m.filtered) != len(m.repos) {
		parts = append(parts, fmt.Sprintf("%d shown", len(m.filtered)))
	}
	return strings.Join(parts, "  ")
}

func renderRepoLine(repo git.RepoStatus, selected bool, width int) string {
	state := cleanStyle.Render("clean")
	if repo.Dirty {
		state = dirtyStyle.Render(fmt.Sprintf("dirty:%d", repo.ModifiedCount))
	}

	aheadBehind := ""
	if repo.Ahead != 0 || repo.Behind != 0 {
		aheadBehind = mutedStyle.Render(fmt.Sprintf("  +%d/-%d", repo.Ahead, repo.Behind))
	}

	line := fmt.Sprintf("%-24s %s  %s%s  %s",
		truncateRight(repo.Name, 24),
		branchStyle.Render(truncateRight(repo.Branch, 18)),
		state,
		aheadBehind,
		mutedStyle.Render(repo.RelPath),
	)

	if width > 0 {
		line = lipgloss.NewStyle().MaxWidth(width - 1).Render(line)
	}
	if selected {
		return selectedStyle.Width(max(0, width-1)).Render(line)
	}
	return line
}

func (m model) renderContent(height int) string {
	if m.width >= 190 {
		listWidth := max(34, m.width/4)
		descriptionWidth := max(26, (m.width-listWidth)/3)
		journalWidth := max(26, (m.width-listWidth-descriptionWidth)/2)
		diffWidth := max(26, m.width-listWidth-descriptionWidth-journalWidth-3)
		descriptionPanel := m.renderDescriptionPanel(descriptionWidth, height)
		if m.settingsOpen {
			descriptionPanel = m.renderSettingsPanel(descriptionWidth, height)
		}
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderListPanel(listWidth, height),
			descriptionPanel,
			m.renderJournalPanel(journalWidth, height),
			m.renderDiffPanel(diffWidth, height),
		)
	}
	if m.width >= 120 {
		listWidth := max(34, m.width/3)
		rightWidth := max(30, m.width-listWidth-1)
		descriptionHeight := max(6, height/3)
		journalHeight := max(6, (height-descriptionHeight)/2)
		diffHeight := max(6, height-descriptionHeight-journalHeight)
		descriptionPanel := m.renderDescriptionPanel(rightWidth, descriptionHeight)
		if m.settingsOpen {
			descriptionPanel = m.renderSettingsPanel(rightWidth, descriptionHeight)
		}
		rightColumn := lipgloss.JoinVertical(
			lipgloss.Left,
			descriptionPanel,
			m.renderJournalPanel(rightWidth, journalHeight),
			m.renderDiffPanel(rightWidth, diffHeight),
		)
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.renderListPanel(listWidth, height),
			rightColumn,
		)
	}

	listHeight := max(5, height/3)
	descriptionHeight := max(6, (height-listHeight)/3)
	journalHeight := max(6, (height-listHeight-descriptionHeight)/2)
	diffHeight := max(6, height-listHeight-descriptionHeight-journalHeight)
	descriptionPanel := m.renderDescriptionPanel(m.width-1, descriptionHeight)
	if m.settingsOpen {
		descriptionPanel = m.renderSettingsPanel(m.width-1, descriptionHeight)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderListPanel(m.width-1, listHeight),
		descriptionPanel,
		m.renderJournalPanel(m.width-1, journalHeight),
		m.renderDiffPanel(m.width-1, diffHeight),
	)
}

func (m model) renderListPanel(width, height int) string {
	if width < 8 {
		width = 8
	}
	innerWidth := max(1, width-4)
	innerHeight := max(1, height-4)

	start := 0
	if m.selected >= innerHeight {
		start = m.selected - innerHeight + 1
	}
	end := min(len(m.filtered), start+innerHeight)

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, renderRepoLine(m.filtered[i], i == m.selected, innerWidth))
	}
	if len(lines) == 0 {
		lines = append(lines, mutedStyle.Render("No repositories"))
	}

	body := append([]string{titleForPanel("list", m.focus == "list"), ""}, lines...)
	return panelForFocus(m.focus == "list").Width(width).Height(height).Render(strings.Join(body, "\n"))
}

func (m model) renderDescriptionPanel(width, height int) string {
	if width < 8 {
		width = 8
	}
	repo := m.selectedRepo()
	if repo == nil {
		return panelStyle.Width(width).Height(height).Render(mutedStyle.Render("No selection"))
	}

	stateText := "clean"
	if repo.Dirty {
		stateText = fmt.Sprintf("dirty (%d files)", repo.ModifiedCount)
	}

	journal := resolveJournalPathForRepo(*repo)
	journalState := "missing"
	journalSource := journal.Source
	if _, err := os.Stat(journal.Path); err == nil {
		journalState = "present"
	}

	lines := []string{
		repo.Name,
		fmt.Sprintf("Branch: %s", repo.Branch),
		fmt.Sprintf("Status: %s", stateText),
		fmt.Sprintf("Ahead/Behind: +%d/-%d", repo.Ahead, repo.Behind),
		fmt.Sprintf("Path: %s", repo.Path),
		fmt.Sprintf("Relative: %s", repo.RelPath),
		fmt.Sprintf("Journal: %s", journalState),
		fmt.Sprintf("Journal source: %s", journalSource),
		fmt.Sprintf("Journal slug: %s", journal.ComputedSlug),
		fmt.Sprintf("Resolved slug: %s", journal.ResolvedSlug),
		fmt.Sprintf("Journal file: %s", journal.FileName),
		fmt.Sprintf("Journal path: %s", journal.Path),
		"",
		"Actions",
		"enter/l  open lazygit",
		"o        open opencode",
		"J        open resolved feature journal",
		"p        cycle tmux mode",
	}
	return m.renderScrollablePanel("description", "Description", width, height, lines, m.descriptionScroll)
}

func (m model) renderJournalPanel(width, height int) string {
	if width < 8 {
		width = 8
	}
	repo := m.selectedRepo()
	if repo == nil {
		return panelStyle.Width(width).Height(height).Render(mutedStyle.Render("No selection"))
	}

	journal := resolveJournalPathForRepo(*repo)
	journalState := "missing"
	journalPreview := mutedStyle.Render("Journal preview unavailable")
	journalSource := journal.Source
	if _, err := os.Stat(journal.Path); err == nil {
		journalState = "present"
		journalPreview = readJournalPreview(journal.Path, max(8, height-10), max(20, width-6))
	}

	lines := []string{
		fmt.Sprintf("State: %s", journalState),
		fmt.Sprintf("Source: %s", journalSource),
		fmt.Sprintf("Slug: %s", journal.ComputedSlug),
		fmt.Sprintf("Resolved: %s", journal.ResolvedSlug),
		fmt.Sprintf("File: %s", journal.FileName),
		"",
		journalPreview,
	}
	return m.renderScrollablePanel("journal", "Journal", width, height, lines, m.journalScroll)
}

func (m model) renderDiffPanel(width int, height int) string {
	if width < 8 {
		width = 8
	}
	repo := m.selectedRepo()
	if repo == nil {
		return panelForFocus(m.focus == "diff").Width(width).Height(height).Render(strings.Join([]string{titleForPanel("diff", m.focus == "diff"), "", mutedStyle.Render("No selection")}, "\n"))
	}
	lines := strings.Split(repo.DiffPreview, "\n")
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("No diff preview available")}
	}
	return m.renderScrollablePanel("diff", "Diff", width, height, lines, m.diffScroll)
}

func (m model) renderSettingsPanel(width int, height int) string {
	if width < 8 {
		width = 8
	}
	items := m.settingsItems()
	lines := make([]string, 0, len(items)+6)
	lines = append(lines, titleStyle.Render("Tmux Settings"), "")
	for i, item := range items {
		line := fmt.Sprintf("%-16s %s", item.label, item.value)
		if i == m.settingsIndex {
			line = settingStyle.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	lines = append(lines, mutedStyle.Render("s/esc close  j/k move  h/l change  R reset defaults"))
	if path, err := configFilePath(); err == nil {
		lines = append(lines, mutedStyle.Render(path))
	}
	body := strings.Join(append([]string{titleForPanel("settings", m.focus == "settings"), ""}, lines...), "\n")
	return panelForFocus(m.focus == "settings").Width(width).Height(height).Render(body)
}

func (m model) selectedRepo() *git.RepoStatus {
	if len(m.filtered) == 0 || m.selected < 0 || m.selected >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.selected]
}

func (m model) selectedRepoPath() string {
	repo := m.selectedRepo()
	if repo == nil {
		return ""
	}
	return repo.Path
}

func (m model) helpLine() string {
	parts := []string{"tab focus", "j/k move-or-scroll", "enter/l lazygit", "o opencode", "J journal", "/ filter", "d dirty-only"}
	parts = append(parts, "s settings")
	if m.tmux.Active {
		parts = append(parts, "p tmux-mode")
	}
	parts = append(parts, "r refresh", "q quit")
	return strings.Join(parts, "  ")
}

func (m *model) cycleFocus(delta int) {
	panels := m.visiblePanels()
	if len(panels) == 0 {
		m.focus = "list"
		return
	}
	index := 0
	for i, panel := range panels {
		if panel == m.focus {
			index = i
			break
		}
	}
	index = (index + delta + len(panels)) % len(panels)
	m.focus = panels[index]
}

func (m *model) setSelected(index int) {
	if len(m.filtered) == 0 {
		m.selected = 0
		m.focus = "list"
		m.resetPanelScrolls()
		return
	}
	index = max(0, min(index, len(m.filtered)-1))
	if index == m.selected {
		return
	}
	m.saveScrollsForSelection()
	m.selected = index
	m.restoreScrollsForSelection()
}

func (m model) visiblePanels() []string {
	panels := []string{"list"}
	if m.settingsOpen {
		panels = append(panels, "settings")
	} else {
		panels = append(panels, "description")
	}
	panels = append(panels, "journal", "diff")
	return panels
}

func (m *model) scrollFocused(delta int) {
	if delta == 0 {
		return
	}
	switch m.focus {
	case "description":
		m.descriptionScroll = max(0, m.descriptionScroll+delta)
	case "journal":
		m.journalScroll = max(0, m.journalScroll+delta)
	case "diff":
		m.diffScroll = max(0, m.diffScroll+delta)
	}
}

func (m *model) saveScrollsForSelection() {
	path := m.selectedRepoPath()
	if path == "" {
		return
	}
	m.scrollMemory[path] = panelScrollState{
		Description: m.descriptionScroll,
		Journal:     m.journalScroll,
		Diff:        m.diffScroll,
		Focus:       m.focusForMemory(),
	}
}

func (m *model) restoreScrollsForSelection() {
	path := m.selectedRepoPath()
	if path == "" {
		m.resetPanelScrolls()
		return
	}
	state, ok := m.scrollMemory[path]
	if !ok {
		m.focus = "list"
		m.resetPanelScrolls()
		return
	}
	m.descriptionScroll = max(0, state.Description)
	m.journalScroll = max(0, state.Journal)
	m.diffScroll = max(0, state.Diff)
	if m.isVisiblePanel(state.Focus) {
		m.focus = state.Focus
	} else {
		m.focus = "list"
	}
}

func (m *model) resetPanelScrolls() {
	m.descriptionScroll = 0
	m.journalScroll = 0
	m.diffScroll = 0
}

func (m model) focusForMemory() string {
	if m.focus == "settings" {
		return "description"
	}
	if m.focus == "" {
		return "list"
	}
	return m.focus
}

func (m model) isVisiblePanel(panel string) bool {
	for _, visible := range m.visiblePanels() {
		if visible == panel {
			return true
		}
	}
	return false
}

func (m model) renderScrollablePanel(panelID string, title string, width int, height int, lines []string, offset int) string {
	if width < 8 {
		width = 8
	}
	innerWidth := max(3, width-4)
	innerHeight := max(1, height-4)
	contentWidth := max(1, innerWidth-2)
	wrapped := wrapPanelLines(lines, contentWidth)
	start := min(max(0, offset), max(0, len(wrapped)-innerHeight))
	end := min(len(wrapped), start+innerHeight)
	visible := make([]string, 0, end-start+2)
	for _, line := range wrapped[start:end] {
		visible = append(visible, line)
	}
	if len(visible) == 0 {
		visible = append(visible, mutedStyle.Render("No content"))
	}
	visible = addScrollbar(visible, contentWidth, innerHeight, start, len(wrapped))
	body := append([]string{titleForPanel(title, m.focus == panelID), ""}, visible...)
	return panelForFocus(m.focus == panelID).Width(width).Height(height).Render(strings.Join(body, "\n"))
}

func addScrollbar(lines []string, contentWidth int, viewportHeight int, start int, total int) []string {
	if viewportHeight <= 0 {
		return lines
	}
	out := make([]string, 0, max(len(lines), viewportHeight))
	thumbStart, thumbEnd := scrollbarThumb(viewportHeight, start, total)
	for i := 0; i < viewportHeight; i++ {
		content := ""
		if i < len(lines) {
			content = padRight(lines[i], contentWidth)
		} else {
			content = strings.Repeat(" ", contentWidth)
		}
		bar := mutedStyle.Render("│")
		if i >= thumbStart && i < thumbEnd {
			bar = titleStyle.Render("█")
		}
		out = append(out, content+" "+bar)
	}
	return out
}

func scrollbarThumb(viewportHeight int, start int, total int) (int, int) {
	if total <= 0 || viewportHeight <= 0 {
		return 0, 0
	}
	if total <= viewportHeight {
		return 0, viewportHeight
	}
	thumbSize := max(1, viewportHeight*viewportHeight/total)
	maxStart := max(1, total-viewportHeight)
	thumbStart := (viewportHeight - thumbSize) * start / maxStart
	return thumbStart, min(viewportHeight, thumbStart+thumbSize)
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func wrapPanelLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped := wrapLine(line, width)
		out = append(out, wrapped...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	parts := make([]string, 0, 1)
	runes := []rune(line)
	for len(runes) > 0 {
		if len(runes) <= width {
			parts = append(parts, string(runes))
			break
		}
		split := width
		for i := width; i > 0; i-- {
			if runes[i-1] == ' ' || runes[i-1] == '\t' {
				split = i
				break
			}
		}
		segment := strings.TrimRight(string(runes[:split]), " \t")
		if segment == "" {
			segment = string(runes[:width])
			split = width
		}
		parts = append(parts, segment)
		runes = []rune(strings.TrimLeft(string(runes[split:]), " \t"))
	}
	return parts
}

func panelForFocus(focused bool) lipgloss.Style {
	if focused {
		return focusStyle
	}
	return panelStyle
}

func titleForPanel(title string, focused bool) string {
	if focused {
		return titleStyle.Render(title + " *")
	}
	return titleStyle.Render(title)
}

func (m model) settingsItems() []settingsItem {
	return []settingsItem{
		{label: "mode", value: m.tmux.Mode},
		{label: "split dir", value: m.tmux.SplitDirection},
		{label: "split size", value: m.tmux.SplitSize},
		{label: "popup width", value: displayConfigValue(m.tmux.PopupWidth)},
		{label: "popup height", value: displayConfigValue(m.tmux.PopupHeight)},
		{label: "popup x", value: displayConfigValue(m.tmux.PopupX)},
		{label: "popup y", value: displayConfigValue(m.tmux.PopupY)},
	}
}

func (m *model) handleSettingsKey(key string) error {
	switch key {
	case "esc", "s":
		m.settingsOpen = false
		return nil
	case "up", "k":
		if m.settingsIndex > 0 {
			m.settingsIndex--
		}
		return nil
	case "down", "j":
		if m.settingsIndex < len(m.settingsItems())-1 {
			m.settingsIndex++
		}
		return nil
	case "left", "h":
		m.adjustSetting(-1)
		return m.persistTmuxConfig()
	case "right", "l", "enter":
		m.adjustSetting(1)
		return m.persistTmuxConfig()
	case "R":
		m.tmux = defaultTmuxConfig(m.tmux.Active)
		return resetPersistedConfig()
	}
	return nil
}

func (m *model) adjustSetting(delta int) {
	switch m.settingsIndex {
	case 0:
		m.tmux.Mode = cycleString([]string{"split", "popup", "window"}, m.tmux.Mode, delta)
	case 1:
		m.tmux.SplitDirection = cycleString([]string{"right", "down"}, m.tmux.SplitDirection, delta)
	case 2:
		m.tmux.SplitSize = cycleString([]string{"30%", "40%", "50%", "60%", "70%"}, m.tmux.SplitSize, delta)
	case 3:
		m.tmux.PopupWidth = cycleString([]string{"70%", "80%", "90%", "95%", "100%"}, m.tmux.PopupWidth, delta)
	case 4:
		m.tmux.PopupHeight = cycleString([]string{"70%", "80%", "90%", "95%", "100%"}, m.tmux.PopupHeight, delta)
	case 5:
		m.tmux.PopupX = cycleString([]string{"", "C", "0", "10", "20"}, m.tmux.PopupX, delta)
	case 6:
		m.tmux.PopupY = cycleString([]string{"", "C", "0", "5", "10"}, m.tmux.PopupY, delta)
	}
}

func (m model) persistTmuxConfig() error {
	return savePersistedConfig(m.tmux.persisted())
}

func runExternalCmd(cmd *exec.Cmd) tea.Cmd {
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return externalCommandFinishedMsg{err: err}
	})
}

func commandInDir(dir string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd
}

func journalPathForRepo(repo git.RepoStatus) string {
	return resolveJournalPathForRepo(repo).Path
}

func resolveJournalPathForRepo(repo git.RepoStatus) journalResolution {
	baseDir := filepath.Join(repo.Path, ".claude", "features")
	primarySlug := journalSlug(repo.Branch)
	first := true
	for _, slug := range journalCandidateSlugs(repo.Branch) {
		candidate := filepath.Join(baseDir, "JOURNAL_"+slug+".md")
		if _, err := os.Stat(candidate); err == nil {
			source := "primary"
			if !first {
				source = "fallback-branch"
			}
			return journalResolution{
				Path:         candidate,
				ComputedSlug: primarySlug,
				ResolvedSlug: slug,
				FileName:     filepath.Base(candidate),
				Source:       source,
			}
		}
		first = false
	}

	matches, err := filepath.Glob(filepath.Join(baseDir, "JOURNAL_*.md"))
	if err == nil && len(matches) == 1 {
		return journalResolution{
			Path:         matches[0],
			ComputedSlug: primarySlug,
			ResolvedSlug: strings.TrimSuffix(strings.TrimPrefix(filepath.Base(matches[0]), "JOURNAL_"), ".md"),
			FileName:     filepath.Base(matches[0]),
			Source:       "fallback-single-match",
		}
	}

	defaultPath := filepath.Join(baseDir, "JOURNAL_"+primarySlug+".md")
	return journalResolution{
		Path:         defaultPath,
		ComputedSlug: primarySlug,
		ResolvedSlug: primarySlug,
		FileName:     filepath.Base(defaultPath),
		Source:       "primary-missing",
	}
}

func journalCandidateSlugs(branch string) []string {
	candidates := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, candidate := range []string{journalSlug(branch), normalizeJournalSlug(branch), lastBranchSegmentSlug(branch)} {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return []string{"detached"}
	}
	return candidates
}

func globalCommand(name string, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("global command not found in PATH: %s", name)
	}
	return exec.Command(path, args...), nil
}

func DependencyWarnings(tmux tmuxConfig) []string {
	warnings := make([]string, 0, 4)
	if _, err := exec.LookPath("lazygit"); err != nil {
		warnings = append(warnings, "lazygit not found in PATH; enter/l disabled")
	}
	if tmux.Active {
		if _, err := exec.LookPath("tmux"); err != nil {
			warnings = append(warnings, "tmux session detected but tmux binary not found; tmux integrations disabled")
		}
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		warnings = append(warnings, "opencode not found in PATH; o disabled")
	}
	if !tmux.Active && runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("osascript"); err != nil {
			warnings = append(warnings, "osascript not found in PATH; o disabled on this system")
		}
	}
	if !tmux.Active && runtime.GOOS != "darwin" {
		warnings = append(warnings, "osascript not found in PATH; o disabled on this system")
	}
	return warnings
}

func globalCommandInDir(dir string, name string, args ...string) (*exec.Cmd, error) {
	cmd, err := globalCommand(name, args...)
	if err != nil {
		return nil, err
	}
	cmd.Dir = dir
	return cmd, nil
}

func openLazygitCmd(repoPath string, tmux tmuxConfig) (*exec.Cmd, error) {
	if tmux.usable() {
		return tmux.command(repoPath, "exec lazygit")
	}

	return globalCommandInDir(repoPath, "lazygit")
}

func openOpenCodeCmd(repoPath string, tmux tmuxConfig) (*exec.Cmd, error) {
	if _, err := exec.LookPath("opencode"); err != nil {
		return nil, fmt.Errorf("global command not found in PATH: opencode")
	}
	if tmux.usable() {
		return tmux.command(repoPath, "exec opencode .")
	}
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("opencode fallback outside tmux is only implemented on macOS")
	}

	escapedPath := strings.ReplaceAll(repoPath, `"`, `\\"`)
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "opencode " & quoted form of "%s"
end tell`, escapedPath)
	return exec.Command("osascript", "-e", script), nil
}

func openJournalCmd(repoPath string, journalPath string, tmux tmuxConfig) (*exec.Cmd, error) {
	if tmux.usable() {
		relPath := strings.TrimPrefix(journalPath, repoPath+string(os.PathSeparator))
		return tmux.command(repoPath, fmt.Sprintf(`sh -lc 'exec "${EDITOR:-vi}" %q'`, relPath))
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", journalPath), nil
	}
	if _, err := exec.LookPath("xdg-open"); err == nil {
		return exec.Command("xdg-open", journalPath), nil
	}
	return nil, fmt.Errorf("journal fallback outside tmux requires macOS open or xdg-open")
}

func LoadTmuxConfigForModel() tmuxConfig {
	active := os.Getenv("TMUX") != ""
	base := defaultTmuxConfig(active)
	if persisted, ok := loadPersistedConfig(); ok {
		base.applyPersisted(persisted)
	}

	if mode := normalizeTmuxMode(strings.TrimSpace(os.Getenv("GITWS_TMUX_MODE"))); mode != "" {
		base.Mode = mode
	}
	if value := strings.TrimSpace(os.Getenv("GITWS_TMUX_POPUP_WIDTH")); value != "" {
		base.PopupWidth = value
	}
	if value := strings.TrimSpace(os.Getenv("GITWS_TMUX_POPUP_HEIGHT")); value != "" {
		base.PopupHeight = value
	}
	if value := strings.TrimSpace(os.Getenv("GITWS_TMUX_POPUP_X")); value != "" {
		base.PopupX = value
	}
	if value := strings.TrimSpace(os.Getenv("GITWS_TMUX_POPUP_Y")); value != "" {
		base.PopupY = value
	}
	if value := normalizeSplitDirection(strings.TrimSpace(os.Getenv("GITWS_TMUX_SPLIT_DIRECTION"))); value != "" {
		base.SplitDirection = value
	}
	if value := strings.TrimSpace(os.Getenv("GITWS_TMUX_SPLIT_SIZE")); value != "" {
		base.SplitSize = value
	}

	return base
}

func (c *tmuxConfig) toggleMode() {
	if c.Mode == "popup" {
		c.Mode = "split"
		return
	}
	if c.Mode == "split" {
		c.Mode = "window"
		return
	}
	c.Mode = "popup"
}

func (c tmuxConfig) usable() bool {
	if !c.Active {
		return false
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (c tmuxConfig) command(repoPath string, shellCommand string) (*exec.Cmd, error) {
	if !c.usable() {
		return nil, fmt.Errorf("tmux integration unavailable")
	}

	if c.Mode == "split" {
		args := []string{"split-window", "-c", repoPath, "-l", c.SplitSize}
		if c.SplitDirection == "right" {
			args = append(args, "-h")
		}
		args = append(args, shellCommand)
		return exec.Command("tmux", args...), nil
	}
	if c.Mode == "window" {
		return exec.Command("tmux", "new-window", "-c", repoPath, shellCommand), nil
	}

	args := []string{"popup", "-E", "-d", repoPath, "-w", c.PopupWidth, "-h", c.PopupHeight}
	if c.PopupX != "" {
		args = append(args, "-x", c.PopupX)
	}
	if c.PopupY != "" {
		args = append(args, "-y", c.PopupY)
	}
	args = append(args, shellCommand)
	return exec.Command("tmux", args...), nil
}

func getenvDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func displayConfigValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "auto"
	}
	return value
}

func cycleString(options []string, current string, delta int) string {
	if len(options) == 0 {
		return current
	}
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	index = (index + delta + len(options)) % len(options)
	return options[index]
}

func defaultTmuxConfig(active bool) tmuxConfig {
	defaults := defaultPersistedConfig()
	return tmuxConfig{
		Active:         active,
		Mode:           defaults.TmuxMode,
		PopupWidth:     defaults.PopupWidth,
		PopupHeight:    defaults.PopupHeight,
		PopupX:         defaults.PopupX,
		PopupY:         defaults.PopupY,
		SplitDirection: defaults.SplitDirection,
		SplitSize:      defaults.SplitSize,
	}
}

func (c *tmuxConfig) applyPersisted(cfg persistedConfig) {
	if mode := normalizeTmuxMode(cfg.TmuxMode); mode != "" {
		c.Mode = mode
	}
	if value := strings.TrimSpace(cfg.PopupWidth); value != "" {
		c.PopupWidth = value
	}
	if value := strings.TrimSpace(cfg.PopupHeight); value != "" {
		c.PopupHeight = value
	}
	if value := strings.TrimSpace(cfg.PopupX); value != "" {
		c.PopupX = value
	}
	if value := strings.TrimSpace(cfg.PopupY); value != "" {
		c.PopupY = value
	}
	if value := normalizeSplitDirection(cfg.SplitDirection); value != "" {
		c.SplitDirection = value
	}
	if value := strings.TrimSpace(cfg.SplitSize); value != "" {
		c.SplitSize = value
	}
}

func (c tmuxConfig) persisted() persistedConfig {
	return persistedConfig{
		TmuxMode:       c.Mode,
		PopupWidth:     c.PopupWidth,
		PopupHeight:    c.PopupHeight,
		PopupX:         c.PopupX,
		PopupY:         c.PopupY,
		SplitDirection: c.SplitDirection,
		SplitSize:      c.SplitSize,
	}
}

func normalizeTmuxMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "popup", "split", "window":
		return value
	default:
		return ""
	}
}

func normalizeSplitDirection(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "right", "down":
		return value
	default:
		return ""
	}
}

func journalSlug(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "detached" {
		return "detached"
	}
	parts := strings.Split(branch, "/")
	if len(parts) > 1 {
		branch = strings.Join(parts[1:], "-")
	}
	return normalizeJournalSlug(branch)
}

func normalizeJournalSlug(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "detached" {
		return "detached"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range branch {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-._")
	if result == "" {
		return "detached"
	}
	return result
}

func lastBranchSegmentSlug(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" || branch == "detached" {
		return "detached"
	}
	parts := strings.Split(branch, "/")
	return normalizeJournalSlug(parts[len(parts)-1])
}

func readJournalPreview(path string, maxLines int, maxWidth int) string {
	f, err := os.Open(path)
	if err != nil {
		return errorStyle.Render(err.Error())
	}
	defer f.Close()

	scanner := bufio.NewScanner(io.LimitReader(f, 32*1024))
	lines := make([]string, 0, maxLines)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \t")
		if maxWidth > 0 {
			line = truncateRight(line, maxWidth)
		}
		lines = append(lines, line)
		if len(lines) >= maxLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return errorStyle.Render(err.Error())
	}
	if len(lines) == 0 {
		return mutedStyle.Render("Journal empty")
	}

	return joinPreviewLines(lines, maxLines)
}

func truncateBlock(text string, maxLines int, maxWidth int) string {
	if strings.TrimSpace(text) == "" {
		return mutedStyle.Render("No diff preview available")
	}

	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, min(len(rawLines), maxLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, " \t")
		if maxWidth > 0 {
			line = truncateRight(line, maxWidth)
		}
		lines = append(lines, line)
		if len(lines) >= maxLines {
			break
		}
	}
	if len(lines) == 0 {
		return mutedStyle.Render("No diff preview available")
	}

	return joinPreviewLines(lines, maxLines)
}

func joinPreviewLines(lines []string, maxLines int) string {

	preview := strings.Join(lines, "\n")
	if len(lines) >= maxLines {
		preview += "\n" + mutedStyle.Render("...")
	}
	return preview
}

func truncateRight(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}

func scanReposCmd(root string) tea.Cmd {
	return func() tea.Msg {
		repos, err := git.Scan(root)
		if err != nil {
			return scanResultMsg{err: err}
		}

		sort.Slice(repos, func(i, j int) bool {
			if repos[i].Dirty != repos[j].Dirty {
				return repos[i].Dirty && !repos[j].Dirty
			}
			return repos[i].RelPath < repos[j].RelPath
		})

		return scanResultMsg{repos: repos}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
