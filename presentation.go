package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type colorPalette struct {
	title, viewer, muted color.Color
	rows                 [2]color.Color
	selectedBackground   color.Color
	selectedForeground   color.Color
	rail                 [2]color.Color
}

func darkPalette() colorPalette {
	return colorPalette{
		title: lipgloss.Color("#9146FF"), viewer: lipgloss.Color("#00C781"), muted: lipgloss.Color("#8A8A8A"),
		rows:               [2]color.Color{lipgloss.Color("#191B1E"), lipgloss.Color("#3C424A")},
		selectedBackground: lipgloss.Color("#59636F"), selectedForeground: lipgloss.Color("#FFFFFF"),
		rail: [2]color.Color{lipgloss.Color("#56C9A8"), lipgloss.Color("#E7B95C")},
	}
}

func lightPalette() colorPalette {
	return colorPalette{
		title: lipgloss.Color("#6D28D9"), viewer: lipgloss.Color("#147D55"), muted: lipgloss.Color("#56616D"),
		rows:               [2]color.Color{lipgloss.Color("#F5F7FA"), lipgloss.Color("#DDE5EE")},
		selectedBackground: lipgloss.Color("#B6CCE0"), selectedForeground: lipgloss.Color("#172B3D"),
		rail: [2]color.Color{lipgloss.Color("#087E6B"), lipgloss.Color("#9A5B00")},
	}
}

// A zero-value palette keeps one-shot output readable in any terminal theme.
func (p colorPalette) titleStyle() lipgloss.Style {
	return styleWithForeground(lipgloss.NewStyle().Bold(true), p.title)
}
func (p colorPalette) channelStyle() lipgloss.Style { return lipgloss.NewStyle().Bold(true) }
func (p colorPalette) viewerStyle() lipgloss.Style {
	return styleWithForeground(lipgloss.NewStyle(), p.viewer)
}
func (p colorPalette) mutedStyle() lipgloss.Style {
	return styleWithForeground(lipgloss.NewStyle(), p.muted)
}
func styleWithForeground(style lipgloss.Style, foreground color.Color) lipgloss.Style {
	if foreground != nil {
		return style.Foreground(foreground)
	}
	return style
}

func render(streams []Stream, table bool) {
	width := terminalWidth()
	if table {
		fmt.Println(renderStreams(streams, width, true))
		return
	}
	fmt.Println(renderStreams(streams, width, false))
}

type streamLineSpan struct {
	start int
	end   int
}

func terminalWidth() int {
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width < 20 {
		return 80
	}
	return width
}

func renderStreams(streams []Stream, width int, table bool) string {
	palette := colorPalette{}
	var output strings.Builder
	output.WriteString(palette.titleStyle().Render("LIVE FOLLOWED CHANNELS"))
	output.WriteByte('\n')
	if len(streams) == 0 {
		output.WriteString(palette.mutedStyle().Render("Nobody you follow is live right now. Check back later."))
		return output.String()
	}
	output.WriteString(palette.mutedStyle().Render(fmt.Sprintf("%d channel(s) live", len(streams))))
	output.WriteString("\n\n")
	if table {
		output.WriteString(renderStreamTableWithPalette(streams, width, -1, palette))
	} else {
		output.WriteString(renderStreamCardsWithPalette(streams, width, -1, palette))
	}
	return strings.TrimRight(output.String(), "\n")
}

func renderStreamTable(streams []Stream, width int) string {
	output := renderStreamTableWithPalette(streams, width, -1, darkPalette())
	return output
}

func renderStreamTableWithSelection(streams []Stream, width, selected int) (string, []streamLineSpan) {
	return renderStreamTableWithPaletteAndSpans(streams, width, selected, darkPalette())
}

func renderStreamTableWithPalette(streams []Stream, width, selected int, palette colorPalette) string {
	output, _ := renderStreamTableWithPaletteAndSpans(streams, width, selected, palette)
	return output
}

func renderStreamTableWithPaletteAndSpans(streams []Stream, width, selected int, palette colorPalette) (string, []streamLineSpan) {
	if len(streams) == 0 {
		return palette.mutedStyle().Render("Nobody you follow is live right now. Check back later."), nil
	}
	if width < 13 {
		width = 13
	}
	const separator = "  "
	showSubscriptions := false
	for _, stream := range streams {
		if stream.Subscription != nil {
			showSubscriptions = true
			break
		}
	}
	headers := []string{"Channel", "Title", "Category", "Viewers", "Live For"}
	if showSubscriptions {
		headers = append(headers, "Subscription")
	}
	rows := make([][]string, 0, len(streams))
	for _, stream := range streams {
		name := stream.Name
		if name == "" {
			name = stream.Login
		}
		category := stream.Game
		if category == "" {
			category = "No category"
		}
		liveFor := "—"
		if !stream.StartedAt.IsZero() {
			liveFor = duration(time.Since(stream.StartedAt))
		}
		row := []string{name, stream.Title, category, comma(stream.Viewers), liveFor}
		if showSubscriptions {
			row = append(row, subscriptionLabel(stream.Subscription))
		}
		rows = append(rows, row)
	}

	widths := make([]int, len(headers))
	maxWidths := []int{20, 60, 24, 9, 9}
	if showSubscriptions {
		maxWidths = append(maxWidths, 20)
	}
	for i, header := range headers {
		widths[i] = ansi.StringWidthWc(header)
		for _, row := range rows {
			if cellWidth := ansi.StringWidthWc(row[i]); cellWidth > widths[i] {
				widths[i] = cellWidth
			}
		}
		if widths[i] > maxWidths[i] {
			widths[i] = maxWidths[i]
		}
	}

	available := width - len(separator)*(len(headers)-1)
	for excess := totalWidth(widths) - available; excess > 0; excess-- {
		for _, column := range []int{1, 2, 0, 4, 3} {
			if widths[column] > 1 {
				widths[column]--
				break
			}
		}
	}

	var output strings.Builder
	writeTableRow(&output, headers, widths, separator, true, nil, false, palette)
	separators := make([]string, len(widths))
	for i, colWidth := range widths {
		separators[i] = strings.Repeat("─", colWidth)
	}
	writeTableRow(&output, separators, widths, separator, false, nil, false)
	spans := make([]streamLineSpan, len(rows))
	for i, row := range rows {
		background := palette.rows[i%len(palette.rows)]
		isSelected := i == selected
		if isSelected {
			background = palette.selectedBackground
		}
		writeTableRow(&output, row, widths, separator, false, background, isSelected, palette)
		spans[i] = streamLineSpan{start: i + 2, end: i + 3}
	}
	return output.String(), spans
}

func tableRowBackground(row int) color.Color {
	palette := darkPalette()
	return palette.rows[row%len(palette.rows)]
}

func cardRailColor(card int) color.Color {
	palette := darkPalette()
	return palette.rail[card%len(palette.rail)]
}

var selectedItemBackground = darkPalette().selectedBackground

func totalWidth(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func writeTableRow(output *strings.Builder, cells []string, widths []int, separator string, header bool, rowBackground color.Color, selected bool, palettes ...colorPalette) {
	palette := colorPalette{}
	if len(palettes) > 0 {
		palette = palettes[0]
	}
	for i, cell := range cells {
		if i > 0 {
			if rowBackground != nil && !header {
				output.WriteString(lipgloss.NewStyle().Background(rowBackground).Render(separator))
			} else {
				output.WriteString(separator)
			}
		}
		cell = fitTableCell(cell, widths[i])
		if header {
			cell = palette.titleStyle().Render(cell)
		} else if rowBackground != nil {
			style := lipgloss.NewStyle().Background(rowBackground)
			if selected {
				style = style.Bold(true)
			}
			switch i {
			case 0:
				style = style.Bold(true)
			case 2, 4, 5:
				style = styleWithForeground(style, palette.muted)
			case 3:
				style = styleWithForeground(style, palette.viewer)
			}
			cell = style.Render(cell)
		} else if i == 0 {
			cell = palette.channelStyle().Render(cell)
		} else if i == 3 {
			cell = palette.viewerStyle().Render(cell)
		} else if i == 2 || i == 4 {
			cell = palette.mutedStyle().Render(cell)
		}
		output.WriteString(cell)
	}
	output.WriteByte('\n')
}

// fitTableCell uses wcwidth measurements throughout so emoji whose grapheme
// width differs from their terminal cell width do not shift later columns.
func fitTableCell(cell string, width int) string {
	if ansi.StringWidthWc(cell) > width {
		cell = ansi.TruncateWc(cell, width, "…")
	}
	if padding := width - ansi.StringWidthWc(cell); padding > 0 {
		cell += strings.Repeat(" ", padding)
	}
	return cell
}

func renderStreamCards(streams []Stream, width int) string {
	output := renderStreamCardsWithPalette(streams, width, -1, darkPalette())
	return output
}

func renderStreamCardsWithSelection(streams []Stream, width, selected int) (string, []streamLineSpan) {
	return renderStreamCardsWithPaletteAndSpans(streams, width, selected, darkPalette())
}

func renderStreamCardsWithPalette(streams []Stream, width, selected int, palette colorPalette) string {
	output, _ := renderStreamCardsWithPaletteAndSpans(streams, width, selected, palette)
	return output
}

func renderStreamCardsWithPaletteAndSpans(streams []Stream, width, selected int, palette colorPalette) (string, []streamLineSpan) {
	if len(streams) == 0 {
		return palette.mutedStyle().Render("Nobody you follow is live right now. Check back later."), nil
	}
	if width < 12 {
		width = 12
	}
	contentWidth := width - 1
	var output strings.Builder
	spans := make([]streamLineSpan, len(streams))
	lineIndex := 0
	for i, stream := range streams {
		name := stream.Name
		if name == "" {
			name = stream.Login
		}
		viewers := fmt.Sprintf("%s viewers", comma(stream.Viewers))
		viewerWidth := contentWidth / 2
		if lipgloss.Width(viewers) > viewerWidth {
			viewers = ansi.Truncate(viewers, viewerWidth, "…")
		}
		nameWidth := contentWidth - lipgloss.Width(viewers) - 2
		name = ansi.Truncate(name, nameWidth, "…")
		plainLines := []string{fmt.Sprintf("%s  %s", name, viewers)}
		styledLines := []string{fmt.Sprintf("%s  %s", palette.channelStyle().Render(name), palette.viewerStyle().Render(viewers))}
		plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, stream.Title, contentWidth-2, palette)
		category := stream.Game
		if category == "" {
			category = "No category"
		}
		categoryLine := category
		if !stream.StartedAt.IsZero() {
			categoryLine += "  · live for " + duration(time.Since(stream.StartedAt))
		}
		plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, categoryLine, contentWidth-2, palette)
		if stream.Subscription != nil {
			subscriptionLine := "Subscription: " + subscriptionLabel(stream.Subscription)
			plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, subscriptionLine, contentWidth-2, palette)
		}
		spans[i].start = lineIndex
		isSelected := i == selected
		for lineIndexInCard, line := range styledLines {
			railGlyph := "│"
			trailStyle := styleWithForeground(lipgloss.NewStyle(), palette.rail[i%len(palette.rail)])
			if isSelected {
				railGlyph = "┃"
				trailStyle = trailStyle.Bold(true)
				line = selectedCardLine(plainLines[lineIndexInCard], contentWidth, palette)
			}
			rail := trailStyle.Render(railGlyph)
			fmt.Fprintf(&output, "%s%s\n", rail, line)
			lineIndex++
		}
		spans[i].end = lineIndex
		if i != len(streams)-1 {
			output.WriteByte('\n')
			lineIndex++
		}
	}
	return strings.TrimRight(output.String(), "\n"), spans
}

func appendStyledCardDetails(plainLines, styledLines []string, text string, width int, palettes ...colorPalette) ([]string, []string) {
	palette := colorPalette{}
	if len(palettes) > 0 {
		palette = palettes[0]
	}
	for _, line := range strings.Split(wrapText(text, width), "\n") {
		plainLines = append(plainLines, "  "+line)
		styledLines = append(styledLines, "  "+palette.mutedStyle().Render(line))
	}
	return plainLines, styledLines
}

func subscriptionLabel(info *SubscriptionInfo) string {
	if info == nil {
		return ""
	}
	if !info.Subscribed {
		return "No"
	}
	if info.Tier > 0 && info.Tier%1000 == 0 {
		return fmt.Sprintf("Tier %d", info.Tier/1000)
	}
	if info.Tier > 0 {
		return fmt.Sprintf("Tier %s", comma(info.Tier))
	}
	return "Subscribed"
}

func selectedCardLine(line string, width int, palettes ...colorPalette) string {
	palette := colorPalette{}
	if len(palettes) > 0 {
		palette = palettes[0]
	}
	line = ansi.Truncate(line, width, "…")
	line += strings.Repeat(" ", width-lipgloss.Width(line))
	style := lipgloss.NewStyle().Bold(true)
	if palette.selectedBackground != nil {
		style = style.Background(palette.selectedBackground)
	}
	return styleWithForeground(style, palette.selectedForeground).Render(line)
}

func duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func formatRefreshInterval(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	units := []struct {
		duration time.Duration
		suffix   string
	}{
		{time.Hour, "h"},
		{time.Minute, "m"},
		{time.Second, "s"},
		{time.Millisecond, "ms"},
		{time.Microsecond, "us"},
		{time.Nanosecond, "ns"},
	}
	parts := make([]string, 0, len(units))
	for _, unit := range units {
		value := d / unit.duration
		if value > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", value, unit.suffix))
			d %= unit.duration
		}
	}
	return strings.Join(parts, " ")
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func wrapText(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return "" + s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	for i, word := range words {
		words[i] = ansi.Truncate(word, width, "…")
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(line)+1+lipgloss.Width(word) > width {
			lines = append(lines, line)
			line = word
		} else {
			line += " " + word
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n  ")
}
