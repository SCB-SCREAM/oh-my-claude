---
name: bubble-tea-tui
description: Patterns for writing Bubble Tea v2 programs in this repo. Use this whenever editing files under internal/tui/, designing a new TUI screen, dealing with tea.Msg routing or WindowSizeMsg, composing nested models, choosing between tea.Cmd and inline state changes, or laying out terminal UI with Lip Gloss. Critical for any work touching the TUI even if the user describes it as a small change — Bubble Tea v2 differs from v1 patterns commonly cited online.
---

# Writing Bubble Tea TUIs in this repo

Bubble Tea v2 follows the Elm Architecture (Model–View–Update). The whole runtime is one event loop: messages come in, the active `Update` produces a new model + optional `tea.Cmd`, then `View` renders. **The single most important rule: keep `Update` and `View` fast.** Anything that blocks the loop blocks every user keystroke.

## The MVU skeleton (one screen)

```go
package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)

type WelcomeModel struct {
	width, height int
	ready         bool
}

func NewWelcome() WelcomeModel { return WelcomeModel{} }

func (m WelcomeModel) Init() tea.Cmd { return nil }

func (m WelcomeModel) Update(msg tea.Msg) (WelcomeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			return m, switchTo(scanScreen) // see "Model tree" below
		}
	}
	return m, nil
}

func (m WelcomeModel) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if !m.ready {
		return v
	}
	v.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, logo)
	return v
}
```

**v2 API gotchas** (these are what trips up code copied from v1 tutorials):

- Module path is **`charm.land/bubbletea/v2`**, not `github.com/charmbracelet/bubbletea/v2`. The repo moved to `charm.land/...` after v2.
- **Key messages are split**: `tea.KeyPressMsg` for presses, `tea.KeyReleaseMsg` for releases, `tea.KeyMsg` is now an *interface* implemented by both. Switch on the concrete `tea.KeyPressMsg` unless you specifically need releases.
- **`View()` returns `tea.View`**, not `string`. Use `tea.NewView(content)` and set fields on the returned struct (`AltScreen`, `Cursor`, `WindowTitle`, etc.). Children that bubble their view up to a parent should also return `tea.View` so the parent can compose / forward without re-wrapping.
- **Alt-screen is a field on `View`**, not a `tea.WithAltScreen()` program option. Set `v.AltScreen = true` per render. (`tea.NewProgram` no longer takes `WithAltScreen`.)
- Returning a child's view directly from a parent (`return m.welcome.View()`) is fine and cheap — `View` is a struct, not an interface.

## Model tree pattern (this repo)

A real app has many screens. Compose them as nested models with a **root model that routes**. Three things the root does:

1. Handle truly global messages (`tea.WindowSizeMsg`, `Ctrl+C`).
2. Forward all other messages to the *active* child.
3. Broadcast certain messages (window size) to *every* child so caches update.

```go
type appModel struct {
	width, height int
	current       screenID
	welcome       WelcomeModel
	scan          ScanModel
	// …
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if w, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = w.Width, w.Height
		// Broadcast to all children so each can resize.
		var cmd tea.Cmd
		m.welcome, cmd = m.welcome.Update(w)
		m.scan, _ = m.scan.Update(w)
		return m, cmd
	}
	// Route to active child.
	var cmd tea.Cmd
	switch m.current {
	case welcomeScreen:
		m.welcome, cmd = m.welcome.Update(msg)
	case scanScreen:
		m.scan, cmd = m.scan.Update(msg)
	}
	return m, cmd
}

func (m appModel) View() tea.View {
	switch m.current {
	case welcomeScreen:
		return m.welcome.View()
	case scanScreen:
		return m.scan.View()
	}
	return tea.NewView("")
}
```

Have **children return their concrete model type** from `Update` (`func (m WelcomeModel) Update(msg tea.Msg) (WelcomeModel, tea.Cmd)`), not `tea.Model`. This avoids the type-assertion dance the v1 docs needed (and the `updateAs` helper they recommended). Only the **root** model has to satisfy the `tea.Model` interface for `tea.NewProgram`.

## Shared session state across screens

When data flows screen-to-screen (signals → profile → components → plan), the natural temptation is to thread every datum through a message on every transition. Don't. Stash the running picture on a `*sessionState` pointer field on `appModel`:

```go
type sessionState struct {
    RepoRoot string
    Signals  []detect.Signal
    Profile  profile.Name
    Selected map[component.ID]bool
    Plan     *apply.Plan
    Results  []apply.WriteResult
}

type appModel struct {
    state   *sessionState
    current screenID
    welcome WelcomeModel
    scan    ScanModel
    // …
}
```

The pointer lets `appModel` be copied by value (preserving the MVU contract) while every screen reads from / commits into the same struct. Rules:

1. **Only `Update` mutates `state`.** Never write to it from inside a `tea.Cmd`.
2. **Children emit decision messages** (`profileChosenMsg{...}`, `componentsCommittedMsg{Selected: ...}`) rather than poking root state directly. Root copies the decision into `state` and then emits `switchScreenMsg`.
3. **Children take their slice of state via constructor injection** (e.g. `m.profile = m.profile.WithSignals(state.Signals)` right before the screen becomes active). Tests can instantiate a single screen in isolation without a full root.

## Screen transitions via switchScreenMsg

Use one universal transition message instead of mutating `root.current` from a child:

```go
type switchScreenMsg struct{ To screenID }
func switchTo(s screenID) tea.Cmd { return func() tea.Msg { return switchScreenMsg{To: s} } }
```

The root handles `switchScreenMsg`, sets `current`, and may fire a follow-up `tea.Cmd` that produces the new screen's data (e.g. switching to `componentsScreen` fires `resolveComponentsCmd(state.Profile, state.Signals)`). Children stay agnostic of the screen enum and never call into a sibling.

**Cross-screen `tea.Cmd`s live at the root**, not in the destination screen's `Init`. The root has all of `sessionState` to read from; the child often doesn't yet. Storing the cmd-builders as `func` fields on `appModel` (defaulting to the real `session.*` implementations) gives tests a seam to inject fakes that synthesize the result message instantly.

## Command discipline

Anything that takes time — file IO, network, sleep, computing a diff over many files — runs in a `tea.Cmd`, **not** inside `Update`. A `tea.Cmd` is just `func() tea.Msg`; it runs in its own goroutine and the result lands as a message on the next loop tick.

```go
// Bad — blocks the event loop.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	signals := detect.Run(os.DirFS(m.repoRoot)) // could take seconds
	m.signals = signals
	return m, nil
}

// Good — kick off, render spinner, react to result.
type signalsMsg []detect.Signal

func runDetectionCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return signalsMsg(detect.Run(os.DirFS(root)))
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case startScanMsg:
		return m, runDetectionCmd(m.repoRoot)
	case signalsMsg:
		m.signals = msg
		m.scanning = false
	}
	return m, nil
}
```

## Window resize

`tea.WindowSizeMsg` arrives at startup and on every terminal resize. Always:

1. Store `width`/`height` on the model.
2. Propagate to every child (broadcast in the root).
3. Compute layout dimensions dynamically with `lipgloss.Height`/`Width` — **never hard-code numbers**.

```go
header := headerStyle.Render("omc — scan")
footer := footerStyle.Render("[enter] continue  [q] quit")
bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
body := bodyStyle.Height(bodyHeight).Render(m.viewport.View())
return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
```

## Message ordering

Messages produced concurrently by `tea.Cmd`s arrive in **arbitrary order**. If order matters, use `tea.Sequence(cmd1, cmd2, …)` — it runs them serially, each waiting for the previous message to be processed. User input messages remain ordered.

Use `tea.Batch` when order doesn't matter (faster, runs in parallel).

## Pointer vs value receivers

Bubble Tea's contract is: `Update` returns a *new* model. Use **value receivers** for `Update`/`View` and return `m` (modified copy). Pointer receivers are tempting for helpers, but they invite mutation across the goroutine boundary that runs `tea.Cmd`s. Keep state changes inside `Update`; never mutate the model from inside a `tea.Cmd`.

## Layout via Lip Gloss

- Use **styles as values**: declare them at package level, derive variants with `.Copy()`.
- Use `lipgloss.Place` to center.
- Use `lipgloss.JoinVertical` / `JoinHorizontal` to compose.
- Theme lives in `internal/tui/theme.go` — respect `NO_COLOR` (`lipgloss.SetColorProfile(termenv.Ascii)` when env says so).

## Multi-pane layouts (preview-style)

`lipgloss.JoinHorizontal` composes panes. Compute pane widths from `m.width` at render time — never store derived widths on the model:

```go
leftW  := m.width * 38 / 100
rightW := m.width - leftW - 2
left   := lipgloss.NewStyle().Width(leftW).Render(fileListView)
right  := lipgloss.NewStyle().Width(rightW).Render(viewport.View())
return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
```

The right pane is usually a `bubbles/v2/viewport.Model` so long content scrolls. Forward unmatched key events to the viewport via `m.vp, cmd = m.vp.Update(msg)` so `pgup/pgdn/u/d` work without you handling them by name. Don't render scrollbars by hand.

## Key matching

Bubble Tea v2's `KeyPressMsg.String()` returns the **canonical key name**, not the literal rune that arrived. A real terminal sends:

- `tea.KeyEnter` → `"enter"`. Tests must construct `tea.KeyPressMsg{Code: tea.KeyEnter}` with no `Text` field (`Text: "\n"` would override the canonical name back to `"\n"`).
- `tea.KeySpace` → `"space"`. So `case " ":` will never fire — use `case "space":`.
- Printable runes → themselves (`"q"`, `"a"`).
- Ctrl combinations → `"ctrl+c"`, `"ctrl+n"`.

When in doubt, log `msg.String()` to confirm what the runtime hands you.

## Bubble Tea v2 vs v1

- Module path is `charm.land/bubbletea/v2` (and `charm.land/bubbles/v2`). The old `github.com/charmbracelet/bubbletea/v2` redirects but `go mod tidy` rewrites it to `charm.land`. Tutorials still use the old path — they will not compile cleanly.
- v2 has improved resize handling and faster rendering. Don't fight it by re-implementing v1 patterns.
- `View()` returns `tea.View`, not `string`. Alt-screen, cursor, window title, mouse mode, focus reporting are all *fields on the View*, set per render.
- Key messages: `tea.KeyPressMsg` / `tea.KeyReleaseMsg` are concrete; `tea.KeyMsg` is an interface. Switch on the press type unless you need releases.
- Synchronized output ("Mode 2026") is on by default; you generally don't need to think about it.

## This repo's TUI shape

```
internal/tui/
  app.go         root model: routes messages, owns child models, app loop entry (Run)
  welcome.go     screen 1
  scan.go        screen 2 (kicks off detect.Run as tea.Cmd)
  profile.go     screen 3
  components.go  screen 4 (checklist)
  preview.go     screen 5 (file diffs in viewport)
  apply.go       screen 6 (progress as cmds complete)
  theme.go       Lip Gloss styles + NO_COLOR handling
```

Add a new screen by: defining the model in its own file, exposing a `New<Name>()` constructor, adding a field to `appModel`, adding a case to the routing `switch`, and emitting `switchTo(<screen>)` from the previous screen.

## Don't

- Don't read files or hit the network from `Update`/`View` — only from `tea.Cmd`.
- Don't store derived layout sizes; recompute in `View`.
- Don't store `*tea.Program` on a model. The model talks to the runtime through returned `tea.Cmd`s, never directly.
- Don't `os.Exit` from a screen. Return `tea.Quit`.
