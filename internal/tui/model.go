package tui

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

const (
	wideLayoutMin       = 100
	paneFrameWidth      = 4
	paneBorderHeight    = 2
	dashboardChromeRows = 3
	detailMetadataRows  = 10
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

type Model struct {
	ctx           context.Context
	selection     app.Selection
	pullRequests  []app.PullRequest
	fetchedAt     time.Time
	loading       bool
	refreshing    bool
	stale         bool
	refreshErr    error
	cacheWarning  error
	selected      int
	checked       map[string]bool
	width         int
	height        int
	showHelp      bool
	statusMessage string
	generation    uint64
	deps          Dependencies
}

func NewModel(
	ctx context.Context,
	selection app.Selection,
	cached *app.Snapshot,
	cacheWarning error,
	deps Dependencies,
) Model {
	model := Model{
		ctx:          ctx,
		selection:    selection,
		loading:      cached == nil,
		refreshing:   true,
		cacheWarning: cacheWarning,
		checked:      make(map[string]bool),
		deps:         deps,
	}
	if cached != nil {
		model.pullRequests = slices.Clone(cached.PullRequests)
		model.fetchedAt = cached.FetchedAt
	}
	return model
}

func (m Model) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Model) resize(width, height int) {
	m.width = max(0, width)
	m.height = max(0, height)
}

func (m Model) chromeRows() int {
	return dashboardChromeRows + max(0, footerLineCount(m.footerText(), m.width)-1)
}

func paneDimensions(width, height, chromeRows int) (listWidth, detailWidth, listHeight, detailHeight int) {
	paneHeight := max(0, height-chromeRows)
	if width >= wideLayoutMin {
		availableWidth := max(0, width-1)
		listWidth = availableWidth * 60 / 100
		detailWidth = availableWidth - listWidth
		return listWidth, detailWidth, paneHeight, min(paneHeight, detailMetadataRows+paneBorderHeight)
	}

	detailHeight = min(paneHeight/2, detailMetadataRows+paneBorderHeight)
	listHeight = paneHeight - detailHeight
	return max(0, width), max(0, width), listHeight, detailHeight
}
