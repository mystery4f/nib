package document_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mystery4f/nib/internal/document"
)

// countingFS records how many times the file was replaced.
type countingFS struct {
	document.FS
	replaces int
}

func (c *countingFS) Replace(name string, data []byte) error {
	c.replaces++
	return c.FS.Replace(name, data)
}

// failingFS always refuses to read or write.
type failingFS struct{ err error }

func (f failingFS) ReadFile(string) ([]byte, error) { return nil, f.err }

func (f failingFS) Replace(string, []byte) error { return f.err }

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadDropsTheByteOrderMark(t *testing.T) {
	path := write(t, "\ufeff# title")
	doc, err := document.Load(path, document.OS{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := doc.Text(); got != "# title" {
		t.Errorf("text = %q, want %q", got, "# title")
	}
	if doc.Dirty() {
		t.Error("freshly loaded document is dirty")
	}
}

func TestSaveLeavesACleanDocumentAlone(t *testing.T) {
	path := write(t, "original")
	files := &countingFS{FS: document.OS{}}
	doc, err := document.Load(path, files)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := doc.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if files.replaces != 0 {
		t.Errorf("writes = %d for an unmodified document, want 0", files.replaces)
	}
}

func TestSaveKeepsLineEndingsAndMissingTrailingNewline(t *testing.T) {
	cases := []struct {
		name    string
		content string
		edit    string
		want    string
	}{
		{
			name:    "crlf is preserved",
			content: "one\r\ntwo\r\n",
			edit:    "one\r\ntwo\r\nthree\r\n",
			want:    "one\r\ntwo\r\nthree\r\n",
		},
		{
			name:    "no trailing newline is added",
			content: "one",
			edit:    "one two",
			want:    "one two",
		},
		{
			name:    "unicode is written unchanged",
			content: "# 标题",
			edit:    "# 标题\r\n\r\n正文 —— ok",
			want:    "# 标题\r\n\r\n正文 —— ok",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := write(t, c.content)
			doc, err := document.Load(path, document.OS{})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			doc.SetText(c.edit)
			if err := doc.Save(); err != nil {
				t.Fatalf("save: %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(raw) != c.want {
				t.Errorf("file = %q, want %q", string(raw), c.want)
			}
			if doc.Dirty() {
				t.Error("document still dirty after a successful save")
			}
		})
	}
}

func TestSaveReplacesAtomically(t *testing.T) {
	path := write(t, "original")
	doc, err := document.Load(path, document.OS{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	doc.SetText("# replaced")
	if err := doc.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "prompt.md" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("directory holds %v, want only the document", names)
	}
}

// readOnlyFS loads fine and refuses the write, which is what a full disk or a
// revoked permission looks like at save time.
type readOnlyFS struct{ document.FS }

func (readOnlyFS) Replace(string, []byte) error { return errors.New("disk full") }

func TestSaveSurfacesAWriteFailure(t *testing.T) {
	path := write(t, "original")
	files := readOnlyFS{FS: document.OS{}}
	doc, err := document.Load(path, files)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	doc.SetText("draft")
	if err := doc.Save(); err == nil {
		t.Fatal("save succeeded through a failing file system")
	}
	if !doc.Dirty() {
		t.Error("document is clean after a failed write")
	}
	if got := doc.Text(); got != "draft" {
		t.Errorf("text = %q, want the edit to survive a failed write", got)
	}
}

func TestProbeReportsAnUnwritableDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone", "prompt.md")
	if err := document.Probe(missing); err == nil {
		t.Fatal("probing a missing directory succeeded")
	}
	path := write(t, "original")
	if err := document.Probe(path); err != nil {
		t.Errorf("probing an existing file failed: %v", err)
	}
}
