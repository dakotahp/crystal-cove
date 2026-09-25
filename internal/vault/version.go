package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Version identifies a text: equal texts share a version and any change
// gives a new one. Tools hand it out with what they read and take it back
// with an edit, which is refused when the text changed in between, so an
// edit built on an old read cannot silently undo a newer change.
func Version(text []byte) string {
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:8])
}

// checkVersion returns an error naming what when want is set and text is no
// longer at that version.
func checkVersion(what string, text []byte, want string) error {
	if want == "" || Version(text) == want {
		return nil
	}
	return fmt.Errorf("%s changed since version %s was read: read it again, then redo the edit on the current text", what, want)
}
