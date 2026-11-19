package app

// View is an immutable snapshot of the session, the only thing the renderer may
// read.
type View struct {
	Path string
	Text string
	// Dirty is a fact about the document, not about the user: the text differs
	// from what is on disk.
	Dirty bool
	// Confirming is true while the confirmation bar asks how to leave.
	Confirming bool
	// Notice is the last thing worth telling the user, such as a save failure.
	Notice string
}
