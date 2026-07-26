# Clipboard Images in clipad — Design

**Date:** 2026-07-03
**Status:** Approved, ready for planning
**Task:** P6 — paste images from the clipboard into markdown notes, rendered inline as a single removable element.

## Summary

Extend clipad so a user can copy an image to the system clipboard, place the cursor on its own line in a note, press **Ctrl+V**, and have the image appear **inline as real pixels** in the editor. The image behaves as a single atomic element: the cursor lands before or after it, Backspace/Delete removes it whole, and Cut/Copy of the lone element writes the real image bytes back to the system clipboard so it round-trips into any app or note.

Storage is file-backed but the *behavior* is inline: pasting saves the bytes to a content-addressed file under `assets/` at the vault root and inserts a short markdown link (`![](assets/img-…png)`) into the `.md`. Notes stay small and index-friendly; the picture is rendered from the referenced file.

Rendering targets the **kitty graphics protocol** using its **Unicode placeholder** mechanism, which lets images live in the text grid and scroll/reflow naturally under Bubble Tea's full-frame redraw. Terminals without kitty-graphics support fall back to a text placeholder chip; the data is still stored and links still work.

## Goals

- Paste a clipboard image into a note with Ctrl+V and see it rendered inline in the editor.
- The image renders as pixels in the Ctrl+P glamour preview as well.
- The image is a single atomic element for cursor movement, Delete/Backspace, Cut, and Copy.
- Copy/Cut of a lone image element puts the real image bytes back on the system clipboard.
- Notes remain plain markdown with short `![](assets/…)` links; image bytes live on disk.
- Graceful degradation on terminals/environments without the required capabilities.

## Non-goals (this iteration)

- Sixel or iTerm2 graphics protocols (kitty-protocol only; text-placeholder fallback elsewhere).
- Image resizing, cropping, or editing.
- Drag-and-drop of image files from the filesystem.
- Orphaned-asset garbage collection (deleting an element leaves its file in place).
- Non-PNG clipboard formats beyond what `wl-paste`/`xclip` expose as `image/png`.

## Architecture

clipad is a single-package (`package main`) Bubble Tea application. Notes are plain `.md` files in a vault directory; there is no database for note content (the sqlite `index.db` holds only a derived semantic-search embedding index). The editor is a custom `SelectableEditor` (`selection.go`) wrapping `bubbles/textarea` with a fully custom per-rune `render()` (`selection.go:620`). Markdown preview uses glamour into a `viewport.Model` (`preview.go`). Text clipboard uses `atotto/clipboard` (text-only).

This feature adds three concerns, each in its own new file, plus targeted edits to the editor, preview, model dispatch, and startup:

| Concern | Location |
|---|---|
| Clipboard image I/O (probe/read/write image bytes, env detection) | new `imageclip.go` |
| Kitty graphics rendering (transmit, Unicode-placeholder cell blocks, sizing) | new `imagerender.go` |
| Image-element model (detect link lines, atomic navigation, element metadata) | new `imageelement.go` |
| Paste/copy/cut dispatch | edits in `model.go` (Ctrl+V/C/X handlers) and `selection.go` |
| Inline render splice | edits in `selection.go` `render()` and `preview.go` |
| Capability detection at startup | edits in `main.go` |

### Data model

An **image element** is a single buffer line matching the markdown image-link pattern whose target resolves to a readable image file:

```
![](assets/img-2026-07-03-a1b2c3d4.png)
```

- `assets/` lives at the **vault root**. The link is written **relative to the note file's directory** so the markdown is portable and standard.
- Filenames are **content-addressed**: `img-<YYYY-MM-DD>-<shortsha>.png` where `<shortsha>` is the first 8 hex chars of `sha256(bytes)`. Pasting identical bytes (including cut-then-paste-back) reuses the existing file — no duplicates.
- The element is detected structurally at render/navigation time (regex on the line, then resolve+stat the target). Nothing about the element is persisted beyond the markdown link itself.

### Paste pipeline (Ctrl+V)

1. **Probe**: does the clipboard hold an image?
   - Wayland (`$WAYLAND_DISPLAY` set): `wl-paste --list-types` contains `image/png`.
   - X11: `xclip -selection clipboard -t TARGETS -o` contains `image/png`.
2. **No image** → fall through to the existing text `editor.Paste()` (unchanged behavior).
3. **Image present**:
   - Read raw bytes: `wl-paste --type image/png` or `xclip -selection clipboard -t image/png -o`.
   - `hash := sha256(bytes)`; compute `assets/img-<date>-<short>.png`.
   - Ensure `<vault>/assets/` exists; write the file if absent (skip if the content-addressed file already exists).
   - Insert a new line `![](<relpath>)` at the cursor via the existing undo-wrapped insert path.
4. **Image present but no clipboard tool installed** → status-line hint: `install wl-clipboard or xclip to paste images`. No insertion.

### Copy / Cut pipeline (Ctrl+C / Ctrl+X)

The selection scope decides the behavior:

- **Lone image element** (cursor on an image-element line with no active text selection, or the active selection is exactly that one element):
  - Read the referenced `assets/` file bytes and **write them to the system clipboard as `image/png`** (`wl-copy --type image/png` / `xclip -selection clipboard -t image/png -i`).
  - For **Cut**, then delete the element line (undo-wrapped).
  - This lets the image paste into any external app and round-trip back into clipad (§paste re-saves idempotently via content-addressing).
- **Mixed selection** (a text range that includes an image link among other text): **normal text copy** of the markdown, exactly as today. The `![](…)` link travels as text and still renders on paste because the asset file exists.

Writing the system clipboard as an image requires `wl-copy` (part of `wl-clipboard`) or `xclip`; if unavailable, fall back to copying the markdown link text and show the same install hint.

### Rendering — kitty Unicode placeholders

To render pixels inside a full-redraw TUI without corruption on scroll/reflow, use the kitty graphics protocol's **Unicode placeholder** feature rather than absolute cursor positioning:

1. **Transmit once**: on first sight of an image (keyed by content hash → stable image ID), transmit the PNG bytes to the terminal with an APC `_G` escape (`a=t`, `q=2` quiet, chunked base64 payload as the protocol requires). A process-lifetime cache maps content-hash → image ID so identical images transmit once.
2. **Place via placeholders**: the element's reserved rows are filled with the placeholder character `U+10EEEE`, encoded with the image ID and per-cell row/column via combining diacritics (the kitty Unicode-placeholder scheme). The terminal paints the image into exactly those cells.
3. **Natural motion**: placeholders are ordinary characters in the emitted frame, so Bubble Tea's normal full-frame redraw makes the image scroll, reflow, and clip with surrounding text — no manual clearing.

**Sizing**: fit the image to the editor content width, capped at a maximum height (~12 rows), preserving aspect ratio. Aspect ratio uses the terminal cell pixel size from `TIOCGWINSZ` (`ws_xpixel/ws_ypixel`); if unavailable, assume a conventional cell aspect (~1:2 width:height). Reserve exactly the computed number of rows for the element in both the editor and preview layouts.

The same placeholder cell-block builder is spliced into:
- the editor's custom `render()` (`selection.go:620`): when a visible line is an image element, emit the reserved placeholder rows instead of the raw link text;
- a new post-process step in `preview.go`: after glamour renders (glamour emits only alt-text/links for images), replace the image link's rendered output with the placeholder block before the viewport displays it.

### Atomic-element behavior in the editor

- **Cursor navigation**: Left/Right/Up/Down and Home/End treat the element line as a single stop — the cursor rests at the start or end of the line but never inside the link text. Vertical movement skips over the reserved image rows as one row of travel.
- **Delete/Backspace**: when the cursor is adjacent to an element, the whole element line is removed as one undo-able operation.
- **Selection**: extending a selection across an element includes the entire element line; rendering shows the element as selected (e.g. a highlighted border/overlay around the reserved block).

### Capability detection & fallback

At startup — alongside the existing `termenv` dark-background probe in `main.go`, before Bubble Tea claims stdin — detect kitty-graphics support (kitty graphics capability query and/or `$KITTY_WINDOW_ID`/`$TERM`/terminfo signals). Store `supportsKittyGraphics bool` on the model.

When **kitty graphics is unavailable**, image elements render everywhere as a compact one-line placeholder chip: `🖼 image (<name>)`. The element remains fully atomic and cut/copy/delete-able; only the pixels are absent. The same chip is used if the referenced asset file is missing (e.g. moved vault). The link and file are untouched, so opening the same note in a kitty terminal later renders the pixels.

### Semantic-search index

No special handling required. Notes contain only short `![](assets/…)` links (no base64), which are negligible in the `chunkFile` embedding pipeline (`index.go`). Left as-is.

## Error handling

- Clipboard tool missing (read or write) → status-line install hint; no data loss.
- `assets/` not writable → status-line error; no insertion.
- Asset file referenced by a link is missing at render time → placeholder chip, no crash.
- Terminal lacks kitty graphics → placeholder chip everywhere.
- Corrupt/oversized clipboard image → guard with a size sanity cap; on decode/read failure, status-line error and no insertion.

## Testing strategy

Use TDD for the pure, terminal-independent logic; use thin wrappers + a manual checklist for the terminal/clipboard boundaries.

**Unit-tested pure functions:**
- image-link line detection & parsing (`imageelement.go`).
- content-addressed filename derivation from bytes (hash → path).
- relative-path computation (asset path relative to a note in a subdir) and resolution back to absolute for reading.
- sizing math (bytes/aspect + cell pixel size → rows/cols, with the fallback ratio).
- kitty transmit-escape and Unicode-placeholder cell-block builders — asserted against exact expected byte output.
- atomic cursor-navigation logic (given a buffer and cursor, next/prev stop skips element interior).
- copy/cut scope decision (lone element vs mixed selection) as a pure predicate over selection state.

**Thin wrappers (logic factored out, wrapper kept minimal):**
- clipboard shell-out (probe/read/write) — the command construction is pure and tested; execution is the wrapper.

**Manual verification checklist (real kitty-protocol terminal):**
1. Copy a screenshot, Ctrl+V → image renders inline; `assets/` file created; `.md` has the link.
2. Scroll the note past the image and back → image redraws correctly, no artifacts.
3. Resize the terminal → image reflows/rescales without corruption.
4. Cursor onto the element, Delete → element removed as one step; Undo restores it.
5. Copy the lone element, paste into an external image app → real image appears.
6. Copy the lone element, paste back into another note → image re-renders (idempotent asset reuse).
7. Ctrl+P preview → image renders as pixels.
8. Run in a non-kitty terminal → placeholder chip; cut/copy/delete still work.
9. Uninstall/absent `wl-clipboard`/`xclip` → install hint on image paste; text paste unaffected.
