package tvxterm

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestKeyToBytesUsesApplicationCursorMode(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, true)
	if !ok {
		t.Fatalf("expected key to be handled")
	}
	if string(got) != "\x1bOB" {
		t.Fatalf("expected application cursor sequence, got %q", string(got))
	}
}

func TestKeyToBytesUsesNormalCursorMode(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected key to be handled")
	}
	if string(got) != "\x1b[B" {
		t.Fatalf("expected normal cursor sequence, got %q", string(got))
	}
}

func TestKeyToBytesUsesCtrlHForBackspace(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected backspace key to be handled")
	}
	if len(got) != 1 || got[0] != 0x08 {
		t.Fatalf("expected ctrl-h backspace, got %q", string(got))
	}
}

func TestKeyToBytesUsesDelForBackspace2(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected backspace2 key to be handled")
	}
	if len(got) != 1 || got[0] != 0x7f {
		t.Fatalf("expected DEL backspace, got %q", string(got))
	}
}

func TestKeyToBytesUsesCtrlA(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected ctrl-a to be handled")
	}
	if len(got) != 1 || got[0] != 0x01 {
		t.Fatalf("expected ctrl-a byte, got %v", got)
	}
}

func TestKeyToBytesUsesCtrlZ(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyCtrlZ, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected ctrl-z to be handled")
	}
	if len(got) != 1 || got[0] != 0x1a {
		t.Fatalf("expected ctrl-z byte, got %v", got)
	}
}

func TestKeyToBytesUsesAltBackspace(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyBackspace2, 0, tcell.ModAlt)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected alt-backspace to be handled")
	}
	want := []byte{0x1b, 0x7f}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected alt-backspace bytes %v, got %v", want, got)
	}
}

func TestKeyToBytesUsesAltLeftForBackwardWord(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected alt-left to be handled")
	}
	want := []byte{0x1b, 'b'}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected alt-left bytes %v, got %v", want, got)
	}
}

func TestKeyToBytesUsesAltRightForForwardWord(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModAlt)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected alt-right to be handled")
	}
	want := []byte{0x1b, 'f'}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected alt-right bytes %v, got %v", want, got)
	}
}

func TestKeyToBytesUsesCtrlLeft(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModCtrl)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected ctrl-left to be handled")
	}
	if string(got) != "\x1b[1;5D" {
		t.Fatalf("expected ctrl-left sequence, got %q", string(got))
	}
}

func TestKeyToBytesUsesHomeAndEnd(t *testing.T) {
	home, ok := keyToBytes(tcell.NewEventKey(tcell.KeyHome, 0, tcell.ModNone), false)
	if !ok || string(home) != "\x1b[H" {
		t.Fatalf("expected home sequence, got %q", string(home))
	}

	end, ok := keyToBytes(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), false)
	if !ok || string(end) != "\x1b[F" {
		t.Fatalf("expected end sequence, got %q", string(end))
	}
}

func TestKeyToBytesUsesDelete(t *testing.T) {
	ev := tcell.NewEventKey(tcell.KeyDelete, 0, tcell.ModNone)

	got, ok := keyToBytes(ev, false)
	if !ok {
		t.Fatalf("expected delete to be handled")
	}
	if string(got) != "\x1b[3~" {
		t.Fatalf("expected delete sequence, got %q", string(got))
	}
}

type shortWriter struct {
	writes [][]byte
	limit  int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	n := w.limit
	if n <= 0 || n > len(p) {
		n = len(p)
	}
	chunk := make([]byte, n)
	copy(chunk, p[:n])
	w.writes = append(w.writes, chunk)
	return n, nil
}

func TestWriteAllRetriesShortWrites(t *testing.T) {
	w := &shortWriter{limit: 1}

	if err := writeAll(w, []byte("\x1b[B")); err != nil {
		t.Fatalf("expected short writes to be retried, got %v", err)
	}
	if len(w.writes) != 3 {
		t.Fatalf("expected 3 writes, got %d", len(w.writes))
	}
}

type zeroWriter struct{}

func (zeroWriter) Write(p []byte) (int, error) {
	return 0, nil
}

func TestWriteAllReturnsShortWriteOnZeroProgress(t *testing.T) {
	err := writeAll(zeroWriter{}, []byte("abc"))
	if err != io.ErrShortWrite {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

func TestViewScrollbackPageMethodsChangeOffset(t *testing.T) {
	v := New(nil)
	v.emu = NewEmulator(4, 3)
	_, _ = v.emu.Write([]byte("1111\n2222\n3333\n4444"))

	v.ScrollbackPageUp()
	if v.scrollOffset == 0 {
		t.Fatalf("expected scroll offset to increase after page up")
	}

	v.ScrollbackPageDown()
	if v.scrollOffset != 0 {
		t.Fatalf("expected scroll offset reset after page down, got %d", v.scrollOffset)
	}
}

type stubBackend struct {
	writes [][]byte
}

func (b *stubBackend) Read(p []byte) (int, error) { return 0, io.EOF }
func (b *stubBackend) Write(p []byte) (int, error) {
	b.writes = append(b.writes, append([]byte(nil), p...))
	return len(p), nil
}
func (b *stubBackend) Resize(cols, rows int) error { return nil }
func (b *stubBackend) Close() error                { return nil }

func TestViewInputResetsScrollbackToBottom(t *testing.T) {
	v := New(nil)
	v.emu = NewEmulator(4, 3)
	_, _ = v.emu.Write([]byte("1111\n2222\n3333\n4444"))
	v.scrollOffset = 1

	backend := &stubBackend{}
	v.backend = backend

	handler := v.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone), func(p tview.Primitive) {})

	if v.scrollOffset != 0 {
		t.Fatalf("expected input to reset scrollback to bottom, got offset %d", v.scrollOffset)
	}
	if len(backend.writes) != 1 || string(backend.writes[0]) != "x" {
		t.Fatalf("expected backend to receive typed rune, got %#v", backend.writes)
	}
}

func TestViewPasteHandlerUsesBracketedPasteWhenEnabled(t *testing.T) {
	v := New(nil)
	backend := &stubBackend{}
	v.backend = backend
	_, _ = v.emu.Write([]byte("\x1b[?2004h"))

	handler := v.PasteHandler()
	handler("hello", func(p tview.Primitive) {})

	if len(backend.writes) != 1 {
		t.Fatalf("expected one backend write, got %d", len(backend.writes))
	}
	if got := string(backend.writes[0]); got != "\x1b[200~hello\x1b[201~" {
		t.Fatalf("expected bracketed paste payload, got %q", got)
	}
}

func TestViewPasteHandlerUsesPlainPasteWhenDisabled(t *testing.T) {
	v := New(nil)
	backend := &stubBackend{}
	v.backend = backend

	handler := v.PasteHandler()
	handler("hello", func(p tview.Primitive) {})

	if len(backend.writes) != 1 {
		t.Fatalf("expected one backend write, got %d", len(backend.writes))
	}
	if got := string(backend.writes[0]); got != "hello" {
		t.Fatalf("expected plain paste payload, got %q", got)
	}
}

func TestDrawStyleSwapsColorsInReverseVideo(t *testing.T) {
	style := tcell.StyleDefault.Foreground(tcell.ColorRed).Background(tcell.ColorBlue).Bold(true)

	got := drawStyle(style, true)
	fg, bg, attr := got.Decompose()
	if fg != tcell.ColorBlue || bg != tcell.ColorRed {
		t.Fatalf("expected swapped colors, got fg=%v bg=%v", fg, bg)
	}
	if attr&tcell.AttrBold == 0 {
		t.Fatalf("expected attributes to be preserved, got %v", attr)
	}
}

func TestViewFocusReportsWhenEnabled(t *testing.T) {
	v := New(nil)
	backend := &stubBackend{}
	v.backend = backend
	_, _ = v.emu.Write([]byte("\x1b[?1004h"))

	v.Focus(nil)
	v.Blur()

	if len(backend.writes) != 2 {
		t.Fatalf("expected two backend writes, got %d", len(backend.writes))
	}
	if got := string(backend.writes[0]); got != "\x1b[I" {
		t.Fatalf("expected focus-in report, got %q", got)
	}
	if got := string(backend.writes[1]); got != "\x1b[O" {
		t.Fatalf("expected focus-out report, got %q", got)
	}
}

func TestViewFocusDoesNotReportWhenDisabled(t *testing.T) {
	v := New(nil)
	backend := &stubBackend{}
	v.backend = backend

	v.Focus(nil)
	v.Blur()

	if len(backend.writes) != 0 {
		t.Fatalf("expected no focus reports when disabled, got %#v", backend.writes)
	}
}

func TestMouseEventToBytesUsesSGREncoding(t *testing.T) {
	ss := Snapshot{MouseVT200: true, MouseSGR: true}
	ev := tcell.NewEventMouse(4, 6, tcell.Button1, tcell.ModCtrl)

	got, ok := mouseEventToBytes(tview.MouseLeftDown, ev, ss, 2, 3)
	if !ok {
		t.Fatalf("expected mouse event to be encoded")
	}
	if string(got) != "\x1b[<16;3;4M" {
		t.Fatalf("expected sgr mouse report, got %q", got)
	}
}

func TestMouseEventToBytesUsesClassicEncoding(t *testing.T) {
	ss := Snapshot{MouseVT200: true}
	ev := tcell.NewEventMouse(1, 2, tcell.Button1, tcell.ModNone)

	got, ok := mouseEventToBytes(tview.MouseLeftDown, ev, ss, 0, 0)
	if !ok {
		t.Fatalf("expected mouse event to be encoded")
	}
	want := []byte{0x1b, '[', 'M', 32, 34, 35}
	if string(got) != string(want) {
		t.Fatalf("expected classic mouse report %v, got %v", want, got)
	}
}

func TestMouseEventToBytesUsesMotionOnlyWhenConfigured(t *testing.T) {
	ev := tcell.NewEventMouse(2, 2, tcell.Button1, tcell.ModNone)

	if _, ok := mouseEventToBytes(tview.MouseMove, ev, Snapshot{MouseVT200: true}, 0, 0); ok {
		t.Fatalf("expected motion ignored without 1002/1003")
	}
	if _, ok := mouseEventToBytes(tview.MouseMove, ev, Snapshot{MouseButtonEvt: true}, 0, 0); !ok {
		t.Fatalf("expected motion encoded with button-event tracking")
	}
	if _, ok := mouseEventToBytes(tview.MouseMove, tcell.NewEventMouse(2, 2, 0, tcell.ModNone), Snapshot{MouseAnyEvt: true}, 0, 0); !ok {
		t.Fatalf("expected motion encoded with any-event tracking")
	}
}

func TestMouseEventToBytesUsesX10PressOnly(t *testing.T) {
	ss := Snapshot{MouseX10: true}

	if _, ok := mouseEventToBytes(tview.MouseLeftDown, tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone), ss, 0, 0); !ok {
		t.Fatalf("expected x10 press to be encoded")
	}
	if _, ok := mouseEventToBytes(tview.MouseLeftUp, tcell.NewEventMouse(1, 1, 0, tcell.ModNone), ss, 0, 0); ok {
		t.Fatalf("expected x10 release to be ignored")
	}
	if _, ok := mouseEventToBytes(tview.MouseMove, tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone), ss, 0, 0); ok {
		t.Fatalf("expected x10 motion to be ignored")
	}
}

func TestViewMouseHandlerReportsWhenEnabled(t *testing.T) {
	v := New(nil)
	v.SetRect(0, 0, 10, 5)
	backend := &stubBackend{}
	v.backend = backend
	_, _ = v.emu.Write([]byte("\x1b[?1000h\x1b[?1006h"))

	handler := v.MouseHandler()
	consumed, _ := handler(tview.MouseLeftDown, tcell.NewEventMouse(1, 1, tcell.Button1, tcell.ModNone), func(p tview.Primitive) {})

	if !consumed {
		t.Fatalf("expected mouse event to be consumed")
	}
	if len(backend.writes) != 1 {
		t.Fatalf("expected one mouse report write, got %d", len(backend.writes))
	}
	if got := string(backend.writes[0]); got != "\x1b[<0;2;2M" {
		t.Fatalf("expected sgr mouse sequence, got %q", got)
	}
}

func TestViewSyncTitleCallsHandler(t *testing.T) {
	v := New(nil)
	var got string
	v.SetTitleHandler(func(_ *View, title string) {
		got = title
	})

	_, _ = v.emu.Write([]byte("\x1b]2;remote shell\x07"))
	v.syncTitle()

	if got != "remote shell" {
		t.Fatalf("expected title handler to receive remote title, got %q", got)
	}
}

func TestViewPTYAccumulatesScrollback(t *testing.T) {
	backend, err := NewPTYBackend(exec.Command("/bin/sh", "-lc", "seq 1 200"), 20, 5)
	if err != nil {
		t.Fatalf("new pty backend: %v", err)
	}
	defer backend.Close()

	v := New(nil)
	v.Attach(backend)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, rows := v.ScrollbackStatus()
		if rows > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, rows := v.ScrollbackStatus()
	t.Fatalf("expected PTY-backed view to accumulate scrollback, got rows=%d", rows)
}

func TestViewInteractiveShellAccumulatesScrollback(t *testing.T) {
	backend, err := NewPTYBackend(exec.Command("/bin/sh"), 20, 5)
	if err != nil {
		t.Fatalf("new pty backend: %v", err)
	}
	defer backend.Close()

	v := New(nil)
	v.Attach(backend)

	if err := writeAll(backend, []byte("seq 1 200\r")); err != nil {
		t.Fatalf("write command: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, rows := v.ScrollbackStatus()
		if rows > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, rows := v.ScrollbackStatus()
	t.Fatalf("expected interactive shell view to accumulate scrollback, got rows=%d", rows)
}

func TestViewUserShellAccumulatesScrollback(t *testing.T) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		t.Skip("SHELL is not set")
	}

	backend, err := NewPTYBackend(exec.Command(shell), 20, 5)
	if err != nil {
		t.Fatalf("new pty backend: %v", err)
	}
	defer backend.Close()

	v := New(nil)
	v.Attach(backend)

	if err := writeAll(backend, []byte("seq 1 200\r")); err != nil {
		t.Fatalf("write command: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, rows := v.ScrollbackStatus()
		if rows > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, rows := v.ScrollbackStatus()
	t.Fatalf("expected user shell view to accumulate scrollback, got rows=%d", rows)
}

func TestViewUserShellWithXtermEnvAccumulatesScrollback(t *testing.T) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		t.Skip("SHELL is not set")
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	backend, err := NewPTYBackend(cmd, 20, 5)
	if err != nil {
		t.Fatalf("new pty backend: %v", err)
	}
	defer backend.Close()

	v := New(nil)
	v.Attach(backend)

	if err := writeAll(backend, []byte("seq 1 200\r")); err != nil {
		t.Fatalf("write command: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, rows := v.ScrollbackStatus()
		if rows > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, rows := v.ScrollbackStatus()
	t.Fatalf("expected xterm-env shell view to accumulate scrollback, got rows=%d", rows)
}
