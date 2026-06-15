# Yoro and vdirsyncer

**Yoro does not sync.** It is a *client* to each source you configure — a local vdir
tree or a remote CalDAV/CardDAV server — and it reads (and, later, writes) whichever
source you are looking at. It never copies data from one source to another.

Keeping a local vdir in step with a remote DAV server is a hard, edge-case-ridden job
(conflict resolution, deletion propagation, ETag handling, per-server quirks).
[vdirsyncer](https://vdirsyncer.pimutils.org/) already does this well, so Yoro
deliberately delegates to it rather than reimplementing it. This is an intentional scope
boundary, not a missing feature.

## When do you need vdirsyncer?

You **don't** need it just to use Yoro:

- Point Yoro straight at a `dav` source and browse iCloud/Fastmail/Nextcloud live.
- Or point Yoro at a local vdir tree and browse that.

You **do** want vdirsyncer when you want a **local mirror** of a remote account — for
offline access, faster startup, backups, or to feed other vdir tools (`khal`, `khard`,
`mutt`). vdirsyncer mirrors the server into `~/.local/share/calendars` /
`~/.local/share/contacts`; Yoro then browses that local copy like any other local source.

## Local mirror + the live server, side by side

Because Yoro shows both the local vdir and the DAV server as distinct sources, you can
configure **both** and watch them:

```toml
[[sources]]
name = "local"
type = "local"
calendars = "~/.local/share/calendars"
contacts  = "~/.local/share/contacts"

[[sources]]
name = "iCloud"
type = "dav"
url  = "https://caldav.icloud.com/"
username = "you@icloud.com"
password_command = "pass icloud/yoro"
```

- **Calendars** from both sources appear together, each tagged with its source glyph;
  toggle the duplicate off if you don't want it overlaid.
- **Contacts** show one source at a time — press `s` to switch between `local` and
  `iCloud`.

The two will **diverge** between vdirsyncer runs: a change made on the server (or, later,
through Yoro against the server) won't appear in the local mirror until `vdirsyncer sync`
runs, and vice-versa. That divergence is expected — the local source is a snapshot, the
DAV source is live. Run `vdirsyncer sync` to reconcile them.

## Example vdirsyncer pairing

A minimal `~/.config/vdirsyncer/config` mirroring iCloud contacts to the vdir layout
Yoro's local source reads:

```ini
[general]
status_path = "~/.local/share/vdirsyncer/status/"

[pair icloud_contacts]
a = "icloud_contacts_remote"
b = "icloud_contacts_local"
collections = ["from a"]

[storage icloud_contacts_remote]
type = "carddav"
url = "https://contacts.icloud.com/"
username = "you@icloud.com"
password.fetch = ["command", "pass", "icloud/yoro"]

[storage icloud_contacts_local]
type = "filesystem"
path = "~/.local/share/contacts/"
fileext = ".vcf"
```

Run `vdirsyncer discover && vdirsyncer sync` to populate the mirror, then browse it as
Yoro's `local` source. See the
[vdirsyncer tutorial](https://vdirsyncer.pimutils.org/en/stable/tutorial.html) for the
calendar (CalDAV) equivalent and for scheduling syncs.
