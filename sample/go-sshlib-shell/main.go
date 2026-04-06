package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	sshlib "github.com/blacknon/go-sshlib"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/crypto/ssh"

	"github.com/blacknon/tvxterm"
)

type sshConfig struct {
	host          string
	port          string
	user          string
	password      string
	keyPath       string
	keyPassphrase string
}

type sshPane struct {
	name    string
	term    *tvxterm.View
	backend *tvxterm.StreamBackend
}

type sshPage struct {
	name   string
	panes  []*sshPane
	focus  *sshPane
	layout *layoutNode
}

type layoutNode struct {
	pane      *sshPane
	direction int
	children  []*layoutNode
}

func main() {
	cfg := parseConfig()
	authMethods := buildAuthMethods(cfg)

	app := tview.NewApplication()
	var stopOnce sync.Once

	status := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	status.SetBorder(true).SetTitle("SSH")

	uiPages := tview.NewPages()
	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(uiPages, 0, 1, true).
		AddItem(status, 3, 0, false)

	var sessionPages []*sshPage
	var currentPage *sshPage
	nextPageID := 1
	nextPaneID := 1

	var updateStatus func(string)
	var refreshMainPage func()
	var showPageList func()

	findPageIndex := func(target *sshPage) int {
		for i, p := range sessionPages {
			if p == target {
				return i
			}
		}
		return -1
	}

	newPane := func(page *sshPage) (*sshPane, error) {
		name := fmt.Sprintf("ssh-%d", nextPaneID)
		nextPaneID++

		backend, err := newSSHBackend(cfg, authMethods)
		if err != nil {
			return nil, err
		}

		p := &sshPane{
			name:    name,
			term:    tvxterm.New(app),
			backend: backend,
		}
		p.term.SetBorder(true).SetTitle(fmt.Sprintf("%s [%s@%s]", name, cfg.user, cfg.host))
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

	createPage := func() (*sshPage, error) {
		page := &sshPage{name: fmt.Sprintf("page-%d", nextPageID)}
		nextPageID++

		firstPane, err := newPane(page)
		if err != nil {
			return nil, err
		}
		page.panes = []*sshPane{firstPane}
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
		list.SetBorder(true).SetTitle("SSH Pages")
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

func removePaneFromPage(page *sshPage, target *sshPane, pages *[]*sshPage, current **sshPage, stopOnce *sync.Once, app *tview.Application) {
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

func parseConfig() sshConfig {
	cfg := sshConfig{}
	flag.StringVar(&cfg.host, "host", envOrDefault("TVXTERM_SSH_HOST", ""), "SSH host")
	flag.StringVar(&cfg.port, "port", envOrDefault("TVXTERM_SSH_PORT", "22"), "SSH port")
	flag.StringVar(&cfg.user, "user", envOrDefault("TVXTERM_SSH_USER", ""), "SSH user")
	flag.StringVar(&cfg.password, "password", envOrDefault("TVXTERM_SSH_PASSWORD", ""), "SSH password")
	flag.StringVar(&cfg.keyPath, "key-path", envOrDefault("TVXTERM_SSH_KEY_PATH", ""), "SSH private key path")
	flag.StringVar(&cfg.keyPassphrase, "key-passphrase", envOrDefault("TVXTERM_SSH_KEY_PASSPHRASE", ""), "SSH private key passphrase")
	flag.Parse()

	if cfg.host == "" {
		log.Fatal("host is required: use -host or TVXTERM_SSH_HOST")
	}
	if cfg.user == "" {
		log.Fatal("user is required: use -user or TVXTERM_SSH_USER")
	}
	return cfg
}

func buildAuthMethods(cfg sshConfig) []ssh.AuthMethod {
	authMethods := make([]ssh.AuthMethod, 0, 2)
	if cfg.keyPath != "" {
		authMethod, err := sshlib.CreateAuthMethodPublicKey(cfg.keyPath, cfg.keyPassphrase)
		if err != nil {
			log.Fatalf("failed to load private key from %s: %v", cfg.keyPath, err)
		}
		authMethods = append(authMethods, authMethod)
	}
	if cfg.password != "" {
		authMethods = append(authMethods, sshlib.CreateAuthMethodPassword(cfg.password))
	}
	if len(authMethods) == 0 {
		log.Fatal("set -key-path or -password, or use TVXTERM_SSH_KEY_PATH / TVXTERM_SSH_PASSWORD")
	}
	return authMethods
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

func (n *layoutNode) split(target *sshPane, next *sshPane, direction int) bool {
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

func (n *layoutNode) remove(target *sshPane) bool {
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

func newSSHBackend(cfg sshConfig, authMethods []ssh.AuthMethod) (*tvxterm.StreamBackend, error) {
	con := &sshlib.Connect{
		ForwardX11:   false,
		ForwardAgent: false,
	}

	if err := con.CreateClient(cfg.host, cfg.port, cfg.user, authMethods); err != nil {
		return nil, err
	}

	session, err := con.CreateSession()
	if err != nil {
		_ = con.Client.Close()
		return nil, err
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		_ = con.Client.Close()
		return nil, err
	}

	outputReader, outputWriter := io.Pipe()
	session.Stdout = outputWriter
	session.Stderr = outputWriter

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		_ = outputWriter.Close()
		_ = session.Close()
		_ = con.Client.Close()
		return nil, err
	}

	if err := session.Shell(); err != nil {
		_ = outputWriter.Close()
		_ = session.Close()
		_ = con.Client.Close()
		return nil, err
	}

	go func() {
		err := session.Wait()
		_ = outputWriter.CloseWithError(err)
	}()

	var once sync.Once
	closeFn := func() error {
		var closeErr error
		once.Do(func() {
			_ = outputWriter.Close()
			if err := session.Close(); err != nil {
				closeErr = err
			}
			if err := con.Client.Close(); closeErr == nil && err != nil {
				closeErr = err
			}
		})
		return closeErr
	}

	return tvxterm.NewStreamBackend(
		outputReader,
		stdin,
		func(cols, rows int) error {
			return session.WindowChange(rows, cols)
		},
		closeFn,
	), nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
