package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/blacknon/tvxterm"
)

type pane struct {
	name    string
	term    *tvxterm.View
	backend *tvxterm.PTYBackend
}

type muxPage struct {
	name   string
	panes  []*pane
	focus  *pane
	layout *layoutNode
}

type layoutNode struct {
	pane      *pane
	direction int
	children  []*layoutNode
}

func main() {
	app := tview.NewApplication()
	var stopOnce sync.Once

	status := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	status.SetBorder(true).SetTitle("Mux")

	uiPages := tview.NewPages()
	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(uiPages, 0, 1, true).
		AddItem(status, 3, 0, false)

	var sessionPages []*muxPage
	var currentPage *muxPage
	nextPageID := 1
	nextPaneID := 1

	var updateStatus func(string)
	var refreshMainPage func()
	var showPageList func()

	findPageIndex := func(target *muxPage) int {
		for i, p := range sessionPages {
			if p == target {
				return i
			}
		}
		return -1
	}

	newPane := func(page *muxPage) (*pane, error) {
		name := fmt.Sprintf("pane-%d", nextPaneID)
		nextPaneID++

		cmd := exec.Command(shell())
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")

		backend, err := tvxterm.NewPTYBackend(cmd, 80, 24)
		if err != nil {
			return nil, err
		}

		p := &pane{
			name:    name,
			term:    tvxterm.New(app),
			backend: backend,
		}
		p.term.SetBorder(true).SetTitle(name)
		p.term.SetBackendExitHandler(func(_ *tvxterm.View, err error) {
			app.QueueUpdateDraw(func() {
				if err != nil {
					status.SetText(fmt.Sprintf("[red]%s pane closed[-]: %v", name, err))
				} else {
					status.SetText(fmt.Sprintf("[red]%s pane closed[-]", name))
				}
				_ = p.term.Close()
				removePaneFromPage(page, p, &sessionPages, &currentPage, &stopOnce, app)
				uiPages.RemovePage("page-list")
				if currentPage != nil {
					refreshMainPage()
				}
			})
		})
		p.term.Attach(backend)
		return p, nil
	}

	createPage := func() (*muxPage, error) {
		page := &muxPage{name: fmt.Sprintf("page-%d", nextPageID)}
		nextPageID++

		firstPane, err := newPane(page)
		if err != nil {
			return nil, err
		}
		page.panes = []*pane{firstPane}
		page.focus = firstPane
		page.layout = &layoutNode{pane: firstPane}
		sessionPages = append(sessionPages, page)
		currentPage = page
		return page, nil
	}

	updateStatus = func(message string) {
		if currentPage == nil || currentPage.focus == nil {
			status.SetText("[yellow]Ctrl+Q[-]: quit")
			return
		}
		offset, rows := currentPage.focus.term.ScrollbackStatus()
		base := fmt.Sprintf(
			"[yellow]Ctrl+A c[-]: new page  [yellow]Ctrl+A %%[-]: vertical split  [yellow]Ctrl+A \"[-]: horizontal split  [yellow]Ctrl+A o[-]: next pane  [yellow]Ctrl+A n/p[-]: next/prev page  [yellow]Ctrl+A w[-]: page list  [yellow]PageUp/PageDown[-]: scroll  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit  [green]page[-]: %s  [green]pane[-]: %s  [blue]scrollback[-]: %d/%d  [blue]pages[-]: %d  [blue]panes[-]: %d",
			currentPage.name, currentPage.focus.name, offset, rows, len(sessionPages), len(currentPage.panes),
		)
		if message != "" {
			base += "  " + message
		}
		status.SetText(base)
	}

	buildMainPage := func() tview.Primitive {
		if currentPage == nil || currentPage.layout == nil {
			return tview.NewBox()
		}
		return tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(currentPage.layout.primitive(), 0, 1, true)
	}

	refreshMainPage = func() {
		uiPages.RemovePage("main")
		uiPages.AddPage("main", buildMainPage(), true, true)
		uiPages.SwitchToPage("main")
		if currentPage != nil && currentPage.focus != nil {
			app.SetFocus(currentPage.focus.term)
		}
		updateStatus("")
	}

	switchPage := func(index int) {
		if len(sessionPages) == 0 {
			return
		}
		if index < 0 {
			index = len(sessionPages) - 1
		}
		if index >= len(sessionPages) {
			index = 0
		}
		currentPage = sessionPages[index]
		refreshMainPage()
	}

	cyclePane := func() {
		if currentPage == nil || len(currentPage.panes) == 0 {
			return
		}
		current := 0
		for i, p := range currentPage.panes {
			if p == currentPage.focus {
				current = i
				break
			}
		}
		currentPage.focus = currentPage.panes[(current+1)%len(currentPage.panes)]
		refreshMainPage()
	}

	splitFocused := func(direction int) error {
		if currentPage == nil || currentPage.focus == nil {
			return fmt.Errorf("no active page")
		}
		p, err := newPane(currentPage)
		if err != nil {
			return err
		}
		if !currentPage.layout.split(currentPage.focus, p, direction) {
			return fmt.Errorf("focused pane not found")
		}
		currentPage.panes = append(currentPage.panes, p)
		currentPage.focus = p
		refreshMainPage()
		return nil
	}

	hidePageList := func() {
		uiPages.HidePage("page-list")
		uiPages.SwitchToPage("main")
		if currentPage != nil && currentPage.focus != nil {
			app.SetFocus(currentPage.focus.term)
		}
		updateStatus("")
	}

	showPageList = func() {
		list := tview.NewList().ShowSecondaryText(false)
		list.SetBorder(true).SetTitle("Mux Pages")
		for i, page := range sessionPages {
			index := i
			label := fmt.Sprintf("%s  panes=%d", page.name, len(page.panes))
			list.AddItem(label, "", 0, func() {
				currentPage = sessionPages[index]
				hidePageList()
			})
		}
		list.SetDoneFunc(func() {
			hidePageList()
		})
		uiPages.RemovePage("page-list")
		uiPages.AddPage("page-list", centered(list, 60, minInt(len(sessionPages)+2, 12)), true, true)
		uiPages.SwitchToPage("page-list")
		app.SetFocus(list)
		updateStatus("[gray]page list[-]")
	}

	if _, err := createPage(); err != nil {
		log.Fatal(err)
	}
	refreshMainPage()

	ctrlA := false
	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if event == nil || currentPage == nil {
			return event, action
		}
		switch action {
		case tview.MouseScrollUp, tview.MouseScrollDown:
			x, y := event.Position()
			for _, p := range currentPage.panes {
				if p.term.InRect(x, y) {
					offset, _ := p.term.ScrollbackStatus()
					if offset == 0 {
						return event, action
					}
					if action == tview.MouseScrollUp {
						p.term.ScrollbackUp(3)
					} else {
						p.term.ScrollbackDown(3)
					}
					if currentPage.focus == p {
						updateStatus("")
					}
					return nil, action
				}
			}
		}
		return event, action
	})

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			return cloneKeyEvent(event)
		}
		switch {
		case event.Key() == tcell.KeyCtrlQ:
			app.Stop()
			return nil
		case event.Key() == tcell.KeyCtrlA:
			ctrlA = true
			updateStatus("[gray]prefix[-]: Ctrl+A")
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && (event.Rune() == 'c' || event.Rune() == 'C'):
			ctrlA = false
			if _, err := createPage(); err != nil {
				updateStatus(fmt.Sprintf("[red]new page failed[-]: %v", err))
			} else {
				refreshMainPage()
			}
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && event.Rune() == '%':
			ctrlA = false
			if err := splitFocused(tview.FlexColumn); err != nil {
				updateStatus(fmt.Sprintf("[red]split failed[-]: %v", err))
			}
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && event.Rune() == '"':
			ctrlA = false
			if err := splitFocused(tview.FlexRow); err != nil {
				updateStatus(fmt.Sprintf("[red]split failed[-]: %v", err))
			}
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && (event.Rune() == 'o' || event.Rune() == 'O'):
			ctrlA = false
			cyclePane()
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && (event.Rune() == 'n' || event.Rune() == 'N'):
			ctrlA = false
			switchPage(findPageIndex(currentPage) + 1)
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && (event.Rune() == 'p' || event.Rune() == 'P'):
			ctrlA = false
			switchPage(findPageIndex(currentPage) - 1)
			return nil
		case ctrlA && event.Key() == tcell.KeyRune && (event.Rune() == 'w' || event.Rune() == 'W'):
			ctrlA = false
			showPageList()
			return nil
		case event.Key() == tcell.KeyPgUp:
			ctrlA = false
			if currentPage != nil && currentPage.focus != nil {
				currentPage.focus.term.ScrollbackPageUp()
				updateStatus("")
			}
			return nil
		case event.Key() == tcell.KeyPgDn:
			ctrlA = false
			if currentPage != nil && currentPage.focus != nil {
				currentPage.focus.term.ScrollbackPageDown()
				updateStatus("")
			}
			return nil
		default:
			ctrlA = false
			return event
		}
	})

	if err := app.SetRoot(root, true).SetFocus(currentPage.focus.term).EnableMouse(true).EnablePaste(true).Run(); err != nil {
		log.Fatal(err)
	}
	for _, page := range sessionPages {
		for _, pane := range page.panes {
			_ = pane.backend.Close()
		}
	}
}

func removePaneFromPage(page *muxPage, target *pane, pages *[]*muxPage, current **muxPage, stopOnce *sync.Once, app *tview.Application) {
	if page == nil {
		return
	}
	index := -1
	for i, p := range page.panes {
		if p == target {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	page.panes = append(page.panes[:index], page.panes[index+1:]...)
	if len(page.panes) == 0 {
		page.layout = nil
		for i, candidate := range *pages {
			if candidate == page {
				*pages = append((*pages)[:i], (*pages)[i+1:]...)
				break
			}
		}
		if len(*pages) == 0 {
			stopOnce.Do(func() {
				app.Stop()
			})
			*current = nil
			return
		}
		if *current == page {
			*current = (*pages)[0]
		}
		return
	}
	_ = page.layout.remove(target)
	if page.focus == target {
		if index >= len(page.panes) {
			index = len(page.panes) - 1
		}
		page.focus = page.panes[index]
	}
}

func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(
			tview.NewFlex().
				SetDirection(tview.FlexRow).
				AddItem(nil, 0, 1, false).
				AddItem(p, height, 1, true).
				AddItem(nil, 0, 1, false),
			width, 1, true,
		).
		AddItem(nil, 0, 1, false)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cloneKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	return tcell.NewEventKey(event.Key(), event.Rune(), event.Modifiers())
}

func (n *layoutNode) primitive() tview.Primitive {
	if n == nil {
		return tview.NewBox()
	}
	if n.pane != nil {
		return n.pane.term
	}

	flex := tview.NewFlex().SetDirection(n.direction)
	for _, child := range n.children {
		flex.AddItem(child.primitive(), 0, 1, false)
	}
	return flex
}

func (n *layoutNode) split(target *pane, next *pane, direction int) bool {
	if n == nil {
		return false
	}
	if n.pane == target {
		current := n.pane
		n.pane = nil
		n.direction = direction
		n.children = []*layoutNode{
			{pane: current},
			{pane: next},
		}
		return true
	}
	for _, child := range n.children {
		if child.split(target, next, direction) {
			return true
		}
	}
	return false
}

func (n *layoutNode) remove(target *pane) bool {
	if n == nil {
		return false
	}
	if n.pane == target {
		n.pane = nil
		return true
	}
	for i, child := range n.children {
		if child.remove(target) {
			if child.pane == nil && len(child.children) == 0 {
				n.children = append(n.children[:i], n.children[i+1:]...)
			}
			n.compact()
			return true
		}
	}
	return false
}

func (n *layoutNode) compact() {
	if n == nil {
		return
	}
	switch len(n.children) {
	case 0:
		n.pane = nil
		n.direction = 0
	case 1:
		only := n.children[0]
		n.pane = only.pane
		n.direction = only.direction
		n.children = only.children
	}
}

func shell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}
