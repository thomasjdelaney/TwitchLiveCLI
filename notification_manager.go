package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

type followedChannelsMsg struct {
	channels []Followed
	err      error
}

type notificationManagerTickMsg struct{}

type notificationManagerModel struct {
	ctx         context.Context
	api         *Twitch
	prefs       preferences
	width       int
	height      int
	palette     colorPalette
	followed    []Followed
	filter      string
	filtering   bool
	selected    int
	scroll      int
	loading     bool
	spinner     spinner.Model
	loadErr     error
	saveErr     error
	deliveryErr error
	standalone  bool
}

func runNotificationManager(ctx context.Context, api *Twitch, prefs preferences) error {
	return runLiveModeTUI(ctx, api, liveModeOptions{
		interval: prefs.RefreshInterval, display: prefs.Display, sortBy: prefs.SortBy,
		sortOrder: prefs.SortOrder, viewer: viewerOptionsFromPreferences(prefs),
		preferences: prefs, initialScreen: notificationManagerScreen,
	})
}

func newNotificationManagerModel(ctx context.Context, api *Twitch, prefs preferences, width, height int, palette colorPalette) notificationManagerModel {
	return notificationManagerModel{
		ctx: ctx, api: api, prefs: prefs, width: width, height: height,
		palette: palette, spinner: newLoadingSpinner(palette),
	}
}

func (m notificationManagerModel) Init() tea.Cmd {
	return tea.Batch(m.fetchFollowed(), m.nextSpinnerTick())
}

func (m notificationManagerModel) open() (notificationManagerModel, tea.Cmd) {
	m.loading = true
	m.loadErr = nil
	m.saveErr = nil
	m.filter = ""
	m.filtering = false
	m.selected = 0
	m.scroll = 0
	if len(m.spinner.Spinner.Frames) == 0 {
		m.spinner = newLoadingSpinner(m.palette)
	}
	return m, tea.Batch(m.fetchFollowed(), m.nextSpinnerTick())
}

func (m notificationManagerModel) fetchFollowed() tea.Cmd {
	return func() tea.Msg {
		channels, err := m.api.FollowedChannels(m.ctx)
		return followedChannelsMsg{channels: channels, err: err}
	}
}

func (m notificationManagerModel) nextSpinnerTick() tea.Cmd {
	return tea.Tick(m.spinner.Spinner.FPS, func(time.Time) tea.Msg { return notificationManagerTickMsg{} })
}

func (m notificationManagerModel) Update(message tea.Msg) (notificationManagerModel, tea.Cmd, bool) {
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampSelection()
	case tea.BackgroundColorMsg:
		if msg.IsDark() {
			m.palette = darkPalette()
		} else {
			m.palette = lightPalette()
		}
		m.spinner.Style = m.palette.titleStyle()
	case notificationManagerTickMsg:
		if m.loading {
			m.spinner, _ = m.spinner.Update(m.spinner.Tick())
			return m, m.nextSpinnerTick(), false
		}
	case followedChannelsMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
		} else {
			m.followed = msg.channels
			sortFollowedChannels(m.followed)
			m.loadErr = nil
		}
		m.clampSelection()
	case notificationResultMsg:
		m.deliveryErr = msg.err
	}
	return m, nil, false
}

func (m notificationManagerModel) updateKey(msg tea.KeyPressMsg) (notificationManagerModel, tea.Cmd, bool) {
	key := msg.String()
	if m.filtering {
		m.updateFilterKey(msg, key)
		return m, nil, false
	}
	switch key {
	case "esc":
		return m, nil, true
	case "up", "k":
		m.selected--
	case "down", "j":
		m.selected++
	case "home":
		m.selected = 0
	case "end":
		m.selected = len(m.filteredChannels()) - 1
	case "pgup":
		m.selected -= m.layout().visibleRows(m.height)
	case "pgdown":
		m.selected += m.layout().visibleRows(m.height)
	case "backspace", "delete":
		filterRunes := []rune(m.filter)
		if len(filterRunes) > 0 {
			m.filter = string(filterRunes[:len(filterRunes)-1])
		}
		m.selected = 0
	case "s":
		m.prefs.NotifyOnStartup = !m.prefs.NotifyOnStartup
		m.saveErr = savePreferences(m.prefs)
	case "r":
		if m.saveErr != nil {
			m.saveErr = savePreferences(m.prefs)
			return m, nil, false
		}
		m.loading = true
		m.loadErr = nil
		return m, tea.Batch(m.fetchFollowed(), m.nextSpinnerTick()), false
	case "space", "enter":
		m.toggleChannel()
	case "/":
		m.filtering = true
	case "q":
		return m, nil, true
	}
	m.clampSelection()
	return m, nil, false
}

func (m *notificationManagerModel) updateFilterKey(msg tea.KeyPressMsg, key string) {
	switch key {
	case "enter", "esc":
		m.filtering = false
	case "backspace", "delete":
		filterRunes := []rune(m.filter)
		if len(filterRunes) > 0 {
			m.filter = string(filterRunes[:len(filterRunes)-1])
		}
		m.selected = 0
	case "up":
		m.selected--
	case "down":
		m.selected++
	case "home":
		m.selected = 0
	case "end":
		m.selected = len(m.filteredChannels()) - 1
	case "pgup":
		m.selected -= m.layout().visibleRows(m.height)
	case "pgdown":
		m.selected += m.layout().visibleRows(m.height)
	default:
		text := msg.Text
		if msg.Code == tea.KeySpace {
			text = " "
		}
		if text != "" && isPrintable(text) {
			m.filter += text
			m.selected = 0
		}
	}
	m.clampSelection()
}

func isPrintable(text string) bool {
	for _, r := range text {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func (m *notificationManagerModel) toggleChannel() {
	channels := m.filteredChannels()
	if m.selected < 0 || m.selected >= len(channels) {
		return
	}
	id := channels[m.selected].ID
	existingIDs := append([]string(nil), m.prefs.NotificationChannels...)
	selected := make(map[string]bool, len(existingIDs))
	for _, existing := range existingIDs {
		selected[existing] = true
	}
	if selected[id] {
		delete(selected, id)
	} else {
		selected[id] = true
	}
	m.prefs.NotificationChannels = m.prefs.NotificationChannels[:0]
	known := make(map[string]bool, len(m.followed))
	for _, channel := range m.followed {
		known[channel.ID] = true
		if selected[channel.ID] {
			m.prefs.NotificationChannels = append(m.prefs.NotificationChannels, channel.ID)
		}
	}
	for _, existing := range existingIDs {
		if selected[existing] && !known[existing] {
			m.prefs.NotificationChannels = append(m.prefs.NotificationChannels, existing)
		}
	}
	m.saveErr = savePreferences(m.prefs)
}

func (m notificationManagerModel) filteredChannels() []Followed {
	filter := strings.ToLower(strings.TrimSpace(m.filter))
	if filter == "" {
		return m.followed
	}
	filtered := make([]Followed, 0)
	for _, channel := range m.followed {
		if strings.Contains(strings.ToLower(channel.Name), filter) || strings.Contains(strings.ToLower(channel.Login), filter) {
			filtered = append(filtered, channel)
		}
	}
	return filtered
}

func sortFollowedChannels(channels []Followed) {
	sort.SliceStable(channels, func(i, j int) bool {
		left := strings.ToLower(channels[i].Name)
		right := strings.ToLower(channels[j].Name)
		if left == right {
			left = strings.ToLower(channels[i].Login)
			right = strings.ToLower(channels[j].Login)
		}
		return left < right
	})
}

type notificationManagerLayout struct {
	width  int
	header []string
	footer []string
}

func (m notificationManagerModel) layout() notificationManagerLayout {
	width := m.viewWidth()
	startupStatus := "Startup alerts: OFF · press s to enable"
	if m.prefs.NotifyOnStartup {
		startupStatus = "Startup alerts: ON · press s to disable"
	}
	filterPrompt := "Filter: " + m.filter + " (press / to edit filter)"
	if m.filtering {
		filterPrompt = "Filter: " + m.filter + "▏"
	}
	header := []string{m.palette.titleStyle().Render("  DESKTOP NOTIFICATIONS"), strings.Repeat("─", width), startupStatus, "", filterPrompt, ""}
	footer := make([]string, 0, 5)
	if m.saveErr != nil && !m.loading {
		footer = append(footer, "", "Could not save preferences: "+m.saveErr.Error()+". Your change remains selected in this session.")
	}
	if m.deliveryErr != nil {
		footer = append(footer, "", "Notification delivery failed: "+m.deliveryErr.Error())
	}
	leaveHelp := "esc back"
	if m.standalone {
		leaveHelp = "esc exit"
	}
	retryHelp := "r reload"
	if m.saveErr != nil {
		retryHelp = "r retry save"
	}
	help := "↑/k up · ↓/j down · home/end · pgup/pgdn · space/enter toggle · / filter · s startup · " + retryHelp + " · " + leaveHelp + " · q quit"
	if m.filtering {
		help = "Type to filter (including spaces) · Backspace delete · Enter/Esc finish · ↑/↓ navigate"
	}
	footer = append(footer, "", help)
	return notificationManagerLayout{width: width, header: header, footer: footer}
}

func (l notificationManagerLayout) visibleRows(height int) int {
	used := wrappedLineCount(l.header, l.width) + wrappedLineCount(l.footer, l.width)
	rows := height - used
	if rows < 1 {
		return 1
	}
	return rows
}

func (m notificationManagerModel) viewWidth() int {
	if m.width < 30 {
		return 30
	}
	if m.width > 160 {
		return 160
	}
	return m.width
}

func wrappedLineCount(lines []string, width int) int {
	count := 0
	for _, line := range lines {
		count += strings.Count(wrapText(line, width), "\n") + 1
	}
	return count
}

func (m *notificationManagerModel) clampSelection() {
	count := len(m.filteredChannels())
	if count == 0 {
		m.selected = 0
		m.scroll = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= count {
		m.selected = count - 1
	}
	rows := m.layout().visibleRows(m.height)
	if m.selected < m.scroll {
		m.scroll = m.selected
	}
	if m.selected >= m.scroll+rows {
		m.scroll = m.selected - rows + 1
	}
}

func (m notificationManagerModel) View() tea.View {
	layout := m.layout()
	lines := append([]string(nil), layout.header...)
	channels := m.filteredChannels()
	switch {
	case m.loading:
		lines = append(lines, m.spinner.View()+" Loading followed channels…")
	case m.loadErr != nil:
		lines = append(lines, "Could not load followed channels: "+m.loadErr.Error())
	case len(m.followed) == 0:
		lines = append(lines, "You don't follow any channels.")
	case len(channels) == 0:
		lines = append(lines, fmt.Sprintf("No followed channels match %q.", m.filter))
	default:
		end := m.scroll + layout.visibleRows(m.height)
		if end > len(channels) {
			end = len(channels)
		}
		selected := make(map[string]bool, len(m.prefs.NotificationChannels))
		for _, id := range m.prefs.NotificationChannels {
			selected[id] = true
		}
		for i, channel := range channels[m.scroll:end] {
			marker := "  "
			if i+m.scroll == m.selected {
				marker = "> "
			}
			check := "[ ]"
			if selected[channel.ID] {
				check = "[x]"
			}
			label := channel.Name
			if label == "" {
				label = channel.Login
			}
			labelWidth := layout.width - len([]rune(marker+check+" "))
			if labelWidth < 1 {
				labelWidth = 1
			}
			labelRunes := []rune(label)
			if len(labelRunes) > labelWidth {
				label = string(labelRunes[:labelWidth])
			}
			lines = append(lines, fmt.Sprintf("%s%s %s", marker, check, label))
		}
	}
	lines = append(lines, layout.footer...)
	for i := range lines {
		lines[i] = wrapText(lines[i], layout.width)
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
