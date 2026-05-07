---
name: go-project-layout
description: Conventions for where Go code lives in this repo — cmd/<binary> for entry points, internal/ for non-importable packages, pkg/ only when truly shared. Use this skill before creating a new directory or package, when moving code between cmd/ and internal/, when uncertain whether something belongs in internal/ or pkg/, or when naming a new package. Apply whenever a task adds new files in a place the project hasn't yet established.
---

# Go project layout in this repo

This repo follows the conventional Go CLI layout: tiny `cmd/`, fat `internal/`, no `pkg/` (yet). Don't fight it.

## The layout

```
oh-my-claude/
├── cmd/
│   └── omc/
│       ├── main.go        // wires everything; almost no logic
│       ├── init.go        // `omc init` subcommand
│       ├── doctor.go      // `omc doctor` subcommand
│       └── stacks.go      // `omc stacks` subcommand
├── internal/
│   ├── detect/            // file-only stack detection
│   ├── templates/         // embedded per-stack templates
│   ├── profile/           // profile → component lists
│   ├── component/         // Component type + manifest loader
│   ├── apply/             // writer, differ, settings_merge
│   ├── tui/               // Bubble Tea v2 screens
│   └── version/           // build-stamped version
├── testdata/              // fixtures + goldens (Go toolchain ignores)
├── go.mod
└── …
```

## Rules

### `cmd/<binary>/` is wiring only

`main.go` should:

- Set up the Cobra root.
- Register subcommands (one per file).
- Translate flags into calls into `internal/...`.

It should **not** contain detection logic, template rendering, file IO beyond reading flags, or Bubble Tea models. If you find yourself writing >50 lines of business logic in `cmd/`, that logic belongs in `internal/`.

### `internal/` is the home for everything else

Anything Go's `internal` mechanism needs to keep private from external importers lives here. The compiler enforces this — packages under `internal/` can only be imported by packages rooted at the parent of `internal/`. We *want* that constraint: nothing in this binary's logic is meant to be a public library.

### `pkg/` — don't add it (yet)

`pkg/` is for code you intend to be importable by *other* projects. We are not a library. If a piece of code starts looking genuinely reusable (e.g. a generic Claude Code `settings.json` deep-merge that another tool would want), then **and only then** consider promoting it into `pkg/<name>/`. Until that day, every new package goes in `internal/`.

### One concept per package

A package is a cohesive unit of meaning, not a folder of utilities. Concrete tests:

- Can you describe the package in one sentence? Good.
- Does the package name appear naturally in callers (`detect.Run`, `apply.Plan`, `profile.Resolve`)? Good.
- Is it called `util/`, `helpers/`, `common/`, `misc/`? Bad. Find the real concept or inline into the caller.

### Package naming

- Short, lowercase, **no underscores** or camelCase: `detect`, `apply`, `tui`, `version`.
- Singular by default (`apply`, not `applies`). Match the dominant noun/verb of the package's API.
- Avoid stuttering: `detect.Detector` is fine, `detect.DetectDetector` is not.
- Don't repeat the binary name (`omcdetect`) — the import path already disambiguates.

### File naming inside a package

- One file per cohesive type or per subcommand: `internal/detect/typescript.go`, `internal/detect/python.go`, `cmd/omc/init.go`.
- Tests next to source: `typescript.go` ↔ `typescript_test.go`.
- Cross-cutting interface/registry lives in a file named after the package (`detect/detector.go`) or `<package>.go`.

### Dependency direction

Dependencies flow **downward** from `cmd/` through `internal/`. Never reverse:

- `cmd/` may import any `internal/*`.
- `internal/tui` may import `internal/detect`, `internal/apply`, etc. (it orchestrates).
- `internal/detect` should not import `internal/tui` or `internal/apply` — it's a leaf, working on `fs.FS` and returning data.
- `internal/apply` should not import `internal/tui` — `apply` builds and executes plans; the TUI calls into it.
- Shared types (e.g. `detect.Signal`, `component.Component`) live in their own package and are imported by callers.

If you find yourself wanting to import "up" the tree, the type is in the wrong package — push it down to a leaf.

## When you genuinely need a new package

Ask yourself, in order:

1. **Can this be a function or a type in an existing package?** Usually yes. Default to "no new package".
2. **Will this package have at least 2–3 distinct exported symbols?** A package that exports one function is almost always premature.
3. **Does it have a meaningful name that isn't a synonym for an existing package?**
4. **Does it own its data?** A package that just operates on another package's types belongs in that other package.

If all four pass, create the directory under `internal/<name>/` with a single file `<name>.go` and grow from there. Add tests in the same step.

## When NOT to add a package

- "Things will get big later, let's pre-split." No. Split when there's a concrete reason (an import cycle, a clear seam, a test boundary).
- "I want a `utils/` for helpers." No. Inline helpers next to their users until a clear concept emerges.
- "Each subcommand should be its own package." No — subcommand wiring lives in `cmd/omc/<name>.go`, business logic lives in `internal/`.

## Moving code

When refactoring across packages:

1. Move the code first, fix imports, run `go build ./...`.
2. Run `go test ./...`.
3. Commit the move *separately* from any behavior changes — keeps diffs reviewable.

## What about `pkg/internal/`?

The `golang-standards/project-layout` repo proposes `internal/pkg/` for shared internal libraries. We don't need that here. If two `internal/*` packages need a shared helper, put it in a new top-level `internal/<helpername>/` package. Avoid nesting.
