package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

var (
	titleStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9146FF"))
	channelStyle        = lipgloss.NewStyle().Bold(true)
	viewerStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C781"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A"))
	tableRowBackgrounds = [...]compat.AdaptiveColor{
		{Light: lipgloss.Color("#FAFAFA"), Dark: lipgloss.Color("#191B1E")},
		{Light: lipgloss.Color("#D6DDE3"), Dark: lipgloss.Color("#3C424A")},
	}
	selectedItemBackground = compat.AdaptiveColor{Light: lipgloss.Color("#AEBCC8"), Dark: lipgloss.Color("#59636F")}
	selectedItemForeground = compat.AdaptiveColor{Light: lipgloss.Color("#111820"), Dark: lipgloss.Color("#FFFFFF")}
	cardRailColors         = [...]compat.AdaptiveColor{
		{Light: lipgloss.Color("#18866B"), Dark: lipgloss.Color("#56C9A8")},
		{Light: lipgloss.Color("#A66B13"), Dark: lipgloss.Color("#E7B95C")},
	}
)

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
	var output strings.Builder
	output.WriteString(titleStyle.Render("LIVE FOLLOWED CHANNELS"))
	output.WriteByte('\n')
	if len(streams) == 0 {
		output.WriteString(mutedStyle.Render("Nobody you follow is live right now. Check back later."))
		return output.String()
	}
	output.WriteString(mutedStyle.Render(fmt.Sprintf("%d channel(s) live", len(streams))))
	output.WriteString("\n\n")
	if table {
		output.WriteString(renderStreamTable(streams, width))
	} else {
		output.WriteString(renderStreamCards(streams, width))
	}
	return strings.TrimRight(output.String(), "\n")
}

func renderStreamTable(streams []Stream, width int) string {
	output, _ := renderStreamTableWithSelection(streams, width, -1)
	return output
}

func renderStreamTableWithSelection(streams []Stream, width, selected int) (string, []streamLineSpan) {
	if len(streams) == 0 {
		return mutedStyle.Render("Nobody you follow is live right now. Check back later."), nil
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
	writeTableRow(&output, headers, widths, separator, true, nil, false)
	separators := make([]string, len(widths))
	for i, colWidth := range widths {
		separators[i] = strings.Repeat("─", colWidth)
	}
	writeTableRow(&output, separators, widths, separator, false, nil, false)
	spans := make([]streamLineSpan, len(rows))
	for i, row := range rows {
		background := tableRowBackground(i)
		isSelected := i == selected
		if isSelected {
			background = selectedItemBackground
		}
		writeTableRow(&output, row, widths, separator, false, &background, isSelected)
		spans[i] = streamLineSpan{start: i + 2, end: i + 3}
	}
	return output.String(), spans
}

func tableRowBackground(row int) compat.AdaptiveColor {
	return tableRowBackgrounds[row%len(tableRowBackgrounds)]
}

func totalWidth(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func writeTableRow(output *strings.Builder, cells []string, widths []int, separator string, header bool, rowBackground *compat.AdaptiveColor, selected bool) {
	for i, cell := range cells {
		if i > 0 {
			if rowBackground != nil && !header {
				output.WriteString(lipgloss.NewStyle().Background(*rowBackground).Render(separator))
			} else {
				output.WriteString(separator)
			}
		}
		cell = fitTableCell(cell, widths[i])
		if header {
			cell = titleStyle.Render(cell)
		} else if rowBackground != nil {
			style := lipgloss.NewStyle().Background(*rowBackground)
			if selected {
				style = style.Bold(true)
			}
			switch i {
			case 0:
				style = style.Bold(true)
			case 2, 4, 5:
				style = style.Foreground(lipgloss.Color("#8A8A8A"))
			case 3:
				style = style.Foreground(lipgloss.Color("#00C781"))
			}
			cell = style.Render(cell)
		} else if i == 0 {
			cell = channelStyle.Render(cell)
		} else if i == 3 {
			cell = viewerStyle.Render(cell)
		} else if i == 2 || i == 4 {
			cell = mutedStyle.Render(cell)
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
	output, _ := renderStreamCardsWithSelection(streams, width, -1)
	return output
}

func renderStreamCardsWithSelection(streams []Stream, width, selected int) (string, []streamLineSpan) {
	if len(streams) == 0 {
		return mutedStyle.Render("Nobody you follow is live right now. Check back later."), nil
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
		styledLines := []string{fmt.Sprintf("%s  %s", channelStyle.Render(name), viewerStyle.Render(viewers))}
		plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, stream.Title, contentWidth-2)
		category := stream.Game
		if category == "" {
			category = "No category"
		}
		categoryLine := category
		if !stream.StartedAt.IsZero() {
			categoryLine += "  · live for " + duration(time.Since(stream.StartedAt))
		}
		plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, categoryLine, contentWidth-2)
		if stream.Subscription != nil {
			subscriptionLine := "Subscription: " + subscriptionLabel(stream.Subscription)
			plainLines, styledLines = appendStyledCardDetails(plainLines, styledLines, subscriptionLine, contentWidth-2)
		}
		spans[i].start = lineIndex
		isSelected := i == selected
		for lineIndexInCard, line := range styledLines {
			railGlyph := "│"
			trailStyle := lipgloss.NewStyle().Foreground(cardRailColor(i))
			if isSelected {
				railGlyph = "┃"
				trailStyle = trailStyle.Bold(true)
				line = selectedCardLine(plainLines[lineIndexInCard], contentWidth)
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

func appendStyledCardDetails(plainLines, styledLines []string, text string, width int) ([]string, []string) {
	for _, line := range strings.Split(wrapText(text, width), "\n") {
		plainLines = append(plainLines, "  "+line)
		styledLines = append(styledLines, "  "+mutedStyle.Render(line))
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

func selectedCardLine(line string, width int) string {
	line = ansi.Truncate(line, width, "…")
	line += strings.Repeat(" ", width-lipgloss.Width(line))
	return lipgloss.NewStyle().Background(selectedItemBackground).Foreground(selectedItemForeground).Bold(true).Render(line)
}

func cardRailColor(card int) compat.AdaptiveColor {
	return cardRailColors[card%len(cardRailColors)]
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
