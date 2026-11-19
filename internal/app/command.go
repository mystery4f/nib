// Package app is Nib's whole state machine: one document, a command in, an exit
// code out. The UI projects it; nothing here imports the UI.
package app

// Exit codes are the contract with the caller. pi adopts the file on 0 and keeps
// the prompt it already had on anything else.
const (
	// ExitAccepted means the document was written, or was left alone because it
	// was never modified.
	ExitAccepted = 0
	// ExitDiscarded means the session ended without writing the file.
	ExitDiscarded = 1
	// ExitFailed means the editor never started.
	ExitFailed = 2
)

// Kind names a command's intent.
type Kind int

const (
	// EditText replaces the document text with the command's Text.
	EditText Kind = iota
	// Save writes the document and stays.
	Save
	// SaveAndQuit writes the document and ends the session.
	SaveAndQuit
	// Quit asks to end the session; a dirty document raises the confirmation bar.
	Quit
	// DiscardConfirmed ends the session without writing.
	DiscardConfirmed
	// ReturnToEditing closes the confirmation bar.
	ReturnToEditing
)

// Command is one intent, carrying the text that belongs to EditText.
type Command struct {
	Kind Kind
	Text string
}

// Edit builds the command the source editor fires after every keystroke.
func Edit(text string) Command { return Command{Kind: EditText, Text: text} }
