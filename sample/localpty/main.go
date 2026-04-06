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

func main() {
	app := tview.NewApplication()
	var stopOnce sync.Once

	status := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	status.SetBorder(true).SetTitle("Status")
	status.SetText("[yellow]PageUp/PageDown[-]: scroll  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit")

	term := tvxterm.New(app)
	term.SetBorder(true).SetTitle("tvxterm")
	term.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseScrollUp:
			term.ScrollbackUp(3)
			offset, rows := term.ScrollbackStatus()
			status.SetText(fmt.Sprintf("[yellow]scrollback[-] offset=%d rows=%d  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit", offset, rows))
			return action, nil
		case tview.MouseScrollDown:
			term.ScrollbackDown(3)
			offset, rows := term.ScrollbackStatus()
			status.SetText(fmt.Sprintf("[yellow]scrollback[-] offset=%d rows=%d  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit", offset, rows))
			return action, nil
		default:
			return action, event
		}
	})

	cmd := exec.Command(shell())
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	backend, err := tvxterm.NewPTYBackend(cmd, 80, 24)
	if err != nil {
		log.Fatal(err)
	}
	defer backend.Close()

	term.SetTitleHandler(func(_ *tvxterm.View, title string) {
		app.QueueUpdateDraw(func() {
			if title == "" {
				term.SetTitle("tvxterm")
				return
			}
			term.SetTitle(title)
		})
	})

	term.SetBackendExitHandler(func(_ *tvxterm.View, err error) {
		stopOnce.Do(func() {
			app.QueueUpdateDraw(func() {
				if err != nil {
					status.SetText(fmt.Sprintf("[red]pane closed:[-] %v", err))
				} else {
					status.SetText("[red]pane closed[-]")
				}
			})
			_ = term.Close()
			app.Stop()
		})
	})

	term.Attach(backend)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(term, 0, 1, true).
		AddItem(status, 3, 0, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			return cloneKeyEvent(event)
		}
		switch event.Key() {
		case tcell.KeyPgUp:
			term.ScrollbackPageUp()
			offset, rows := term.ScrollbackStatus()
			status.SetText(fmt.Sprintf("[yellow]scrollback[-] offset=%d rows=%d  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit", offset, rows))
			return nil
		case tcell.KeyPgDn:
			term.ScrollbackPageDown()
			offset, rows := term.ScrollbackStatus()
			status.SetText(fmt.Sprintf("[yellow]scrollback[-] offset=%d rows=%d  [yellow]Mouse wheel[-]: scroll  [yellow]Ctrl+Q[-]: quit", offset, rows))
			return nil
		case tcell.KeyCtrlQ:
			app.Stop()
			return nil
		default:
			return event
		}
	})

	if err := app.SetRoot(root, true).SetFocus(term).EnableMouse(true).EnablePaste(true).Run(); err != nil {
		log.Fatal(err)
	}
}

func cloneKeyEvent(event *tcell.EventKey) *tcell.EventKey {
	return tcell.NewEventKey(event.Key(), event.Rune(), event.Modifiers())
}

func shell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}
