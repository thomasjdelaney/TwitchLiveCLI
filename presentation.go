package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9146FF"))
	channelStyle = lipgloss.NewStyle().Bold(true)
	viewerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C781"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A"))
)

func render(streams []Stream, table bool) {
	width := terminalWidth()
	if table {
		fmt.Println(renderStreams(streams, width, true))
		return
	}
	fmt.Println(renderStreams(streams, width, false))
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
	if len(streams) == 0 {
		return mutedStyle.Render("Nobody you follow is live right now. Check back later.")
	}
	if width < 13 {
		width = 13
	}
	const separator = "  "
	headers := []string{"Channel", "Title", "Category", "Viewers", "Live For"}
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
		rows = append(rows, []string{name, stream.Title, category, comma(stream.Viewers), liveFor})
	}

	widths := make([]int, len(headers))
	maxWidths := []int{20, 60, 24, 9, 9}
	for i, header := range headers {
		widths[i] = lipgloss.Width(header)
		for _, row := range rows {
			if cellWidth := lipgloss.Width(row[i]); cellWidth > widths[i] {
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
	writeTableRow(&output, headers, widths, separator, true)
	separators := make([]string, len(widths))
	for i, colWidth := range widths {
		separators[i] = strings.Repeat("─", colWidth)
	}
	writeTableRow(&output, separators, widths, separator, false)
	for _, row := range rows {
		writeTableRow(&output, row, widths, separator, false)
	}
	return output.String()
}

func totalWidth(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}

func writeTableRow(output *strings.Builder, cells []string, widths []int, separator string, header bool) {
	for i, cell := range cells {
		if i > 0 {
			output.WriteString(separator)
		}
		cell = ansi.Truncate(cell, widths[i], "…")
		cell += strings.Repeat(" ", widths[i]-lipgloss.Width(cell))
		if header {
			cell = titleStyle.Render(cell)
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

func renderStreamCards(streams []Stream, width int) string {
	if len(streams) == 0 {
		return mutedStyle.Render("Nobody you follow is live right now. Check back later.")
	}
	if width < 12 {
		width = 12
	}
	var output strings.Builder
	for i, stream := range streams {
		name := stream.Name
		if name == "" {
			name = stream.Login
		}
		fmt.Fprintf(&output, "%s  %s\n", channelStyle.Render(name), viewerStyle.Render(fmt.Sprintf("%s viewers", comma(stream.Viewers))))
		fmt.Fprintf(&output, "  %s\n", wrapText(stream.Title, width-2))
		category := stream.Game
		if category == "" {
			category = "No category"
		}
		fmt.Fprintf(&output, "  %s", mutedStyle.Render(category))
		if !stream.StartedAt.IsZero() {
			fmt.Fprintf(&output, "  %s", mutedStyle.Render("· live for "+duration(time.Since(stream.StartedAt))))
		}
		output.WriteByte('\n')
		if i != len(streams)-1 {
			output.WriteByte('\n')
		}
	}
	return strings.TrimRight(output.String(), "\n")
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
