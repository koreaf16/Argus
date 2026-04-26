package promptinput

func RenderTextInput(inner string, width int) string {
	return RenderTextInputWithTheme(inner, width, ResolveTheme("default"))
}

func RenderTextInputWithTheme(inner string, width int, theme Theme) string {
	return RenderBaseTextInputWithTheme(inner, width, theme)
}
