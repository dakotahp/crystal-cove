package server

import (
	"fmt"
	"strings"

	"github.com/dakotahp/crystal-cove/internal/vault"
)

// InstructionsFile is the vault-root file whose contents are sent to MCP
// clients as the server's instructions. The leading dot keeps Obsidian's
// own UI from listing it as a note, but Obsidian Sync does not carry
// arbitrary dotfiles, so this one has to be placed on the server by hand.
const InstructionsFile = ".mcp-instructions.md"

// SyncedInstructionsFile is the fallback used when InstructionsFile is
// absent. It is an ordinary note, so it syncs from any device and can be
// edited in Obsidian itself.
const SyncedInstructionsFile = "mcp-instructions.md"

// loadInstructions reads InstructionsFile from each vault root. Vaults
// without the file contribute nothing, and an unreadable file is treated the
// same as a missing one: vault guidance is a convenience, never a reason to
// refuse to serve. With more than one contributing vault, each section is
// labelled so the model can tell the guidance apart.
func loadInstructions(vaults []*vault.Vault) string {
	type section struct {
		name string
		body string
	}
	var sections []section
	for _, v := range vaults {
		for _, name := range []string{InstructionsFile, SyncedInstructionsFile} {
			data, err := v.ReadAll(name)
			if err != nil {
				continue
			}
			if body := strings.TrimSpace(string(data)); body != "" {
				sections = append(sections, section{name: v.Name(), body: body})
			}
			break
		}
	}
	if len(sections) == 1 {
		return sections[0].body
	}
	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## Vault: %s\n\n%s", s.name, s.body)
	}
	return b.String()
}
