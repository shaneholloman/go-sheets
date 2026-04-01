package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hinshun/vt10x"
)

const (
	integrationCols    = 80
	integrationRows    = 24
	integrationTimeout = 3 * time.Second
	pollInterval       = 10 * time.Millisecond
)

var (
	ctrlBKey  = tea.KeyMsg{Type: tea.KeyCtrlB}
	ctrlFKey  = tea.KeyMsg{Type: tea.KeyCtrlF}
	ctrlRKey  = tea.KeyMsg{Type: tea.KeyCtrlR}
	ctrlUKey  = tea.KeyMsg{Type: tea.KeyCtrlU}
	enterKey  = tea.KeyMsg{Type: tea.KeyEnter}
	escapeKey = tea.KeyMsg{Type: tea.KeyEscape}
)

type terminalSession struct {
	t       *testing.T
	program *tea.Program
	term    vt10x.Terminal
	done    chan error
	once    sync.Once
}

type terminalScreen struct {
	raw     string
	command string
	status  string
}

func TestTerminalIntegration(t *testing.T) {
	sampleCSV := writeSampleCSV(t)

	t.Run("loads csv and navigates formulas", func(t *testing.T) {
		session := startTerminalSession(t, sampleCSV)

		session.waitForInitialSheet("1")

		session.sendText("jjjj")
		screen := session.waitForScreen("formula row render", func(screen terminalScreen) bool {
			return screen.statusContains("A5", "=SUM(A1:A4)") &&
				screen.screenContains("10")
		})
		screen.assertStatusContains(t, "A5", "=SUM(A1:A4)")
	})

	t.Run("supports goto and visual selection status", func(t *testing.T) {
		session := startTerminalSession(t, sampleCSV)
		session.waitForStatus("initial spreadsheet render", "A1")

		session.sendText("gA5")
		session.waitForStatus("goto command", "A5", "gA5")

		session.sendText("vlj")
		session.waitForStatus("visual selection render", "VISUAL", "A5:B6")
	})

	t.Run("templates formula from visual selection", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForInitialSheet()

		session.fillColumnA("1", "2", "3", "4", "5", "6")

		session.sendText("ggvjjjjj=")
		session.waitForStatus("formula template enters insert mode", "INSERT", "A7", "=(A1:A6)")

		session.enterTextAndWaitForStatus("formula template commits", "SUM", "NORMAL", "A7", "=SUM(A1:A6)")
	})

	t.Run("supports colon command prompt in status bar", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForInitialSheet()

		session.sendText(":goto 5")
		screen := session.waitForScreen("command prompt render", func(screen terminalScreen) bool {
			return screen.statusContains(":goto 5") && screen.statusOmits("NORMAL")
		})
		screen.assertStatusContains(t, ":goto 5")
		screen.assertCommandEmpty(t)

		session.sendKey(ctrlBKey)
		session.sendText("E")
		session.sendKey(ctrlFKey)
		session.waitForStatus("command prompt mid-buffer edit", ":goto E5")

		session.sendKey(enterKey)
		screen = session.waitForScreen("command prompt executes goto", func(screen terminalScreen) bool {
			return screen.commandBlank() && screen.statusContains("NORMAL", "E5")
		})
		screen.assertStatusContains(t, "NORMAL", "E5")
	})

	t.Run("recalculates formulas after terminal edits", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForEmptySheet()

		session.sendTextAndEscape("ggi1\t2\t=A1+B1")
		screen := session.waitForScreen("initial formula entry", func(screen terminalScreen) bool {
			return screen.statusContains("C1", "=A1+B1") &&
				screen.screenContains("=3")
		})
		screen.assertStatusContains(t, "C1", "=A1+B1")

		session.sendText("ggi")
		session.sendKey(ctrlUKey)
		session.sendText("4")
		session.sendKey(escapeKey)
		session.waitForStatus("updated source cell", "A1", "4")

		session.sendText("ll")
		screen = session.waitForScreen("recalculated formula", func(screen terminalScreen) bool {
			return screen.statusContains("C1", "=A1+B1") &&
				screen.screenContains("=6")
		})
		screen.assertStatusContains(t, "C1", "=A1+B1")
	})

	t.Run("preserves copy paste undo redo flows", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForEmptySheet()

		session.enterTextAndWaitForStatus("seed cell value", "ggiabc", "A1", "abc")

		session.sendText("ylp")
		session.waitForStatus("pasted value", "B1", "abc")

		session.sendText("u")
		screen := session.waitForScreen("undo paste", func(screen terminalScreen) bool {
			return screen.statusContains("B1") && screen.statusOmits("abc")
		})
		screen.assertStatusContains(t, "B1")
		screen.assertStatusOmits(t, "abc")

		session.sendKey(ctrlRKey)
		session.waitForStatus("redo paste", "B1", "abc")
	})

	t.Run("opens rows above and below in insert mode", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForEmptySheet()

		session.enterTextAndWaitForStatus("seed first row", "ggiabove", "A1", "above")

		session.enterTextAndWaitForStatus("seed second row", "jibelow", "A2", "below")

		session.sendText("ko")
		screen := session.waitForScreen("open row below enters insert mode", func(screen terminalScreen) bool {
			return screen.statusContains("INSERT", "A2") &&
				screen.screenContains("above", "below")
		})
		screen.assertStatusContains(t, "INSERT", "A2")
		session.sendTextAndEscape("middle")
		screen = session.waitForScreen("inserted below row committed", func(screen terminalScreen) bool {
			return screen.statusContains("NORMAL", "A2", "middle") &&
				screen.screenContains("above", "below")
		})
		screen.assertStatusContains(t, "NORMAL", "A2", "middle")

		session.sendText("O")
		screen = session.waitForScreen("open row above enters insert mode", func(screen terminalScreen) bool {
			return screen.statusContains("INSERT", "A2") &&
				screen.screenContains("middle", "below")
		})
		screen.assertStatusContains(t, "INSERT", "A2")
		session.sendTextAndEscape("top")
		screen = session.waitForScreen("inserted above row committed", func(screen terminalScreen) bool {
			return screen.statusContains("NORMAL", "A2", "top") &&
				screen.screenContains("above", "middle", "below")
		})
		screen.assertStatusContains(t, "NORMAL", "A2", "top")
	})

	t.Run("deletes rows with dd and restores them with undo", func(t *testing.T) {
		session := startTerminalSession(t)
		session.waitForEmptySheet()

		session.fillColumnA("above", "middle", "below")
		screen := session.waitForScreen("seed rows for dd", func(screen terminalScreen) bool {
			return screen.statusContains("A3", "below") &&
				screen.screenContains("above", "middle")
		})
		screen.assertStatusContains(t, "A3", "below")

		session.sendText("kd")
		session.waitForStatus("pending dd command", "NORMAL", "A2", "d")

		session.sendText("d")
		screen = session.waitForScreen("dd deletes middle row", func(screen terminalScreen) bool {
			return screen.statusContains("NORMAL", "A2", "below") &&
				screen.screenContains("above") &&
				screen.screenOmits("middle")
		})
		screen.assertStatusContains(t, "NORMAL", "A2", "below")

		session.sendText("u")
		screen = session.waitForScreen("undo restores deleted row", func(screen terminalScreen) bool {
			return screen.statusContains("NORMAL", "A2", "middle") &&
				screen.screenContains("above", "below")
		})
		screen.assertStatusContains(t, "NORMAL", "A2", "middle")
	})
}

func startTerminalSession(t *testing.T, args ...string) *terminalSession {
	t.Helper()

	model, err := newProgramModel(args)
	if err != nil {
		t.Fatalf("create program model: %v", err)
	}

	session := &terminalSession{
		t:    t,
		term: vt10x.New(vt10x.WithSize(integrationCols, integrationRows)),
		done: make(chan error, 1),
	}

	session.program = tea.NewProgram(
		model,
		tea.WithOutput(session.term),
		tea.WithInput(nil),
		tea.WithAltScreen(),
		tea.WithoutSignals(),
	)

	go func() {
		_, err := session.program.Run()
		session.done <- err
	}()

	session.program.Send(tea.WindowSizeMsg{Width: integrationCols, Height: integrationRows})
	t.Cleanup(session.close)

	return session
}

func (s *terminalSession) close() {
	s.once.Do(func() {
		s.program.Quit()

		select {
		case err := <-s.done:
			if err != nil && s.t != nil {
				s.t.Errorf("terminal session exited with error: %v", err)
			}
		case <-time.After(integrationTimeout):
			s.program.Kill()
			if err := <-s.done; err != nil && s.t != nil {
				s.t.Errorf("terminal session kill result: %v", err)
			}
		}
	})
}

func (s *terminalSession) sendText(input string) {
	s.t.Helper()

	for _, r := range input {
		switch r {
		case '\t':
			s.sendKey(tea.KeyMsg{Type: tea.KeyTab})
		case '\n', '\r':
			s.sendKey(tea.KeyMsg{Type: tea.KeyEnter})
		default:
			s.sendKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
	}
}

func (s *terminalSession) sendTextAndEscape(input string) {
	s.t.Helper()
	s.sendText(input)
	s.sendKey(escapeKey)
}

func (s *terminalSession) enterTextAndWaitForStatus(description, input string, expected ...string) terminalScreen {
	s.t.Helper()
	s.sendTextAndEscape(input)
	return s.waitForStatus(description, expected...)
}

func (s *terminalSession) fillColumnA(values ...string) {
	s.t.Helper()

	if len(values) == 0 {
		return
	}

	s.sendTextAndEscape("ggi" + values[0])
	for _, value := range values[1:] {
		s.sendTextAndEscape("ji" + value)
	}
}

func (s *terminalSession) sendKey(msg tea.KeyMsg) {
	s.t.Helper()
	s.program.Send(msg)
}

func (s *terminalSession) waitForInitialSheet(expected ...string) terminalScreen {
	s.t.Helper()
	expected = append([]string{"NORMAL", "A1"}, expected...)
	return s.waitForStatus("initial spreadsheet render", expected...)
}

func (s *terminalSession) waitForEmptySheet() terminalScreen {
	s.t.Helper()
	return s.waitForStatus("empty spreadsheet render", "NORMAL", "A1")
}

func (s *terminalSession) waitForScreen(description string, condition func(screen terminalScreen) bool) terminalScreen {
	s.t.Helper()

	raw := s.waitFor(description, func(screen string) bool {
		return condition(newTerminalScreen(screen))
	})

	return newTerminalScreen(raw)
}

func (s *terminalSession) waitForStatus(description string, expected ...string) terminalScreen {
	s.t.Helper()

	screen := s.waitForScreen(description, func(screen terminalScreen) bool {
		return screen.statusContains(expected...)
	})
	screen.assertStatusContains(s.t, expected...)

	return screen
}

func (s *terminalSession) waitFor(description string, condition func(screen string) bool) string {
	s.t.Helper()

	deadline := time.Now().Add(integrationTimeout)
	var lastScreen string
	for time.Now().Before(deadline) {
		lastScreen = s.term.String()
		if condition(lastScreen) {
			return lastScreen
		}
		time.Sleep(pollInterval)
	}

	s.t.Fatalf("timed out waiting for %s\nlast screen:\n%s", description, lastScreen)
	return ""
}

func newTerminalScreen(raw string) terminalScreen {
	lines := screenLines(raw)

	screen := terminalScreen{raw: raw}
	if len(lines) > 0 {
		screen.status = lines[len(lines)-1]
	}
	if len(lines) > 1 {
		screen.command = lines[len(lines)-2]
	}

	return screen
}

func (s terminalScreen) statusContains(expected ...string) bool {
	return containsAll(s.status, expected...)
}

func (s terminalScreen) statusOmits(unexpected ...string) bool {
	return containsNone(s.status, unexpected...)
}

func (s terminalScreen) screenContains(expected ...string) bool {
	return containsAll(s.raw, expected...)
}

func (s terminalScreen) screenOmits(unexpected ...string) bool {
	return containsNone(s.raw, unexpected...)
}

func (s terminalScreen) commandBlank() bool {
	return strings.TrimSpace(s.command) == ""
}

func (s terminalScreen) assertStatusContains(t *testing.T, expected ...string) {
	t.Helper()
	assertContains(t, "status line", s.status, s.raw, expected...)
}

func (s terminalScreen) assertStatusOmits(t *testing.T, unexpected ...string) {
	t.Helper()
	assertNotContains(t, "status line", s.status, s.raw, unexpected...)
}

func (s terminalScreen) assertCommandEmpty(t *testing.T) {
	t.Helper()
	if !s.commandBlank() {
		t.Fatalf("expected no separate command line while typing command\nscreen:\n%s", s.raw)
	}
}

func containsAll(value string, expected ...string) bool {
	for _, part := range expected {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func containsNone(value string, unexpected ...string) bool {
	for _, part := range unexpected {
		if strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func assertContains(t *testing.T, lineName, line, screen string, expected ...string) {
	t.Helper()

	for _, part := range expected {
		if !strings.Contains(line, part) {
			t.Fatalf("expected %s %q to contain %q\nscreen:\n%s", lineName, line, part, screen)
		}
	}
}

func assertNotContains(t *testing.T, lineName, line, screen string, unexpected ...string) {
	t.Helper()

	for _, part := range unexpected {
		if strings.Contains(line, part) {
			t.Fatalf("expected %s %q to not contain %q\nscreen:\n%s", lineName, line, part, screen)
		}
	}
}

func screenLines(screen string) []string {
	screen = strings.TrimRight(screen, "\n")
	return strings.Split(screen, "\n")
}

func writeSampleCSV(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sample-data.csv")
	if err := os.WriteFile(path, []byte("1\n2\n3\n4\n=SUM(A1:A4)\n"), 0o644); err != nil {
		t.Fatalf("write sample csv: %v", err)
	}

	return path
}
