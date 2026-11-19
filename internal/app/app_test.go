package app_test

import (
	"errors"
	"testing"

	"github.com/mystery4f/nib/internal/app"
)

const fixturePath = "prompt.md"

// memoryFS keeps one file in memory, so a case asserts the bytes the session
// would have left on disk without touching it.
type memoryFS struct {
	files    map[string]string
	replaces int
	fail     error
}

func newMemoryFS(content string) *memoryFS {
	return &memoryFS{files: map[string]string{fixturePath: content}}
}

func (m *memoryFS) ReadFile(name string) ([]byte, error) {
	content, ok := m.files[name]
	if !ok {
		return nil, errors.New("no such file")
	}
	return []byte(content), nil
}

func (m *memoryFS) Replace(name string, data []byte) error {
	if m.fail != nil {
		return m.fail
	}
	m.files[name] = string(data)
	m.replaces++
	return nil
}

func open(t *testing.T, files *memoryFS) *app.App {
	t.Helper()
	session, err := app.Open(fixturePath, files)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return session
}

func TestSessionEndsWithAnExitCode(t *testing.T) {
	cases := []struct {
		name     string
		commands []app.Command
		wantCode int
		wantFile string
		wantWrit int
	}{
		{
			name:     "untouched document is accepted without a write",
			commands: []app.Command{{Kind: app.Quit}},
			wantCode: app.ExitAccepted,
			wantFile: "original",
		},
		{
			name:     "save keeps the session open",
			commands: []app.Command{app.Edit("draft"), {Kind: app.Save}, {Kind: app.Quit}},
			wantCode: app.ExitAccepted,
			wantFile: "draft",
			wantWrit: 1,
		},
		{
			name:     "save and quit writes once",
			commands: []app.Command{app.Edit("draft"), {Kind: app.SaveAndQuit}},
			wantCode: app.ExitAccepted,
			wantFile: "draft",
			wantWrit: 1,
		},
		{
			name:     "quit on a modified document waits for the answer",
			commands: []app.Command{app.Edit("draft"), {Kind: app.Quit}, {Kind: app.DiscardConfirmed}},
			wantCode: app.ExitDiscarded,
			wantFile: "original",
		},
		{
			name: "going back to editing keeps the edit",
			commands: []app.Command{
				app.Edit("draft"), {Kind: app.Quit}, {Kind: app.ReturnToEditing},
				{Kind: app.SaveAndQuit},
			},
			wantCode: app.ExitAccepted,
			wantFile: "draft",
			wantWrit: 1,
		},
		{
			name:     "discarding without a pending quit is ignored",
			commands: []app.Command{app.Edit("draft"), {Kind: app.DiscardConfirmed}, {Kind: app.Quit}, {Kind: app.DiscardConfirmed}},
			wantCode: app.ExitDiscarded,
			wantFile: "original",
		},
		{
			name:     "commands after the end change nothing",
			commands: []app.Command{{Kind: app.SaveAndQuit}, app.Edit("late"), {Kind: app.Quit}},
			wantCode: app.ExitAccepted,
			wantFile: "original",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := newMemoryFS("original")
			session := open(t, files)
			for _, command := range c.commands {
				session.Handle(command)
			}
			code, done := session.ExitCode()
			if !done {
				t.Fatalf("session still running after %v", c.commands)
			}
			if code != c.wantCode {
				t.Errorf("exit code = %d, want %d", code, c.wantCode)
			}
			if got := files.files[fixturePath]; got != c.wantFile {
				t.Errorf("file = %q, want %q", got, c.wantFile)
			}
			if files.replaces != c.wantWrit {
				t.Errorf("writes = %d, want %d", files.replaces, c.wantWrit)
			}
		})
	}
}

func TestQuitOnModifiedDocumentAsksFirst(t *testing.T) {
	files := newMemoryFS("original")
	session := open(t, files)
	session.Handle(app.Edit("draft"))
	session.Handle(app.Command{Kind: app.Quit})

	if _, done := session.ExitCode(); done {
		t.Fatal("session ended before the user answered")
	}
	if !session.View().Confirming {
		t.Error("confirmation bar is not asking")
	}
	if files.replaces != 0 {
		t.Errorf("writes = %d while asking, want 0", files.replaces)
	}
}

func TestFailedSaveKeepsTheSessionAlive(t *testing.T) {
	files := newMemoryFS("original")
	files.fail = errors.New("disk full")
	session := open(t, files)
	session.Handle(app.Edit("draft"))
	session.Handle(app.Command{Kind: app.SaveAndQuit})

	if _, done := session.ExitCode(); done {
		t.Fatal("session ended although the write failed")
	}
	if session.View().Notice == "" {
		t.Error("no notice about the failed save")
	}
	if got := files.files[fixturePath]; got != "original" {
		t.Errorf("file = %q after a failed save, want %q", got, "original")
	}

	files.fail = nil
	session.Handle(app.Command{Kind: app.SaveAndQuit})
	code, done := session.ExitCode()
	if !done || code != app.ExitAccepted {
		t.Fatalf("exit code = %d done = %v after retrying the save", code, done)
	}
	if got := files.files[fixturePath]; got != "draft" {
		t.Errorf("file = %q, want %q", got, "draft")
	}
}

func TestOpenRejectsAnEmptyPath(t *testing.T) {
	if _, err := app.Open("", newMemoryFS("original")); err == nil {
		t.Fatal("opening an empty path succeeded")
	}
}

func TestOpenReportsAnUnreadableFile(t *testing.T) {
	files := &memoryFS{files: map[string]string{}}
	if _, err := app.Open(fixturePath, files); err == nil {
		t.Fatal("opening a missing file succeeded")
	}
}
