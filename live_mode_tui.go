package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type liveModeModel struct {
	ctx        context.Context
	api        *Twitch
	interval   time.Duration
	display    string
	sortBy     string
	sortOrder  string
	streams    []Stream
	lastUpdate time.Time
	refreshErr error
	fetching   bool
	hasResult  bool
	deviceURL  string
	deviceCode string
	width      int
	height     int
	scroll     int
}

type liveModeResultMsg struct {
	streams []Stream
	err     error
}

type liveModeTickMsg time.Time

type deviceAuthorizationMsg struct {
	url  string
	code string
}

func runLiveModeTUI(ctx context.Context, api *Twitch, interval time.Duration, display, sortBy, sortOrder string) error {
	uiCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := liveModeModel{
		ctx:       uiCtx,
		api:       api,
		interval:  interval,
		display:   display,
		sortBy:    sortBy,
		sortOrder: sortOrder,
		fetching:  true,
		width:     80,
		height:    24,
	}
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(uiCtx))
	api.Auth.OnDeviceAuthorization = func(verificationURL, userCode string) {
		program.Send(deviceAuthorizationMsg{url: verificationURL, code: userCode})
	}
	if _, err := program.Run(); err != nil {
		if uiCtx.Err() != nil || errors.Is(err, tea.ErrProgramKilled) {
			return nil
		}
		return fmt.Errorf("run live mode screen: %w", err)
	}
	return nil
}

func (m liveModeModel) Init() tea.Cmd { return m.fetch() }

func (m liveModeModel) fetch() tea.Cmd {
	return func() tea.Msg {
		streams, err := m.api.LiveFollowedSorted(m.ctx, m.sortBy, m.sortOrder)
		return liveModeResultMsg{streams: streams, err: err}
	}
}

func (m liveModeModel) scheduleRefresh() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg { return liveModeTickMsg(t) })
}

func (m liveModeModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampScroll()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "r":
			if !m.fetching {
				m.fetching = true
				m.refreshErr = nil
				return m, m.fetch()
			}
		case "up", "k":
			m.scroll--
		case "down", "j":
			m.scroll++
		case "pgup":
			m.scroll -= m.bodyHeight()
		case "pgdown":
			m.scroll += m.bodyHeight()
		case "home":
			m.scroll = 0
		case "end":
			m.scroll = m.maxScroll()
		}
		m.clampScroll()
	case deviceAuthorizationMsg:
		m.deviceURL = msg.url
		m.deviceCode = msg.code
	case liveModeResultMsg:
		m.fetching = false
		m.deviceURL = ""
		m.deviceCode = ""
		if msg.err != nil {
			m.refreshErr = msg.err
		} else {
			m.streams = msg.streams
			m.lastUpdate = time.Now()
			m.refreshErr = nil
			m.hasResult = true
		}
		m.clampScroll()
		return m, m.scheduleRefresh()
	case liveModeTickMsg:
		if !m.fetching {
			m.fetching = true
			m.refreshErr = nil
			return m, m.fetch()
		}
	}
	return m, nil
}

func (m liveModeModel) View() string {
	width := m.width
	if width < 20 {
		width = 20
	}
	if width > 160 {
		width = 160
	}
	header := []string{titleStyle.Render("LIVE FOLLOWED CHANNELS")}
	status := fmt.Sprintf("Refresh every %s", formatRefreshInterval(m.interval))
	if m.fetching {
		status += " · updating"
	}
	if !m.lastUpdate.IsZero() {
		status += " · updated " + m.lastUpdate.Format("15:04:05")
	}
	if m.refreshErr != nil {
		status += " · refresh failed: " + m.refreshErr.Error()
	}
	header = append(header, wrapText(status, width))
	if m.deviceURL != "" {
		header = append(header, "Sign in: "+m.deviceURL, "Enter code: "+m.deviceCode)
	}
	header = append(header, strings.Repeat("─", width))

	var body []string
	switch {
	case !m.hasResult && m.refreshErr != nil:
		body = []string{"Could not load live channels. Press r to retry."}
	case !m.hasResult && m.fetching:
		body = []string{"Loading followed channels…"}
	default:
		if m.display == "table" {
			body = strings.Split(renderStreamTable(m.streams, width), "\n")
		} else {
			body = strings.Split(renderStreamCards(m.streams, width-2), "\n")
		}
	}

	start := m.scroll
	end := start + m.bodyHeight()
	if end > len(body) {
		end = len(body)
	}
	visible := body[start:end]
	lines := append(header, visible...)
	if m.refreshErr != nil && m.hasResult {
		lines = append(lines, mutedStyle.Render("Showing the last successful results."))
	}
	lines = append(lines, mutedStyle.Render("↑/↓ scroll · r refresh · q quit"))
	return strings.Join(lines, "\n")
}

func (m liveModeModel) bodyHeight() int {
	headerLines := 3
	if m.deviceURL != "" {
		headerLines += 2
	}
	height := m.height - headerLines - 1
	if height < 1 {
		return 1
	}
	return height
}

func (m liveModeModel) maxScroll() int {
	width := m.width
	if width < 20 {
		width = 20
	}
	var rendered string
	if m.display == "table" {
		rendered = renderStreamTable(m.streams, width)
	} else {
		rendered = renderStreamCards(m.streams, width-2)
	}
	count := len(strings.Split(rendered, "\n"))
	if !m.hasResult && (m.fetching || m.refreshErr != nil) {
		count = 1
	}
	max := count - m.bodyHeight()
	if max < 0 {
		return 0
	}
	return max
}

func (m *liveModeModel) clampScroll() {
	if m.scroll < 0 {
		m.scroll = 0
	}
	if max := m.maxScroll(); m.scroll > max {
		m.scroll = max
	}
}
