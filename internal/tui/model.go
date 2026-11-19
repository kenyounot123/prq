package tui

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kenyounot123/prq/internal/app"
)

const (
	wideLayoutMin   = 100
	pageMargin      = 1
	summaryRows     = 1
	spacerRows      = 1
	headerRows      = summaryRows + spacerRows
	wideListPercent = 60
	stackedRuleRows = 1
)

type statusKind int

const (
	statusSuccess statusKind = iota
	statusWarning
	statusError
)

type statusLine struct {
	text string
	kind statusKind
}

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
	ctx          context.Context
	selection    app.Selection
	pullRequests []app.PullRequest
	fetchedAt    time.Time
	loading      bool
	refreshing   bool
	stale        bool
	refreshErr   error
	cacheWarning error
	selected     int
	checked      map[string]bool
	width        int
	height       int
	showHelp     bool
	status       statusLine
	collapsed    map[app.StatusGroup]bool
	scrollTop    int
	generation   uint64
	deps         Dependencies
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
		collapsed:    map[app.StatusGroup]bool{app.StatusDraft: true},
		deps:         deps,
	}
	if cached != nil {
		model.pullRequests = slices.Clone(cached.PullRequests)
		model.fetchedAt = cached.FetchedAt
	}
	model.selected = model.firstNavigable(model.rows())
	return model
}

func (m Model) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m *Model) resize(width, height int) {
	m.width = max(0, width)
	m.height = max(0, height)
}

func (m Model) contentWidth() int {
	return max(0, m.width-2*pageMargin)
}

func (m Model) chromeRows() int {
	rows := headerRows + len(m.footerLines())
	if len(m.checked) > 0 {
		rows++
	}
	return rows
}

func (m Model) bodyHeight() int {
	return max(0, m.height-m.chromeRows())
}

type bodyLayout struct {
	wide                     bool
	listWidth, detailWidth   int
	listHeight, detailHeight int
}

func layoutBody(width, height int) bodyLayout {
	if width >= wideLayoutMin {
		available := max(0, width-columnSeparatorWidth)
		listWidth := available * wideListPercent / 100
		return bodyLayout{
			wide:         true,
			listWidth:    listWidth,
			detailWidth:  available - listWidth,
			listHeight:   height,
			detailHeight: height,
		}
	}
	stacked := max(0, height-stackedRuleRows)
	detailHeight := stacked / 2
	return bodyLayout{
		listWidth:    width,
		detailWidth:  width,
		listHeight:   stacked - detailHeight,
		detailHeight: detailHeight,
	}
}
