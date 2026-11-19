// Package document owns one file's content: it is read once, edited in memory,
// and replaced atomically when the session is accepted.
package document

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FS is the slice of the file system a document needs, so tests can inject
// failures such as an unwritable directory.
type FS interface {
	ReadFile(name string) ([]byte, error)
	Replace(name string, data []byte) error
}

// OS is the real file system.
type OS struct{}

// ReadFile reads the whole file.
func (OS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

// Replace writes data to a sibling temporary file, syncs it, and renames it over
// name, so a reader never observes a partially written document.
func (OS) Replace(name string, data []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(name); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".nib-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename succeeded
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, name)
}

// Probe reports whether the directory holding name accepts a sibling write. It
// runs before the window opens, so a doomed session exits instead of losing the
// edit at save time.
func Probe(name string) error {
	file, err := os.CreateTemp(filepath.Dir(name), ".nib-probe-*")
	if err != nil {
		return err
	}
	file.Close()
	return os.Remove(file.Name())
}

const bom = "\ufeff"

// Document is the file's text plus the bytes it had on disk.
type Document struct {
	path   string
	files  FS
	onDisk string
	text   string
}

// Load reads path once. A byte order mark is dropped; line endings are kept as
// they are found, because the editor writes back exactly what the user typed.
func Load(path string, files FS) (*Document, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.TrimPrefix(string(raw), bom)
	return &Document{path: path, files: files, onDisk: content, text: content}, nil
}

// Path is the file this document was loaded from.
func (d *Document) Path() string { return d.path }

// Text is the current content.
func (d *Document) Text() string { return d.text }

// SetText replaces the current content without touching the file.
func (d *Document) SetText(text string) { d.text = text }

// Dirty reports whether the text differs from what is on disk.
func (d *Document) Dirty() bool { return d.text != d.onDisk }

// Save replaces the file with the edited text. A clean document is not written
// at all, so an untouched prompt keeps its exact bytes.
func (d *Document) Save() error {
	if !d.Dirty() {
		return nil
	}
	if err := d.files.Replace(d.path, []byte(d.text)); err != nil {
		return err
	}
	d.onDisk = d.text
	return nil
}
