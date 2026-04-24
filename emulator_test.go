package tvxterm

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestEmulatorLineFeedReturnsToColumnZero(t *testing.T) {
	e := NewEmulator(10, 4)

	_, _ = e.Write([]byte("abc\ndef"))
	ss := e.Snapshot()

	if got := ss.Cells[1][0].Ch; got != 'd' {
		t.Fatalf("expected next line to start at column 0, got %q", got)
	}
	if got := ss.CursorX; got != 3 {
		t.Fatalf("expected cursorX=3 after writing def, got %d", got)
	}
	if got := ss.CursorY; got != 1 {
		t.Fatalf("expected cursorY=1 after newline, got %d", got)
	}
}

func TestEmulatorWrapDefersUntilNextPrintable(t *testing.T) {
	e := NewEmulator(4, 3)

	_, _ = e.Write([]byte("abcd\rZ"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'Z' {
		t.Fatalf("expected carriage return to overwrite same row, got %q", got)
	}
	if got := ss.CursorY; got != 0 {
		t.Fatalf("expected cursor to stay on first row, got %d", got)
	}
}

func TestEmulatorWideRuneConsumesTwoCells(t *testing.T) {
	e := NewEmulator(6, 2)

	_, _ = e.Write([]byte("AあB"))
	ss := e.Snapshot()

	if got := ss.Cells[0][1].Ch; got != 'あ' {
		t.Fatalf("expected wide rune at col 1, got %q", got)
	}
	if !ss.Cells[0][1].Occupied || ss.Cells[0][1].Width != 2 {
		t.Fatalf("expected wide rune to occupy width 2, got occupied=%v width=%d", ss.Cells[0][1].Occupied, ss.Cells[0][1].Width)
	}
	if !ss.Cells[0][2].Occupied || ss.Cells[0][2].Width != 0 {
		t.Fatalf("expected continuation cell at col 2, got occupied=%v width=%d", ss.Cells[0][2].Occupied, ss.Cells[0][2].Width)
	}
	if got := ss.Cells[0][3].Ch; got != 'B' {
		t.Fatalf("expected B at col 3, got %q", got)
	}
	if got := ss.CursorX; got != 4 {
		t.Fatalf("expected cursorX=4 after wide rune sequence, got %d", got)
	}
}

func TestEmulatorCombiningRuneAttachesToPreviousCell(t *testing.T) {
	e := NewEmulator(6, 2)

	_, _ = e.Write([]byte("e\u0301"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'e' {
		t.Fatalf("expected base rune e, got %q", got)
	}
	if len(ss.Cells[0][0].Comb) != 1 || ss.Cells[0][0].Comb[0] != '\u0301' {
		t.Fatalf("expected combining acute accent attached to previous cell, got %#v", ss.Cells[0][0].Comb)
	}
	if got := ss.CursorX; got != 1 {
		t.Fatalf("expected combining rune not to advance cursor, got %d", got)
	}
}

func TestEmulatorIgnoresOSCSequenceTerminatedByBEL(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1b]112;ignored\x07b"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected first rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected OSC payload to be ignored, got %q", got)
	}
	if got := ss.CursorX; got != 2 {
		t.Fatalf("expected cursorX=2 after ignored OSC, got %d", got)
	}
	if got := ss.WindowTitle; got != "" {
		t.Fatalf("expected non-title OSC to be ignored for title state, got %q", got)
	}
}

func TestEmulatorIgnoresOSCSequenceTerminatedByST(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1b]2;title\x1b\\b"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected first rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected OSC payload terminated by ST to be ignored, got %q", got)
	}
	if got := ss.CursorX; got != 2 {
		t.Fatalf("expected cursorX=2 after ignored OSC ST, got %d", got)
	}
	if got := ss.WindowTitle; got != "title" {
		t.Fatalf("expected OSC 2 title to be captured, got %q", got)
	}
}

func TestEmulatorTracksOSCZeroTitleTerminatedByBEL(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("\x1b]0;shell title\x07"))
	if got := e.Snapshot().WindowTitle; got != "shell title" {
		t.Fatalf("expected OSC 0 title to be captured, got %q", got)
	}
}

func TestEmulatorSupportsDECSpecialGraphicsViaG1Shift(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b)0\x0elqxk\x0f"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != '┌' {
		t.Fatalf("expected DEC special graphics corner, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != '─' {
		t.Fatalf("expected DEC special graphics horizontal line, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != '│' {
		t.Fatalf("expected DEC special graphics vertical line, got %q", got)
	}
	if got := ss.Cells[0][3].Ch; got != '┐' {
		t.Fatalf("expected DEC special graphics corner, got %q", got)
	}
}

func TestEmulatorRestoresASCIIAfterShiftIn(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b)0\x0eq\x0fxq"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != '─' {
		t.Fatalf("expected special graphics before shift-in, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'x' {
		t.Fatalf("expected ASCII after shift-in, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'q' {
		t.Fatalf("expected ASCII q after shift-in, got %q", got)
	}
}

func TestEmulatorIgnoresDCSTerminatedByST(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1bP$qignored\x1b\\b"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected leading rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected DCS payload to be ignored, got %q", got)
	}
	if got := ss.CursorX; got != 2 {
		t.Fatalf("expected cursorX=2 after ignored DCS, got %d", got)
	}
}

func TestEmulatorIgnoresAPCAndPMTerminatedByST(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1b_payload\x1b\\b\x1b^meta\x1b\\c"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected leading rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected APC payload to be ignored, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'c' {
		t.Fatalf("expected PM payload to be ignored, got %q", got)
	}
}

func TestEmulatorAbortsControlStringOnCAN(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1bPpayload\x18b"))
	ss := e.Snapshot()

	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected CAN to abort control string, got %q", got)
	}
}

func TestEmulatorConsumesColonSeparatedCSIParams(t *testing.T) {
	e := NewEmulator(20, 2)

	_, _ = e.Write([]byte("a\x1b[4:3m%b"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected first rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != '%' {
		t.Fatalf("expected printable data after CSI to remain, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'b' {
		t.Fatalf("expected trailing printable data to remain, got %q", got)
	}
	if got := ss.CursorX; got != 3 {
		t.Fatalf("expected cursorX=3 after colon CSI sequence, got %d", got)
	}
}

func TestEmulatorApplies256ColorSGR(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[38;5;196;48;5;21mX"))
	ss := e.Snapshot()
	fg, bg, _ := ss.Cells[0][0].Style.Decompose()

	if fg != tcell.PaletteColor(196) {
		t.Fatalf("expected 256-color foreground, got %v", fg)
	}
	if bg != tcell.PaletteColor(21) {
		t.Fatalf("expected 256-color background, got %v", bg)
	}
}

func TestEmulatorAppliesTrueColorSGR(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[38;2;1;2;3;48;2;250;251;252mX"))
	ss := e.Snapshot()
	fg, bg, _ := ss.Cells[0][0].Style.Decompose()

	if fg != tcell.NewRGBColor(1, 2, 3) {
		t.Fatalf("expected truecolor foreground, got %v", fg)
	}
	if bg != tcell.NewRGBColor(250, 251, 252) {
		t.Fatalf("expected truecolor background, got %v", bg)
	}
}

func TestEmulatorAppliesColonSeparatedTrueColorSGR(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[38:2:10:20:30mX"))
	ss := e.Snapshot()
	fg, _, _ := ss.Cells[0][0].Style.Decompose()

	if fg != tcell.NewRGBColor(10, 20, 30) {
		t.Fatalf("expected colon-separated truecolor foreground, got %v", fg)
	}
}

func TestEmulatorSavesAndRestoresCursor(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("ab\x1b7\x1b[10GZ\x1b8c"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected a at col 0, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected b at col 1, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'c' {
		t.Fatalf("expected restore to place c at col 2, got %q", got)
	}
	if got := ss.Cells[0][9].Ch; got != 'Z' {
		t.Fatalf("expected saved move target to keep Z at col 9, got %q", got)
	}
	if got := ss.CursorX; got != 3 {
		t.Fatalf("expected cursorX=3 after restore and write, got %d", got)
	}
}

func TestEmulatorTracksCursorVisibilityPrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?25l"))
	if ss := e.Snapshot(); ss.CursorVis {
		t.Fatalf("expected cursor to be hidden after ?25l")
	}

	_, _ = e.Write([]byte("\x1b[?25h"))
	if ss := e.Snapshot(); !ss.CursorVis {
		t.Fatalf("expected cursor to be visible after ?25h")
	}
}

func TestEmulatorTracksApplicationCursorMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?1h"))
	if ss := e.Snapshot(); !ss.AppCursor {
		t.Fatalf("expected app cursor mode after ?1h")
	}

	_, _ = e.Write([]byte("\x1b[?1l"))
	if ss := e.Snapshot(); ss.AppCursor {
		t.Fatalf("expected normal cursor mode after ?1l")
	}
}

func TestEmulatorTracksReverseVideoPrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?5h"))
	if ss := e.Snapshot(); !ss.ReverseVideo {
		t.Fatalf("expected reverse video after ?5h")
	}

	_, _ = e.Write([]byte("\x1b[?5l"))
	if ss := e.Snapshot(); ss.ReverseVideo {
		t.Fatalf("expected normal video after ?5l")
	}
}

func TestEmulatorTracksCursorBlinkPrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?12h"))
	if ss := e.Snapshot(); !ss.CursorBlink {
		t.Fatalf("expected cursor blink after ?12h")
	}

	_, _ = e.Write([]byte("\x1b[?12l"))
	if ss := e.Snapshot(); ss.CursorBlink {
		t.Fatalf("expected cursor blink disabled after ?12l")
	}
}

func TestEmulatorTracksColumnModePrivateMode(t *testing.T) {
	e := NewEmulator(4, 2)

	_, _ = e.Write([]byte("abcd"))
	_, _ = e.Write([]byte("\x1b[?3h"))
	ss := e.Snapshot()
	if !ss.ColumnMode132 {
		t.Fatalf("expected 132-column mode after ?3h")
	}
	if got := ss.Cells[0][0].Ch; got != ' ' {
		t.Fatalf("expected screen cleared on ?3h, got %q", got)
	}

	_, _ = e.Write([]byte("\x1b[?3l"))
	if ss := e.Snapshot(); ss.ColumnMode132 {
		t.Fatalf("expected 80-column mode after ?3l")
	}
}

func TestEmulatorTracksOriginModeWithinScrollRegion(t *testing.T) {
	e := NewEmulator(4, 5)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\n5555"))
	_, _ = e.Write([]byte("\x1b[2;4r\x1b[?6h\x1b[1;1H@\x1b[6n"))
	ss := e.Snapshot()

	if got := ss.Cells[1][0].Ch; got != '@' {
		t.Fatalf("expected origin-mode home to map into scroll region top, got %q", got)
	}
	if got := ss.Cells[0][0].Ch; got != '1' {
		t.Fatalf("expected row above scroll region to remain unchanged, got %q", got)
	}

	responses := e.DrainResponses()
	if len(responses) != 1 {
		t.Fatalf("expected one DSR response, got %d", len(responses))
	}
	if got := string(responses[0]); got != "\x1b[1;2R" {
		t.Fatalf("expected origin-relative cursor report, got %q", got)
	}

	_, _ = e.Write([]byte("\x1b[?6l\x1b[1;1H#"))
	ss = e.Snapshot()
	if got := ss.Cells[0][0].Ch; got != '#' {
		t.Fatalf("expected normal origin after ?6l, got %q", got)
	}
}

func TestEmulatorTracksApplicationKeypadMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b="))
	if ss := e.Snapshot(); !ss.AppKeypad {
		t.Fatalf("expected app keypad mode after ESC =")
	}

	_, _ = e.Write([]byte("\x1b>"))
	if ss := e.Snapshot(); ss.AppKeypad {
		t.Fatalf("expected normal keypad mode after ESC >")
	}
}

func TestEmulatorTracksApplicationKeypadPrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?66h"))
	if ss := e.Snapshot(); !ss.AppKeypad {
		t.Fatalf("expected app keypad mode after ?66h")
	}

	_, _ = e.Write([]byte("\x1b[?66l"))
	if ss := e.Snapshot(); ss.AppKeypad {
		t.Fatalf("expected normal keypad mode after ?66l")
	}
}

func TestEmulatorTracksBracketedPastePrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?2004h"))
	if ss := e.Snapshot(); !ss.BracketedPaste {
		t.Fatalf("expected bracketed paste after ?2004h")
	}

	_, _ = e.Write([]byte("\x1b[?2004l"))
	if ss := e.Snapshot(); ss.BracketedPaste {
		t.Fatalf("expected normal paste after ?2004l")
	}
}

func TestEmulatorTracksFocusReportingPrivateMode(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?1004h"))
	if ss := e.Snapshot(); !ss.FocusReporting {
		t.Fatalf("expected focus reporting after ?1004h")
	}

	_, _ = e.Write([]byte("\x1b[?1004l"))
	if ss := e.Snapshot(); ss.FocusReporting {
		t.Fatalf("expected focus reporting disabled after ?1004l")
	}
}

func TestEmulatorTracksMouseReportingPrivateModes(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("\x1b[?9h\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h"))
	ss := e.Snapshot()
	if !ss.MouseX10 || !ss.MouseVT200 || !ss.MouseButtonEvt || !ss.MouseAnyEvt || !ss.MouseSGR {
		t.Fatalf("expected mouse modes enabled, got %+v", ss)
	}

	_, _ = e.Write([]byte("\x1b[?9l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"))
	ss = e.Snapshot()
	if ss.MouseX10 || ss.MouseVT200 || ss.MouseButtonEvt || ss.MouseAnyEvt || ss.MouseSGR {
		t.Fatalf("expected mouse modes disabled, got %+v", ss)
	}
}

func TestEmulatorTracksCharsetDesignationEscape(t *testing.T) {
	e := NewEmulator(10, 2)

	_, _ = e.Write([]byte("a\x1b(Bb"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected leading rune to remain visible, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected ESC(B to keep ASCII designation, got %q", got)
	}
	if got := ss.CursorX; got != 2 {
		t.Fatalf("expected cursorX=2 after charset designation, got %d", got)
	}
}

func TestEmulatorRespondsToCursorPositionReport(t *testing.T) {
	e := NewEmulator(10, 4)

	_, _ = e.Write([]byte("ab\ncd\x1b[6n"))
	responses := e.DrainResponses()
	if len(responses) != 1 {
		t.Fatalf("expected one DSR response, got %d", len(responses))
	}
	if got := string(responses[0]); got != "\x1b[2;3R" {
		t.Fatalf("expected cursor report response, got %q", got)
	}
}

func TestEmulatorRespondsToPrimaryDeviceAttributes(t *testing.T) {
	e := NewEmulator(10, 4)

	_, _ = e.Write([]byte("\x1b[c"))
	responses := e.DrainResponses()
	if len(responses) != 1 {
		t.Fatalf("expected one DA response, got %d", len(responses))
	}
	if got := string(responses[0]); got != "\x1b[?1;2c" {
		t.Fatalf("expected primary DA response, got %q", got)
	}
}

func TestEmulatorRespondsToSecondaryDeviceAttributes(t *testing.T) {
	e := NewEmulator(10, 4)

	_, _ = e.Write([]byte("\x1b[>c"))
	responses := e.DrainResponses()
	if len(responses) != 1 {
		t.Fatalf("expected one secondary DA response, got %d", len(responses))
	}
	if got := string(responses[0]); got != "\x1b[>0;10;1c" {
		t.Fatalf("expected secondary DA response, got %q", got)
	}
}

func TestEmulatorTracksAutoWrapPrivateMode(t *testing.T) {
	e := NewEmulator(3, 2)

	_, _ = e.Write([]byte("\x1b[?7labc"))
	ss := e.Snapshot()
	if !ss.Cells[0][2].Occupied || ss.Cells[0][2].Ch != 'c' {
		t.Fatalf("expected c to stay on last cell with autowrap off, got %q", ss.Cells[0][2].Ch)
	}
	if got := ss.CursorY; got != 0 {
		t.Fatalf("expected cursor to stay on first row with autowrap off, got %d", got)
	}

	_, _ = e.Write([]byte("\x1b[?7h"))
	if ss := e.Snapshot(); !ss.AutoWrap {
		t.Fatalf("expected autowrap to be re-enabled after ?7h")
	}
}

func TestEmulatorAlternateScreenRestoresPrimaryBuffer(t *testing.T) {
	e := NewEmulator(6, 3)

	_, _ = e.Write([]byte("main"))
	before := e.Snapshot()
	if before.UsingAlt {
		t.Fatalf("expected primary screen initially")
	}

	_, _ = e.Write([]byte("\x1b[?1049halt"))
	alt := e.Snapshot()
	if !alt.UsingAlt {
		t.Fatalf("expected alternate screen after ?1049h")
	}
	if got := alt.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected alternate screen to start blank before writing, got %q", got)
	}
	if got := alt.Cells[0][1].Ch; got != 'l' {
		t.Fatalf("expected alternate buffer contents after write, got %q", got)
	}

	_, _ = e.Write([]byte("\x1b[?1049l"))
	after := e.Snapshot()
	if after.UsingAlt {
		t.Fatalf("expected primary screen after ?1049l")
	}
	if got := after.Cells[0][0].Ch; got != 'm' {
		t.Fatalf("expected primary buffer restored at col 0, got %q", got)
	}
	if got := after.Cells[0][1].Ch; got != 'a' {
		t.Fatalf("expected primary buffer restored at col 1, got %q", got)
	}
	if got := after.Cells[0][2].Ch; got != 'i' {
		t.Fatalf("expected primary buffer restored at col 2, got %q", got)
	}
	if got := after.Cells[0][3].Ch; got != 'n' {
		t.Fatalf("expected primary buffer restored at col 3, got %q", got)
	}
	if got := after.CursorX; got != before.CursorX {
		t.Fatalf("expected cursorX restored to %d, got %d", before.CursorX, got)
	}
	if got := after.CursorY; got != before.CursorY {
		t.Fatalf("expected cursorY restored to %d, got %d", before.CursorY, got)
	}
}

func TestEmulatorPrivateMode1048SavesAndRestoresCursor(t *testing.T) {
	e := NewEmulator(8, 3)

	_, _ = e.Write([]byte("ab\x1b[?1048h\x1b[3;4HZ\x1b[?1048lc"))
	ss := e.Snapshot()

	if got := ss.Cells[2][3].Ch; got != 'Z' {
		t.Fatalf("expected write at moved cursor before restore, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'c' {
		t.Fatalf("expected restore from ?1048 to place c after saved cursor, got %q", got)
	}
	if got := ss.CursorX; got != 3 {
		t.Fatalf("expected cursorX=3 after restored write, got %d", got)
	}
	if got := ss.CursorY; got != 0 {
		t.Fatalf("expected cursorY=0 after restored write, got %d", got)
	}
}

func TestEmulatorScrollRegionScrollsWithinMargins(t *testing.T) {
	e := NewEmulator(4, 5)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\n5555"))
	_, _ = e.Write([]byte("\x1b[2;4r\x1b[4;1H\n"))
	ss := e.Snapshot()

	if got := ss.ScrollTop; got != 1 {
		t.Fatalf("expected scroll top 1, got %d", got)
	}
	if got := ss.ScrollBottom; got != 3 {
		t.Fatalf("expected scroll bottom 3, got %d", got)
	}
	if got := ss.Cells[0][0].Ch; got != '1' {
		t.Fatalf("expected row 1 unchanged outside region, got %q", got)
	}
	if got := ss.Cells[1][0].Ch; got != '3' {
		t.Fatalf("expected row 2 to receive previous row 3 after scroll, got %q", got)
	}
	if got := ss.Cells[2][0].Ch; got != '4' {
		t.Fatalf("expected row 3 to receive previous row 4 after scroll, got %q", got)
	}
	if got := ss.Cells[3][0].Ch; got != ' ' {
		t.Fatalf("expected bottom row of region blanked after scroll, got %q", got)
	}
	if got := ss.Cells[4][0].Ch; got != '5' {
		t.Fatalf("expected row 5 unchanged outside region, got %q", got)
	}
}

func TestEmulatorInsertDeleteLinesRespectScrollRegion(t *testing.T) {
	e := NewEmulator(4, 5)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\n5555"))
	_, _ = e.Write([]byte("\x1b[2;4r\x1b[2;1H\x1b[L"))
	ss := e.Snapshot()
	if got := ss.Cells[0][0].Ch; got != '1' {
		t.Fatalf("expected row 1 unchanged after insert line in region, got %q", got)
	}
	if got := ss.Cells[1][0].Ch; got != ' ' {
		t.Fatalf("expected inserted blank line at region top, got %q", got)
	}
	if got := ss.Cells[2][0].Ch; got != '2' {
		t.Fatalf("expected former row 2 shifted down within region, got %q", got)
	}
	if got := ss.Cells[4][0].Ch; got != '5' {
		t.Fatalf("expected row 5 unchanged after insert line in region, got %q", got)
	}

	_, _ = e.Write([]byte("\x1b[2;1H\x1b[M"))
	ss = e.Snapshot()
	if got := ss.Cells[1][0].Ch; got != '2' {
		t.Fatalf("expected delete line to pull row 3 into region top, got %q", got)
	}
	if got := ss.Cells[4][0].Ch; got != '5' {
		t.Fatalf("expected row 5 unchanged after delete line in region, got %q", got)
	}
}

func TestEmulatorTracksScrollbackForFullScreenScroll(t *testing.T) {
	e := NewEmulator(4, 3)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444"))
	ss := e.Snapshot()
	if got := ss.ScrollbackRows; got != 1 {
		t.Fatalf("expected one scrollback row after full-screen scroll, got %d", got)
	}

	view := e.SnapshotAt(1)
	if got := view.Cells[0][0].Ch; got != '1' {
		t.Fatalf("expected scrollback view to show first scrolled row, got %q", got)
	}
	if view.CursorVis {
		t.Fatalf("expected cursor hidden while viewing scrollback")
	}
}

func TestEmulatorResizeKeepsFullscreenScrollRegion(t *testing.T) {
	e := NewEmulator(4, 3)
	e.Resize(4, 5)

	ss := e.Snapshot()
	if got := ss.ScrollTop; got != 0 {
		t.Fatalf("expected fullscreen scroll region top 0 after resize, got %d", got)
	}
	if got := ss.ScrollBottom; got != 4 {
		t.Fatalf("expected fullscreen scroll region bottom 4 after resize, got %d", got)
	}

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\n5555\n6666"))
	if got := e.Snapshot().ScrollbackRows; got == 0 {
		t.Fatalf("expected scrollback after resize-adjusted fullscreen scroll region")
	}
}

func TestEmulatorAlternateScreenDoesNotAccumulateScrollback(t *testing.T) {
	e := NewEmulator(4, 3)

	_, _ = e.Write([]byte("\x1b[?1049h1111\n2222\n3333\n4444"))
	ss := e.Snapshot()
	if !ss.UsingAlt {
		t.Fatalf("expected alternate screen active")
	}
	if got := ss.ScrollbackRows; got != 0 {
		t.Fatalf("expected no scrollback in alternate screen, got %d", got)
	}
}

func TestEmulatorEscIndexMovesDownWithoutCarriageReturn(t *testing.T) {
	e := NewEmulator(4, 3)

	_, _ = e.Write([]byte("ab\x1bDc"))
	ss := e.Snapshot()

	if got := ss.Cells[1][2].Ch; got != 'c' {
		t.Fatalf("expected ESC D to preserve column while moving down, got %q", got)
	}
	if got := ss.CursorX; got != 3 {
		t.Fatalf("expected cursorX=3 after ESC D write, got %d", got)
	}
	if got := ss.CursorY; got != 1 {
		t.Fatalf("expected cursorY=1 after ESC D write, got %d", got)
	}
}

func TestEmulatorEscNextLineMovesToColumnZero(t *testing.T) {
	e := NewEmulator(4, 3)

	_, _ = e.Write([]byte("ab\x1bEc"))
	ss := e.Snapshot()

	if got := ss.Cells[1][0].Ch; got != 'c' {
		t.Fatalf("expected ESC E to move to next line column 0, got %q", got)
	}
	if got := ss.CursorX; got != 1 {
		t.Fatalf("expected cursorX=1 after ESC E write, got %d", got)
	}
	if got := ss.CursorY; got != 1 {
		t.Fatalf("expected cursorY=1 after ESC E write, got %d", got)
	}
}

func TestEmulatorEscReverseIndexScrollsFullAlternateScreen(t *testing.T) {
	e := NewEmulator(6, 4)

	_, _ = e.Write([]byte("\x1b[?1049h111111\n222222\n333333\n444444"))
	_, _ = e.Write([]byte("\x1b[H\x1bMAAAAAA"))
	ss := e.Snapshot()

	if !ss.UsingAlt {
		t.Fatalf("expected alternate screen active")
	}
	if got := ss.Cells[0][0].Ch; got != 'A' {
		t.Fatalf("expected reverse index redraw line at top, got %q", got)
	}
	if got := ss.Cells[1][0].Ch; got != '1' {
		t.Fatalf("expected former top row shifted down after reverse index, got %q", got)
	}
	if got := ss.Cells[2][0].Ch; got != '2' {
		t.Fatalf("expected second row shifted down after reverse index, got %q", got)
	}
	if got := ss.Cells[3][0].Ch; got != '3' {
		t.Fatalf("expected third row shifted down after reverse index, got %q", got)
	}
}

func TestEmulatorEscReverseIndexRespectsScrollRegion(t *testing.T) {
	e := NewEmulator(4, 5)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\n5555"))
	_, _ = e.Write([]byte("\x1b[2;4r\x1b[2;1H\x1bM"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != '1' {
		t.Fatalf("expected row above scroll region unchanged, got %q", got)
	}
	if got := ss.Cells[1][0].Ch; got != ' ' {
		t.Fatalf("expected blank row inserted at scroll-region top, got %q", got)
	}
	if got := ss.Cells[2][0].Ch; got != '2' {
		t.Fatalf("expected former region top shifted down within region, got %q", got)
	}
	if got := ss.Cells[3][0].Ch; got != '3' {
		t.Fatalf("expected remaining region rows shifted down within region, got %q", got)
	}
	if got := ss.Cells[4][0].Ch; got != '5' {
		t.Fatalf("expected row below scroll region unchanged, got %q", got)
	}
}

func TestEmulatorDeleteChars(t *testing.T) {
	e := NewEmulator(6, 2)

	_, _ = e.Write([]byte("abcdef\x1b[1;3H\x1b[P"))
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != 'a' {
		t.Fatalf("expected a at col 0, got %q", got)
	}
	if got := ss.Cells[0][1].Ch; got != 'b' {
		t.Fatalf("expected b at col 1, got %q", got)
	}
	if got := ss.Cells[0][2].Ch; got != 'd' {
		t.Fatalf("expected d shifted into col 2, got %q", got)
	}
}

func TestEmulatorInsertDeleteLines(t *testing.T) {
	e := NewEmulator(4, 4)

	_, _ = e.Write([]byte("1111\n2222\n3333\n4444\x1b[2;1H\x1b[M"))
	ss := e.Snapshot()
	if got := ss.Cells[1][0].Ch; got != '3' {
		t.Fatalf("expected line 3 to move into row 2 after delete line, got %q", got)
	}

	_, _ = e.Write([]byte("\x1b[2;1H\x1b[L"))
	ss = e.Snapshot()
	if got := ss.Cells[1][0].Ch; got != ' ' {
		t.Fatalf("expected inserted blank line at row 2, got %q", got)
	}
	if got := ss.Cells[2][0].Ch; got != '3' {
		t.Fatalf("expected shifted content below inserted line, got %q", got)
	}
}

func TestEmulatorScrollDown(t *testing.T) {
	e := NewEmulator(3, 3)

	_, _ = e.Write([]byte("111\n222\n333"))
	e.scrollDown(1)
	ss := e.Snapshot()

	if got := ss.Cells[0][0].Ch; got != ' ' {
		t.Fatalf("expected blank top row after scroll down, got %q", got)
	}
	if got := ss.Cells[1][0].Ch; got != '1' {
		t.Fatalf("expected first row content shifted down, got %q", got)
	}
}
