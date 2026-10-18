package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9146FF"))
	channelStyle = lipgloss.NewStyle().Bold(true)
	viewerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00C781"))
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A"))
)

func render(streams []Stream) {
	fmt.Println(titleStyle.Render("LIVE FOLLOWED CHANNELS"))
	if len(streams) == 0 {
		fmt.Println(mutedStyle.Render("Nobody you follow is live right now. Check back later."))
		return
	}
	fmt.Printf("%s\n\n", mutedStyle.Render(fmt.Sprintf("%d channel(s) live", len(streams))))
	for i, stream := range streams {
		name := stream.Name
		if name == "" {
			name = stream.Login
		}
		fmt.Printf("%s  %s\n", channelStyle.Render(name), viewerStyle.Render(fmt.Sprintf("%s viewers", comma(stream.Viewers))))
		fmt.Printf("  %s\n", wrapText(stream.Title, 74))
		category := stream.Game
		if category == "" {
			category = "No category"
		}
		fmt.Printf("  %s", mutedStyle.Render(category))
		if !stream.StartedAt.IsZero() {
			fmt.Printf("  %s", mutedStyle.Render("· live for "+duration(time.Since(stream.StartedAt))))
		}
		fmt.Println()
		if i != len(streams)-1 {
			fmt.Println()
		}
	}
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
	if width <= 0 || len(s) <= width {
		return "" + s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
		} else {
			line += " " + word
		}
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n  ")
}
