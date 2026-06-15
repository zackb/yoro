# Yoro Design Notes

Forward-looking design decisions that are not yet implemented, plus a record of
the architectural decisions already made, so we can pick work up later without
re-deriving it.

## Sources: local vdir and CalDAV/CardDAV as co-equal citizens

Yoro browses any number of **sources** at once. A source is one backend: a local
vdir tree (`store.NewLocal`) or a remote DAV account (`store.../dav`). Both
implement the read-only `store.Backend`; `memStore` aggregates them, keying
everything by a **source-namespaced collection ID** so IDs stay unique across
sources. `model.Collection.Source` records which source a collection came from,
and `Store.Sources()` exposes source metadata to the UI for provenance glyphs.

Principles (deliberate scope boundaries):

- **Yoro is a pure client; it never syncs sources to each other.** Reconciling a
  local vdir with a DAV server is vdirsyncer's job (see `docs/vdirsyncer.md`).
  This avoids reimplementing vdirsyncer and the double-sync trap. KDE Akonadi and
  GNOME Evolution Data Server are themselves *peer clients* of DAV, not storage
  backends, so they are out of scope — wiring Yoro to both them and the DAV they
  mirror would create update loops.
- **Provenance is always visible.** When more than one source is configured, the
  UI marks each collection with its source, so when writes land it is unambiguous
  whether a mutation targets a remote DAV collection or a local file.

Per-domain presentation (these differ on purpose):

- **Calendars** are an overlay: all sources' calendars are listed together,
  tagged by source, and merged in the agenda with the existing per-collection
  toggles. If a local vdir mirrors a DAV calendar, just don't toggle the
  duplicate on.
- **Contacts** are a single list: one *active* source at a time (switch with
  `s`), because a contact list is a lookup and showing every person twice when a
  vdir mirrors a DAV account would be noise.

### Loading

Item loading is eager for every source today (same as the original local-only
flow), tolerating per-source/per-collection failures so one unreachable DAV
server can't take down browsing of the others. The `Store.Reload(colID)` seam
(routed to the owning backend) is where lazy/per-collection refresh or fsnotify
would later hang.

### DAV backend specifics

The DAV backend re-encodes the parsed go-ical/go-vcard objects returned by
`go-webdav` back to bytes and feeds them through the existing `internal/ical`
and `internal/vcard` decoders, so there is a single parse path. `ETag` is
captured for the future `If-Match` write seam; calendar color is not exposed by
the CalDAV client structs, so DAV calendars currently have no color.

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
