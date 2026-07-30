package tui

import (
	"context"
	"slices"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kenyounot123/prq/internal/app"
)

const (
	wideLayoutMin       = 100
	paneFrameWidth      = 4
	paneBorderHeight    = 2
	dashboardChromeRows = 3
	detailMetadataRows  = 9
)

type Dependencies struct {
	Refresh func(context.Context, app.Selection) (app.RefreshResult, error)
	Copy    func(context.Context, string) error
	Open    func(context.Context, string) error
	Now     func() time.Time
}

type refreshDoneMsg struct {
	result app.RefreshResult
	err    error
}

type actionDoneMsg struct {
	generation uint64
	action     string
	success    string
	err        error
}

type focusMode int

const (
	listFocus focusMode = iota
	detailFocus
)

type Model struct {
	ctx            context.Context
	selection      app.Selection
	pullRequests   []app.PullRequest
	fetchedAt      time.Time
	loading        bool
	refreshing     bool
	stale          bool
	refreshErr     error
	cacheWarning   error
	selected       int
	focus          focusMode
	width          int
	height         int
	showHelp       bool
	statusMessage  string
	generation     uint64
	detailViewport viewport.Model
	deps           Dependencies
}

func NewModel(
	ctx context.Context,
	selection app.Selection,
	cached *app.Snapshot,
	cacheWarning error,
	deps Dependencies,
) Model {
	model := Model{
		ctx:            ctx,
		selection:      selection,
		loading:        cached == nil,
		refreshing:     true,
		cacheWarning:   cacheWarning,
		detailViewport: viewport.New(),
		deps:           deps,
	}
	if cached != nil {
		model.pullRequests = slices.Clone(cached.PullRequests)
		model.fetchedAt = cached.FetchedAt
	}
	model.syncDetailViewport()
	return model
}

func (m Model) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Model) syncDetailViewport() {
	content := ""
	if m.selected < 0 || m.selected >= len(m.pullRequests) {
	} else {
		content = m.pullRequests[m.selected].Body
	}
	if m.detailViewport.Width() > 0 {
		content = ansi.Wrap(content, m.detailViewport.Width(), " ")
	}
	m.detailViewport.SetContent(content)
	m.detailViewport.GotoTop()
}

func (m *Model) resize(width, height int) {
	m.width = max(0, width)
	m.height = max(0, height)
	_, detailWidth, _, detailHeight := paneDimensions(m.width, m.height, m.chromeRows())
	m.detailViewport.SetWidth(max(0, detailWidth-paneFrameWidth))
	m.detailViewport.SetHeight(max(0, detailHeight-paneBorderHeight-detailMetadataRows))
	m.syncDetailViewport()
}

func (m *Model) resizeDetailViewportHeight() {
	_, _, _, detailHeight := paneDimensions(m.width, m.height, m.chromeRows())
	offset := m.detailViewport.YOffset()
	m.detailViewport.SetHeight(max(0, detailHeight-paneBorderHeight-detailMetadataRows))
	m.detailViewport.SetYOffset(offset)
}

func (m Model) chromeRows() int {
	return dashboardChromeRows + max(0, footerLineCount(m.footerText(), m.width)-1)
}

func paneDimensions(width, height, chromeRows int) (listWidth, detailWidth, listHeight, detailHeight int) {
	paneHeight := max(0, height-chromeRows)
	if width >= wideLayoutMin {
		availableWidth := max(0, width-1)
		listWidth = availableWidth * 40 / 100
		detailWidth = availableWidth - listWidth
		return listWidth, detailWidth, paneHeight, paneHeight
	}

	listHeight = paneHeight / 2
	detailHeight = paneHeight - listHeight
	return max(0, width), max(0, width), listHeight, detailHeight
}
