package markdown

// Palette holds rendering colors for the markdown package.
// Populated from uiTheme in render.go.
type Palette struct {
	Body        string
	InlineCode  string
	Link        string
	Strike      string
	Heading1    string
	Heading2    string
	Heading3    string
	Heading4    string
	Quote       string
	Rule        string
	CodeHead    string
	CodeLine    string
	TableBorder string
	TableHeader string
	TableRow    string
	DisableANSI bool
}
