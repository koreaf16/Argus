package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	examplesPerCategory = 5
)

type snapshotConfig struct {
	Snapshot string
	Example  int
	Variant  uiVariant
	NoANSI   bool
	Width    int
}

type tickMsg time.Time

type model struct {
	categories []category
	catIndex   int
	example    int
	frame      int
	width      int
	ansi       bool
	variant    uiVariant
}

func main() {
	cfg, interactive, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "argus_demo: %v\n", err)
		os.Exit(2)
	}

	if !interactive {
		out, err := renderSnapshotOutput(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "argus_demo: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
		return
	}

	m := model{
		categories: catalog(),
		catIndex:   0,
		example:    0,
		width:      cfg.Width,
		ansi:       !cfg.NoANSI,
		variant:    cfg.Variant,
	}

	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "argus_demo: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs() (snapshotConfig, bool, error) {
	var rawVariant string
	cfg := snapshotConfig{}

	fs := flag.NewFlagSet("argus_demo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.Snapshot, "snapshot", "", "snapshot target (all or category id)")
	fs.IntVar(&cfg.Example, "example", 0, "example index (1-5), 0 means all examples")
	fs.StringVar(&rawVariant, "variant", "argus-signature", "current|argus-signature|minimal-pro")
	fs.BoolVar(&cfg.NoANSI, "no-ansi", false, "disable ANSI color styling")
	fs.IntVar(&cfg.Width, "width", 0, "render width (interactive default: terminal width, snapshot default: 120)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return cfg, false, err
	}

	if cfg.Example < 0 || cfg.Example > examplesPerCategory {
		return cfg, false, fmt.Errorf("--example must be 0..%d", examplesPerCategory)
	}
	if cfg.Width < 0 {
		return cfg, false, fmt.Errorf("--width must be >= 0")
	}

	v, err := parseVariant(rawVariant)
	if err != nil {
		return cfg, false, err
	}
	cfg.Variant = v

	return cfg, strings.TrimSpace(cfg.Snapshot) == "", nil
}

func (m model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Every(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		if m.width == 0 {
			m.width = maxInt(72, v.Width-4)
		}
	case tickMsg:
		m.frame++
		return m, tick()
	case tea.KeyMsg:
		switch v.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.example = (m.example + 1) % examplesPerCategory
		case "shift+tab":
			m.example = (m.example - 1 + examplesPerCategory) % examplesPerCategory
		case "left":
			m.catIndex = (m.catIndex - 1 + len(m.categories)) % len(m.categories)
			m.example = 0
		case "right":
			m.catIndex = (m.catIndex + 1) % len(m.categories)
			m.example = 0
		case "[":
			m.width = maxInt(72, m.width-8)
		case "]":
			m.width += 8
		case "a":
			m.ansi = !m.ansi
		case "v":
			m.variant = nextVariant(m.variant)
		default:
			if idx := categoryIndexByKey(m.categories, v.String()); idx >= 0 {
				m.catIndex = idx
				m.example = 0
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	if len(m.categories) == 0 {
		return "no categories configured"
	}

	cat := m.categories[m.catIndex]
	width := m.width
	if width <= 0 {
		width = 120
	}
	theme := themeFor(m.variant, m.ansi)
	ctx := renderContext{
		Width:    width,
		Frame:    m.frame,
		Theme:    theme,
		Category: cat,
		Example:  m.example,
	}

	title := paint(theme.ANSI, "ARGUS DEMO RENDER LAB", theme.Accent, "", true, false)
	sub := fmt.Sprintf("category: %s (%s)  |  example: %d/%d  |  variant: %s  |  ansi: %v  |  width: %d",
		cat.Name, cat.ID, m.example+1, examplesPerCategory, m.variant, m.ansi, width)

	nav := renderCategoryNav(m.categories, m.catIndex, theme, width)
	content := renderCategoryExample(cat, m.example, ctx)
	footer := paint(theme.ANSI,
		"keys: 1-9/0/- category  tab cycle example  v variant  a ansi  [ ] width  left/right category  q quit",
		theme.Muted, "", false, false)

	parts := []string{title, paint(theme.ANSI, sub, theme.Muted, "", false, false), nav, "", content, "", footer}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func renderSnapshotOutput(cfg snapshotConfig) (string, error) {
	width := cfg.Width
	if width <= 0 {
		width = 120
	}

	theme := themeFor(cfg.Variant, !cfg.NoANSI)
	cats := catalog()

	target := strings.TrimSpace(strings.ToLower(cfg.Snapshot))
	selected := make([]category, 0, len(cats))
	if target == "all" {
		selected = append(selected, cats...)
	} else {
		cat, ok := findCategory(cats, target)
		if !ok {
			return "", fmt.Errorf("unknown snapshot category %q", cfg.Snapshot)
		}
		selected = append(selected, cat)
	}

	indices := []int{}
	if cfg.Example == 0 {
		for i := 0; i < examplesPerCategory; i++ {
			indices = append(indices, i)
		}
	} else {
		indices = append(indices, cfg.Example-1)
	}

	sections := make([]string, 0, len(selected)*len(indices))
	for _, cat := range selected {
		header := fmt.Sprintf("=== [%s] %s (%s) ===", cat.Key, cat.Name, cat.ID)
		sections = append(sections, paint(theme.ANSI, header, theme.Accent, "", true, false))
		for _, idx := range indices {
			label := fmt.Sprintf("--- example %d: %s ---", idx+1, cat.ExampleNames[idx])
			ctx := renderContext{
				Width:    width,
				Frame:    idx * 3,
				Theme:    theme,
				Category: cat,
				Example:  idx,
			}
			sections = append(sections,
				paint(theme.ANSI, label, theme.Muted, "", false, false),
				renderCategoryExample(cat, idx, ctx),
			)
		}
	}

	return strings.Join(sections, "\n\n") + "\n", nil
}

func renderCategoryNav(cats []category, active int, theme demoTheme, width int) string {
	items := make([]string, 0, len(cats))
	for i, cat := range cats {
		text := fmt.Sprintf("%s:%s", cat.Key, cat.ID)
		if i == active {
			items = append(items, paint(theme.ANSI, "["+text+"]", theme.Accent, "", true, false))
		} else {
			items = append(items, paint(theme.ANSI, "["+text+"]", theme.Muted, "", false, false))
		}
	}
	line := strings.Join(items, " ")
	return trimToWidth(line, width)
}

func categoryIndexByKey(cats []category, key string) int {
	for i := range cats {
		if cats[i].Key == key {
			return i
		}
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
