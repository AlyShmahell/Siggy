# Usage

Siggy is a local coding agent you talk to in the terminal. It works in a workspace (a folder), uses tools to read and change files, and keeps its own state in `~/.siggy`.

## Start

```bash
siggy
```

![Session](<assets/1. session.png>)

Or pass a folder: `siggy /path/to/repo`. The workspace is that folder, or the current directory if you omit it. Click the workspace name in the top bar to pick another folder later. If a saved path no longer exists, Siggy starts in the current directory.

Home is `~/.siggy` (config, sessions, memory).

## Connect a model

The first time, open settings (`⚙` in the top right) and go to **Providers**.

![Providers](<assets/2. providers.png>)

Add an OpenAI-compatible connection: name, URL, API key, and at least one model. Save. That is how providers are set; they persist for the next launch.

![Provider form](<assets/3. provider-form.png>)

Click a provider row to make it active. The model chip under the composer switches among that provider’s models.

## Session

Top bar, left to right:

- **Workspace name** — click to browse and switch folders

![Workspace](<assets/4. workspace.png>)

- **Session title** — click to return to the session
- **siggy** — brand, centered
- **`+`** — new session
- **`◷`** — session list (open, delete)

![Sessions](<assets/5. sessions.png>)

- **`⚙`** — settings (providers, version)
- **`✕`** — quit

The middle of the screen is the transcript. The box at the bottom is the composer: type a prompt and press Enter to send. While a run is in progress, **`⏹`** on the right rail cancels it.

Under the composer:

- **Mode chip** — `chat`, `plan`, or `act`

![Mode](<assets/6. mode.png>)

- **Model chip** — active provider and model

![Model](<assets/7. model.png>)

- **Usage** — token use; click for a breakdown

![Usage](<assets/8. usage.png>)

Type `@` plus a path fragment in the composer to attach a file from the workspace.

## Mouse

Siggy owns the mouse. The terminal’s usual click-to-select and right-click paste will not work while it is running. That is intentional: clicks hit the UI, not the emulator.

- Hover highlights the control under the pointer
- Left-click activates it: nav buttons, chips, lists, approvals, provider rows and icons
- Click the composer to type; click the transcript to focus it
- Drag to select text; double-click selects all
- `Ctrl+C` / `Ctrl+X` / `Ctrl+V` copy, cut, and paste through the clipboard
- Wheel scrolls whatever is under the pointer (transcript, lists, or composer)
- Right-click does nothing (no terminal menu)

## Keyboard

- **Enter** — send (composer) or confirm a list/approval
- **Ctrl+J**, **Alt+Enter**, or **Shift+Enter** — newline in the composer
- **Arrows** — move the caret; hold **Shift** to extend a selection
- **Esc** — close a popover; on an approval, deny
- **Ctrl+A** — select all
- **Ctrl+C** with a selection — copy; with none — quit
- **Ctrl+X** / **Ctrl+V** — cut / paste

## Approvals and modes

Write, shell, and network tools ask first: **allow once**, **allow session**, or **deny**. Click a choice or use arrows and Enter.

- **act** (default) — tools may run after you approve
- **plan** — those tools are blocked until you switch to act
- **chat** — conversation without steering plan vs act

`--yes` auto-approves for that process. It is not the usual way to work.

## What the agent can do

In the workspace it can read and edit files, search (grep/glob), list directories, run a shell, read PDFs, search and fetch the web, keep a todo list, and remember notes. It can also spawn focused subagents (explore, implement, review) for a slice of work.

## CLI

```bash
siggy [dir]
siggy --plan
siggy --resume <session-id>
siggy -p "prompt"          # headless, no TUI
siggy --yes                # auto-approve this process
siggy --version
```

## Typed aliases

Lines in the composer that start with `/` are shortcuts for the same UI actions. You do not need them.

`/help` `/new` `/resume` `/plan` `/act` `/allow` `/providers` `/compress` `/export` `/restore` `/branch` `/rewind` `/remember` `/forget` `/memory` `/dream` `/quit`

## MCP

Optional extra tools can be declared as `[[mcp]]` entries in `~/.siggy/config.toml`. There is no UI for this, and it is not required.
