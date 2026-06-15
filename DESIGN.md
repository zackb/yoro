# Yoro Design Notes

Forward-looking design decisions that are not yet implemented. Milestone 1 is a
read-only browser; this document records how planned work fits the existing
architecture so we can pick it up later without re-deriving it.

## Vim-style editing (with visual mode)

Yoro edits **structured records** (vCard fields, iCal event fields), not freeform
text, so "vim editing" splits into two layers designed separately.

### Layer 1 — modal operations over records & fields

This is the yazi-style layer and maps cleanly onto bubbletea, which is already a
modal state machine; adding modes is idiomatic. Introduce an edit-mode enum
(`Normal`, `Insert`, `Visual`, `VisualLine`, `Command`).

- **Normal** over a field list or the agenda/contact list: `j/k` move; `i`/`a`/`c`
  edit the field under the cursor; `o`/`O` add a field (new email, new phone);
  `dd` delete a field; `cc` replace a value.
- **Visual** = range-select, like yazi's file selection:
  - In the **list** (events/contacts): `V` selects a range of records → `d`
    bulk-delete, `y` yank, `m`/`c` move to another collection.
  - In a **record's field list**: `V` selects a range of fields to delete/yank.
- **Command** `:w` / `:q` / `ZZ` to save/quit; `u` / `ctrl+r` undo (an in-memory
  snapshot stack per record taken before each save).

### Layer 2 — vim text editing inside one field value

When editing a value (an email, a NOTE/DESCRIPTION):

- **Single-line fields** (name, email, phone) need little — a plain insert-mode
  line editor covers nearly everything.
- **Multi-line fields** (NOTE, DESCRIPTION): `bubbles/textarea` provides editing
  but not vim motions, so true vim-in-field means layering a small motion engine
  (`w`/`b`/`ciw`/`dt,` …) on top. Ship a plain insert editor first; upgrade the
  motion set later.

### How it rides on the existing architecture

- **`store.WriteBackend`** (already defined, unimplemented) is where writes land —
  the same interface for local files now and CalDAV/CardDAV PUTs later.
- Models already retain **`Raw` bytes + `UID`/`Rev`/`Sequence`/`ETag`**. Refinement
  when implementing: edit by **mutating the re-parsed go-ical/go-vcard component
  and re-encoding**, rather than serializing our simplified `model` type — this
  preserves unknown properties we don't model (custom `X-` props, attachments)
  losslessly. Both libraries retain the full property tree, so this is clean.
- Saves should be **atomic** (write temp file + rename) and bump `SEQUENCE`/`REV`.
  That same conditional-write discipline becomes `If-Match` ETag PUTs for DAV with
  no rework.

### Suggested phasing (editing milestone)

1. Single-field edit + save (insert mode, atomic write, undo) — proves the write
   path end-to-end.
2. Add/delete fields and records (`o`, `dd`, create new contact/event).
3. Visual mode for bulk record operations (the yazi-parity win).
4. Richer in-field vim motions for multi-line values.
