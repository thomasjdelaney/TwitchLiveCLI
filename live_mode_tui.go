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
	viewer     viewerOptions
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
	selected   int
	openErr    error
	launching  bool
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

type streamOpenResultMsg struct {
	err error
}

type viewerPreparedMsg struct {
	plan launchPlan
	err  error
}

func runLiveModeTUI(ctx context.Context, api *Twitch, interval time.Duration, display, sortBy, sortOrder string, viewer viewerOptions) error {
	uiCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	model := liveModeModel{
		ctx:       uiCtx,
		api:       api,
		interval:  interval,
		display:   display,
		sortBy:    sortBy,
		sortOrder: sortOrder,
		viewer:    viewer,
		fetching:  true,
		width:     80,
		height:    24,
		selected:  -1,
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
		case "enter":
			if !m.launching {
				if cmd := m.openSelected(); cmd != nil {
					m.launching = true
					m.openErr = nil
					return m, cmd
				}
			}
		case "r":
			if !m.fetching {
				m.fetching = true
				m.refreshErr = nil
				return m, m.fetch()
			}
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "pgup":
			m.scroll -= m.bodyHeight()
		case "pgdown":
			m.scroll += m.bodyHeight()
		case "home":
			m.selectStream(0)
		case "end":
			m.selectStream(len(m.streams) - 1)
		}
		m.clampScroll()
	case deviceAuthorizationMsg:
		m.deviceURL = msg.url
		m.deviceCode = msg.code
	case streamOpenResultMsg:
		m.launching = false
		m.openErr = msg.err
	case viewerPreparedMsg:
		if msg.err != nil {
			m.launching = false
			m.openErr = msg.err
			break
		}
		if err := startChat(msg.plan); err != nil {
			m.launching = false
			m.openErr = err
			break
		}
		return m, tea.ExecProcess(msg.plan.stream, func(err error) tea.Msg {
			if err != nil {
				err = fmt.Errorf("run Streamlink: %w", err)
			}
			return streamOpenResultMsg{err: err}
		})
	case liveModeResultMsg:
		m.fetching = false
		m.deviceURL = ""
		m.deviceCode = ""
		if msg.err != nil {
			m.refreshErr = msg.err
		} else {
			m.selected = reconcileSelectedStream(m.streams, m.selected, msg.streams)
			m.streams = msg.streams
			m.lastUpdate = time.Now()
			m.refreshErr = nil
			m.hasResult = true
			m.keepSelectedVisible()
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
	if m.launching {
		status += " · preparing viewer"
	}
	if !m.lastUpdate.IsZero() {
		status += " · updated " + m.lastUpdate.Format("15:04:05")
	}
	if m.refreshErr != nil {
		status += " · refresh failed: " + m.refreshErr.Error()
	}
	if m.openErr != nil {
		status += " · open failed: " + m.openErr.Error()
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
		rendered, _ := m.renderedStreamList(width)
		body = strings.Split(rendered, "\n")
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
	lines = append(lines, mutedStyle.Render("↑/↓ select · enter open · pgup/pgdn scroll · r refresh · q quit"))
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
	rendered, _ := m.renderedStreamList(width)
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

func (m liveModeModel) renderedStreamList(width int) (string, []streamLineSpan) {
	if m.display == "table" {
		return renderStreamTableWithSelection(m.streams, width, m.selected)
	}
	return renderStreamCardsWithSelection(m.streams, width-2, m.selected)
}

func (m *liveModeModel) moveSelection(delta int) {
	if len(m.streams) == 0 {
		return
	}
	m.selectStream(m.selected + delta)
}

func (m *liveModeModel) selectStream(index int) {
	if len(m.streams) == 0 {
		m.selected = -1
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.streams) {
		index = len(m.streams) - 1
	}
	m.selected = index
	m.openErr = nil
	m.keepSelectedVisible()
}

func (m liveModeModel) openSelected() tea.Cmd {
	if m.selected < 0 || m.selected >= len(m.streams) || m.launching {
		return nil
	}
	channel := m.streams[m.selected].Login
	if m.viewer.Viewer == "streamlink" {
		return func() tea.Msg {
			plan, err := prepareLaunch(m.ctx, channel, m.viewer)
			return viewerPreparedMsg{plan: plan, err: err}
		}
	}
	return func() tea.Msg {
		return streamOpenResultMsg{err: launchChannel(m.ctx, channel, m.viewer)}
	}
}

func reconcileSelectedStream(previous []Stream, selected int, refreshed []Stream) int {
	if len(refreshed) == 0 {
		return -1
	}
	if selected < 0 || selected >= len(previous) {
		return 0
	}
	login := previous[selected].Login
	for i, stream := range refreshed {
		if strings.EqualFold(stream.Login, login) {
			return i
		}
	}
	if selected >= len(refreshed) {
		return len(refreshed) - 1
	}
	return selected
}

func (m *liveModeModel) keepSelectedVisible() {
	if m.selected < 0 || m.selected >= len(m.streams) {
		return
	}
	width := m.width
	if width < 20 {
		width = 20
	}
	if width > 160 {
		width = 160
	}
	_, spans := m.renderedStreamList(width)
	if m.selected >= len(spans) {
		return
	}
	span := spans[m.selected]
	if span.start < m.scroll {
		m.scroll = span.start
	} else if span.end > m.scroll+m.bodyHeight() {
		m.scroll = span.end - m.bodyHeight()
	}
	m.clampScroll()
}

func (m *liveModeModel) clampScroll() {
	if m.scroll < 0 {
		m.scroll = 0
	}
	if max := m.maxScroll(); m.scroll > max {
		m.scroll = max
	}
}
