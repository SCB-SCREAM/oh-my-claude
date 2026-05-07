# Stack templates

Each subdirectory here is one stack template (e.g. `typescript/`, `python/`,
`golang/`). A template directory is the on-disk layout omc renders into a
target project, plus a sibling `manifest.yaml` describing what triggers it
and which profiles include which files.

```
stacks/<stack>/
  manifest.yaml         # AppliesTo (signal predicates), profile membership
  CLAUDE.md.tmpl        # rendered to <project>/CLAUDE.md
  settings.json.tmpl    # deep-merged into <project>/.claude/settings.json
  hooks/                # copied to <project>/.claude/hooks/
  commands/             # copied to <project>/.claude/commands/
  agents/               # copied to <project>/.claude/agents/
  mcp.json              # suggestion fragment (commented in by user)
```

Templates are populated in milestone M6; M1 only ships the embed scaffold.
