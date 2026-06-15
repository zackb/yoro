# Yoro

> A blazing-fast, terminal UI for your contacts and calendar.

Yoro is a TUI for browsing (and, soon, editing) calendars and contacts that live on
disk in the standard [vdirsyncer](https://vdirsyncer.pimutils.org/)/[khal](https://lostpackets.de/khal/)
layout — `~/.local/share/calendars` and `~/.local/share/contacts`. It reads and writes
plain iCalendar (`.ics`) and vCard (`.vcf`) files directly, and is designed to sync with
CalDAV/CardDAV servers with full read/write capability.

If you love [`yazi`](https://github.com/sxyazi/yazi), Yoro should feel like home:
miller-column navigation, a live preview pane that follows your cursor, nerd-font icons,
and vim keybindings throughout.

> **Status: early.** Milestone 1 is a polished, **read-only** browser for your local
> calendars and contacts. Writing files and CalDAV/CardDAV sync are planned and the
> architecture is built around those seams, but they are not implemented yet.

## Features

- **Local-first.** First-class support for the `vdirsyncer`/`khal` on-disk format —
  per-collection directories with `displayname`/`color` metadata.
- **Two modes, one feel.** A Calendar mode and a Contacts mode that share the same vim
  navigation and preview-follows-cursor behavior.
- **Calendar.** A day-grouped agenda with a mini-month navigator and per-collection color
  toggles. Recurring events (`RRULE`/`RDATE`/`EXDATE`) are expanded on the fly.
- **Contacts.** A three-column miller view (address books → contacts → detail) with live
  search.
- **Modern graphics.** Uses the kitty graphics protocol to render contact photos where the
  terminal (and embedded vCard `PHOTO` data) support it, degrading gracefully otherwise.
- **Single static binary.** Written in modern Go, `CGO_ENABLED=0`, no runtime dependencies
  (timezone data is embedded).

## Installation

### Arch Linux (AUR)

```sh
# release binary
yay -S yoro-bin
# or build from latest git
yay -S yoro-git
```

### From source

Requires Go 1.24+.

```sh
git clone https://github.com/zackb/yoro
cd yoro
make build
make install   # installs to /usr/local by default; override with PREFIX=
```

## Usage

```sh
yoro                 # open on the default (calendars + contacts) store
yoro --config PATH   # use an alternate config file
```

By default Yoro reads:

| Data      | Path                              |
| --------- | --------------------------------- |
| Calendars | `~/.local/share/calendars`        |
| Contacts  | `~/.local/share/contacts`         |

These are overridable via the config file or `$YORO_CONFIG`.

### Keybindings

Yoro uses vim motions, deviating only where a calendar has no filesystem analog.

| Key            | Action                                          |
| -------------- | ----------------------------------------------- |
| `Tab` `1` `2`  | Switch between Calendar and Contacts            |
| `h`            | Move focus to the column on the left            |
| `l` / `Enter`  | Move focus into the column on the right         |
| `j` / `k`      | Move down / up                                  |
| `gg` / `G`     | Jump to top / bottom                            |
| `ctrl+d` / `ctrl+u` | Half-page down / up                        |
| `/`            | Search within the current pane                  |
| `n` / `N`      | Next / previous search match                    |
| `R`            | Reload the store from disk                       |
| `?`            | Toggle help                                     |
| `q` / `ctrl+c` | Quit                                            |

**Calendar mode**

| Key       | Action                                       |
| --------- | -------------------------------------------- |
| `t`       | Jump to today                                |
| `}` / `{` | Next / previous day with events              |
| `J` / `K` | Next / previous month in the mini-month      |
| `space`   | Toggle the highlighted collection on/off     |
| `T`       | Toggle visibility of tasks (VTODO)           |

**Contacts mode**

| Key | Action                                              |
| --- | --------------------------------------------------- |
| `y` | Yank the highlighted email/phone to the clipboard   |

## Development

```sh
make build    # static binary into ./build
make test     # go test ./...
make lint     # gofmt + go vet
make run      # build and run
```

See [`man/yoro.1`](man/yoro.1) for the manual page.

## Roadmap

- [x] Read-only local browsing (Calendar + Contacts) — **Milestone 1**
- [ ] Editing `.ics`/`.vcf` files in place — vim-style modal editing with visual
  mode; see [DESIGN.md](DESIGN.md)
- [ ] CalDAV/CardDAV sync (read/write)
- [ ] Full month-grid calendar view (toggle)

## License

MIT — see [LICENSE](LICENSE).
