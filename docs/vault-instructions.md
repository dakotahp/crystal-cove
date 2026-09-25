# Vault instructions

It is possible to inform your agents with any special instructions for your vault, such as how it is organized, what concepts it contains, or anything you want an agent to know at the start of a session. Think AGENTS.md, although an AGENTS.md file in your vault will not be automatically read over an MCP unlike in coding tool usage of a filesystem. The idea is for an agent to start with context that MCPs usually don't allow.

At startup the server reads one file from each vault root and sends it to MCP clients as the server's `instructions`, the same field other connectors use for usage guidance.

Two file names are accepted, in this order:

| File | Syncs? | Use it when |
| --- | --- | --- |
| `.mcp-instructions.md` | No | You keep the guidance on the server only. Obsidian Sync does not carry arbitrary dotfiles, so you place this one by hand. |
| `mcp-instructions.md` | Yes | You want to edit the guidance in Obsidian from any device. It is an ordinary note. |

The first one found per vault wins. Vaults without either file add nothing. If several vaults supply guidance, each section is labelled `## Vault: <name>`.

The file is re-read for each new session, so an edit made on another device reaches the next conversation once it syncs. The boot log reports how many characters were loaded at startup.

The MCP tools can read `mcp-instructions.md` but cannot create, edit, move, delete or restore it. Its text steers every later session, so a note that talks an assistant into rewriting it would steer every client from then on. Edit it in Obsidian instead.
