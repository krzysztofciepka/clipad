# AI Shortcut Library — Design

**Date:** 2026-04-19
**Status:** Approved

## Summary

Define a curated library of twenty-three AI shortcuts tailored to the user's writing workflow (software requirements, todos, tech notes, general polish, formatting), and ship that library two ways:

1. **Local config write** — populate the user's existing `~/.config/clipad/ai_shortcuts.toml` with the full set right now.
2. **Built-in defaults in clipad** — embed the same TOML in the clipad binary via `go:embed`, and have `loadShortcuts` seed the user's config file with these defaults on first run (when the file is missing). Subsequent users of clipad get the library out of the box, can edit it freely, and never have it overwritten.

## Background

Clipad's AI shortcut system is a small piece of the plugin layer:

- Storage: `~/.config/clipad/ai_shortcuts.toml`, schema `[[shortcuts]]` with `name` (string) and `prompt` (string).
- Invocation: opened via the plugin selector; the model receives a fixed system prompt — *"You are a text processing assistant. Apply the following instruction to the provided text. Return ONLY the processed text, nothing else."* — followed by `Instruction: <prompt>\n\nText:\n<note content>`.
- Result handling: side-by-side diff with `y` to accept (replaces note) or `n` to reject.

Only one shortcut exists today: `prd`, which converts informal text into a professional product requirements document with explicit `TBD` markers for unspecified pieces.

## Goals

1. Provide twenty-two additional shortcuts covering the user's three primary writing modes plus general utilities and formatters.
2. Each prompt is precise about output format, what to preserve, and what to rewrite — no vague "make it better" instructions.
3. Mix three behavioral kinds:
   - **Heavy transforms** that replace the note with a new structure.
   - **Additive helpers** that augment the note without touching the original wording.
   - **Pure formatters** that restructure layout without rewriting content.
4. Install directly into the user's local `~/.config/clipad/ai_shortcuts.toml`, preserving the existing `prd` entry as the first shortcut.
5. Ship the same library as the built-in default set in clipad, so any user who has not configured shortcuts gets the full library on first run.

## Non-goals

- No new shortcut categories beyond the six in this spec (Requirements, Todos, Tech Notes, Universal Utilities, Formatting). Formats the user said they don't write (ADR, runbook, RFC, postmortem) are explicitly excluded.
- No syncing, sharing, or export of the shortcut file.
- No "restore defaults" command. Once a user has a config file (even an empty one), they own it. If they want defaults back they delete the file.
- No first-run wizard step or interactive prompt for shortcuts. Seeding is silent.

## Design

### Categorization

Shortcuts are grouped into six logical categories. Within the TOML file they appear in the order: existing `prd` first, then category by category as listed below.

### Behavioral kinds

Each shortcut is one of three kinds, signaled in the prompt itself:

- **Transform** — instructs the model to restructure freely; the note's existing wording is not preserved.
- **Additive** — instructs the model to insert new content (a section, inline annotations, a code block) while keeping the original text intact.
- **Formatter** — instructs the model to change layout/syntax only, preserving content verbatim.

The user's stated rule — *"do not preserve wording if something better is possible"* — applies to transforms. Additive and formatter shortcuts intentionally preserve wording because preservation is the value they offer.

### Voice rule for transforms

Transform-class prompts explicitly say "rewrite freely for clarity." This is the contract that lets the model improve phrasing rather than mechanically reorder.

### Prompt design conventions

Every prompt:

1. Names the action in the imperative ("Convert", "Extract", "Restructure").
2. Specifies the output shape (markdown headings, checkbox list, Gherkin scenarios, table, fenced code block, etc.).
3. Says what to preserve and what to change.
4. For additive/formatter prompts, includes an explicit "do not modify" / "do not rewrite" clause.
5. Where applicable, includes an escape hatch for input that doesn't fit (e.g., `table` returns the text unchanged if the content is not parallel; `diagram` returns it unchanged if no diagram helps).

### The shortcuts

#### Existing — preserved

**`prd`**
> Turn the text into professional product requirements document. Add TBDs for things that are not clear but relevant in your view to have a complete and full spec

#### Requirements (3 transforms)

**`userstory`**
> Convert the text into one or more user stories in the format "As a <role>, I want <capability>, so that <benefit>." Below each story add 3-6 acceptance criteria as a bulleted list. Group related stories under a short heading. Rewrite freely for clarity.

**`acceptance`**
> Extract or write acceptance criteria for the feature described in the text. Output as Gherkin scenarios using "Scenario:", "Given", "When", "Then" (and "And" for additional steps). Cover the happy path plus important edge cases and error conditions. If the text is too vague to derive a scenario, list missing information under an "Open questions" heading at the end.

**`critique`**
> Review the text as if it were a draft spec or design doc. Output a structured critique with these sections: "Ambiguities" (statements that could be interpreted multiple ways), "Missing edge cases" (scenarios not addressed), "Hidden assumptions" (things presented as obvious that may not be), "Contradictions" (parts that conflict with each other). Quote the specific sentence under each finding. If a section has no findings, omit it.

#### Todos (3 transforms)

**`todos`**
> Extract every actionable item from the text and output as a markdown checkbox list (- [ ] item). Phrase each item as a concrete verb-led action. Group related items under short bold headings if there are obvious clusters. Drop items that are observations, decisions already made, or aspirations without an action — only true todos.

**`prioritize`**
> Take the todo items in the text and re-rank them by impact and effort. Output three sections: "## Now" (high-impact, ready to start), "## Next" (important but blocked or lower urgency), "## Later" (nice-to-have or low-impact). Within each section use a markdown checkbox list. After each item, append a short " — <reason>" explaining the placement. Drop duplicates and merge near-duplicates.

**`breakdown`**
> Take the high-level task or goal in the text and decompose it into concrete subtasks. Output as a hierarchical markdown checkbox list, with sub-items nested under their parent. Each leaf should be small enough to complete in one sitting. Add a short "## Open questions" section at the end for anything that needs to be resolved before starting.

#### Tech notes (2 transforms)

**`onboard`**
> Restructure the text as an onboarding document for an engineer who has never seen this system. Use these sections in order: "## What it is" (one paragraph), "## Why it exists" (problem it solves), "## Mental model" (the core concepts and how they relate), "## How to use it" (the typical workflow), "## Where the code lives" (key files/modules if mentioned), "## Common gotchas" (pitfalls to avoid). Rewrite freely; assume the reader has solid general engineering background but zero context on this project.

**`explain`**
> Restructure the text as a clean explainer of how the system works. Open with a one-paragraph summary. Then build up understanding from the ground up: introduce concepts before they are used, show the simple case before complications, and end with a section on edge cases or surprising behavior. Use short sections with descriptive headings. Add concrete examples inline. Rewrite freely for clarity.

#### Universal utilities (8 — mix of transforms and additives)

**`tighten`** *(transform)*
> Tighten the text. Cut filler, hedging, redundancy, and throat-clearing. Keep all substantive points. Do not add new information, do not change the meaning, do not change the document structure. Aim for roughly 60-70% of the original length.

**`tldr`** *(additive)*
> Add a "## TL;DR" section at the very top of the document containing 1-3 sentences (or a short bullet list if the content is heterogeneous) that capture the key takeaway. Keep the rest of the document unchanged.

**`outline`** *(transform — produces a view, not an edit)*
> Produce a hierarchical outline of the text as a nested markdown bullet list. Each bullet should be a short noun phrase or sentence fragment naming a topic, not a summary. Mirror the logical structure even if the source is not well-organized.

**`questions`** *(additive)*
> Extract every open question, uncertainty, "TBD", or follow-up implied by the text. Output as a markdown bullet list under "## Open questions". For each, add a short " — <context>" pointing to where it came from. Include both explicit questions and ones clearly implied by gaps in the text.

**`examples`** *(additive)*
> Find abstract or generic claims in the text and add concrete examples that illustrate them. Insert each example inline, immediately after the claim it illustrates, prefixed with "Example:" on its own line. Keep all original wording. Do not invent facts about the system being described; if you cannot ground an example in the source, use a generic but realistic scenario.

**`diagram`** *(additive)*
> Identify parts of the text that would be clearer with a diagram (sequence of steps, state machine, component relationships, data flow). For each, insert a Mermaid code block at the appropriate spot in the text, choosing the right diagram type (flowchart, sequenceDiagram, stateDiagram, classDiagram). Keep the surrounding prose intact. If nothing in the text benefits from a diagram, return the text unchanged.

**`glossary`** *(additive)*
> Identify domain-specific terms, jargon, and acronyms used in the text. Add a "## Glossary" section at the end with each term as a bold entry followed by a one-sentence plain-language definition. Order alphabetically. Do not modify the original text.

**`risks`** *(additive)*
> Extract risks, pitfalls, edge cases, failure modes, and gotchas from the text — both ones explicitly mentioned and ones a careful reader would infer. Output as a markdown bullet list under "## Risks & gotchas". For each, add a short note on what could trigger it and, if obvious, how to mitigate.

#### Formatting (6 formatters)

**`bullets`**
> Convert the text into a markdown bullet list. Split by topic or item — one bullet per discrete point. Do not rewrite content beyond minor cleanup needed to make each item read well as a standalone bullet. Preserve order. If the source already uses bullets, only fix formatting (indentation, marker consistency).

**`steps`**
> Convert the text into a numbered markdown list of sequential steps (1., 2., 3., …). Use this when the content describes an ordered procedure. Each step starts with an imperative verb. Preserve order and original wording where possible; tighten only for clarity. If a step has substeps, nest them as a bullet sublist.

**`table`**
> Convert the text into a markdown table when the content has parallel structure (multiple items sharing the same attributes). Choose columns from the attributes that appear repeatedly. Emit rows in source order. Right-align numeric columns. If the content does not lend itself to a table, return the text unchanged.

**`headers`**
> Add markdown headers (## and ###) to organize the text by topic. Do not rewrite paragraphs; only insert section headers and small whitespace fixes. Use sentence case. Choose header levels that reflect logical hierarchy. Do not add a top-level # title unless the text clearly lacks one.

**`fmtjson`**
> Pretty-print every JSON object or array in the text with 2-space indentation and sorted keys where the source order is not meaningful. Preserve everything else verbatim. If a fenced code block contains JSON but lacks the json language tag, add it. If a JSON-looking blob is loose in the text, wrap it in a fenced code block tagged json.

**`markdown`**
> Clean up the markdown formatting of the text without changing any content: fix list indentation, normalize bullet and number markers, fix heading levels, close unclosed code fences, escape stray characters, fix broken link syntax, collapse consecutive blank lines. Do not rewrite, summarize, or remove anything.

## Implementation

### Part 1 — Local config write (already done)

Wrote a complete twenty-three-entry `~/.config/clipad/ai_shortcuts.toml` with the `prd` shortcut at index 0 followed by the twenty-two new entries in category order (Requirements → Todos → Tech notes → Universal utilities → Formatting). Verified by parsing with Python `tomllib` and by re-loading with the same Go TOML library clipad uses. The previous file is preserved as a timestamped backup.

### Part 2 — Built-in defaults in clipad

The same TOML content ships in the clipad binary as the embedded default set, and `loadShortcuts` is taught to seed the user's config file from those defaults the first time it is called against a missing file.

**File layout in the repo:**

- `defaults/ai_shortcuts.toml` — single source of truth for the default library; identical content to what was written to the user's config.
- `shortcuts.go` — adds `import _ "embed"`, declares `//go:embed defaults/ai_shortcuts.toml` `var defaultShortcutsTOML []byte`, and modifies `loadShortcuts` per the semantics below.
- `shortcuts_test.go` — adds tests for the new behavior; the existing `TestLoadShortcuts_Missing` test flips to assert the new seed-on-missing semantics.

**`loadShortcuts` semantics:**

| State of `~/.config/clipad/ai_shortcuts.toml` | Behavior |
|---|---|
| File does not exist | Seed: `MkdirAll` the parent dir, write `defaultShortcutsTOML` to the path, then parse and return the embedded bytes. |
| File exists with valid TOML containing N shortcuts | Return those N shortcuts unchanged. Never overwrite. |
| File exists but parses to zero shortcuts (empty file, comments only, or `shortcuts = []`) | Return zero shortcuts. Never reseed — the user has expressed an intent to have none. |
| File exists with invalid TOML | Return a parse error, as today. Do not reseed (would mask the user's broken file). |

The seed write is silent — no log line, no UI prompt. If `MkdirAll` or `WriteFile` fail, surface the error with `fmt.Errorf("seeding shortcuts: %w", err)` so the user can see why.

**Test plan (table-driven would also work; flat tests are fine for clarity):**

1. `TestDefaultShortcutsEmbeddedTOMLParses` — `toml.Unmarshal(defaultShortcutsTOML, &cfg)` succeeds, `len(cfg.Shortcuts) == 23`, names match the spec's order, every entry has a non-empty name and prompt.
2. `TestLoadShortcuts_SeedsWhenMissing` (renames/replaces the existing `TestLoadShortcuts_Missing`) — point `XDG_CONFIG_HOME` at `t.TempDir()`, call `loadShortcuts`, assert it returns 23 entries and that the file now exists at the expected path with the embedded bytes.
3. `TestLoadShortcuts_DoesNotOverwriteExisting` — pre-populate the temp config dir with a one-entry file, call `loadShortcuts`, assert the returned slice has that one entry and the file content is byte-for-byte unchanged.
4. `TestLoadShortcuts_KeepsExplicitlyEmpty` — pre-populate the temp config dir with a comment-only file (zero shortcuts), call `loadShortcuts`, assert the returned slice has length 0 and the file content is unchanged.

The existing `TestSaveAndLoadShortcuts` is unaffected because `saveShortcuts` runs before `loadShortcuts`, so the file always exists at load time.

**README update:** add a one-paragraph entry under the Plugins section describing the AI shortcut library, where the file lives, that defaults seed on first run, and how to customize. List the 23 default names so users can scan what they get without opening the file.

**Backwards compatibility:** any existing user with their own `ai_shortcuts.toml` is unaffected — the seed only runs when the file is missing. Users with no file (which previously meant "no shortcuts available") now get the full default library; this is a feature, not a regression, but worth flagging in the release notes.

## Risks and considerations

- **Prompt drift over time.** Prompts that work well today may degrade as models change. Each prompt is small and self-contained, so individual edits are cheap.
- **Picker length.** The plugin selector renders shortcuts as a flat list. Twenty-three entries is scrollable but not overwhelming; if it becomes unwieldy, a future iteration could add categorization to the picker (out of scope here).
- **Overlap.** `acceptance` and `userstory` both produce requirement-shaped output but in different formats; `outline` and `tldr` both summarize but at different granularities. These overlaps are intentional — different shapes serve different stages of work.
- **Additive vs. transform mismatch.** Running an additive shortcut twice will duplicate its added section. The diff/accept flow makes this visible before the user commits.
- **Default seed surprise.** Existing clipad users who previously had no shortcut file will silently get a populated one on next launch. This is documented in the release notes and visible in the picker; the file is theirs to edit or delete.
- **Defaults drift between repo and local config.** The user's local config and the embedded defaults are now two copies of the same content. Drift is intentional — once a user customizes, the embedded defaults stop applying to them. The repo's `defaults/ai_shortcuts.toml` is the authoritative version going forward; updates land there and ship via release.
