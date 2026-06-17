# Connecting your accounts

Yoro browses one or more **sources**, listed in `$XDG_CONFIG_HOME/yoro/config.toml`
(usually `~/.config/yoro/config.toml`). A source is either a **local vdir tree** on
disk or a **remote CalDAV/CardDAV account**. You can mix as many as you like; they are
browsed together, each collection tagged with the source it came from.

This page is a cookbook of working `[[sources]]` blocks for the common providers. For
the general configuration reference, see the [README](../README.md#configuration).

## How sources work

Each `[[sources]]` block is one source:

```toml
[[sources]]
name = "local"          # unique label — this is the source's identity
type = "local"          # "local" or "dav"
calendars = "~/.local/share/calendars"
contacts  = "~/.local/share/contacts"
```

A DAV source instead points at a server:

```toml
[[sources]]
name = "example"
type = "dav"
url  = "https://dav.example.com/"
username = "you@example.com"
password = "..."                 # an inline secret (discouraged — see below)
# password_command = "pass yoro/example"   # ...or resolve it from a command
```

A few rules that apply to every account below:

- **Names must be unique.** The `name` is how a source is identified throughout the UI.
- **Keep secrets out of the file.** Prefer `password_command` over `password`: it runs
  via the shell and its first line is used as the password, so the plaintext lives in
  your password manager instead of the config. Examples: `pass yoro/icloud`,
  `secret-tool lookup service yoro account icloud`, `op read "op://Private/iCloud/password"`.
- **App passwords, not your login password.** Google, iCloud, and Fastmail all require a
  per-app password (and 2-factor enabled on the account); your normal password will be
  rejected. Each section below says where to generate one.
- **Some providers split calendars and contacts across two hostnames.** Yoro probes both
  protocols at the `url` you give, but it can't reach a host you didn't list — so for
  iCloud, Google, and Fastmail you add **two** `dav` sources, one per host.
- **Calendars** from every source overlay in the agenda; **contacts** show one source at
  a time (press `s` to switch).

---

## Local vdir tree

The default if you configure nothing. Plain `.ics`/`.vcf` files in the
[vdirsyncer](https://vdirsyncer.pimutils.org/)/[khal](https://lostpackets.de/khal/)
layout (one directory per collection). Use this to browse data that
[vdirsyncer](vdirsyncer.md) has mirrored locally, or any khal/khard setup.

```toml
[[sources]]
name = "local"
type = "local"
calendars = "~/.local/share/calendars"
contacts  = "~/.local/share/contacts"
```

## Nextcloud (and ownCloud)

Nextcloud serves CalDAV and CardDAV from a **single** base URL, so one `dav` source
covers both. Replace `cloud.example.com` with your instance's host.

```toml
[[sources]]
name = "nextcloud"
type = "dav"
url  = "https://cloud.example.com/remote.php/dav/"
username = "you"
password_command = "pass yoro/nextcloud"
```

If your account has 2-factor enabled, generate a device password under
**Settings → Security → Devices & sessions** and use that instead of your login password.

## Google

Google puts CalDAV and CardDAV on **different hosts**, so an account needs **two**
sources. Both authenticate with a [Google **app password**](https://myaccount.google.com/apppasswords)
(2-Step Verification must be on); your account password will not work.

```toml
[[sources]]
name = "google-calendar"
type = "dav"
url  = "https://www.google.com/calendar/dav/"
username = "you@gmail.com"
password_command = "pass yoro/google"

[[sources]]
name = "google-contacts"
type = "dav"
url  = "https://www.googleapis.com/carddav/v1/principals/you@gmail.com/lists/default/"
username = "you@gmail.com"
password_command = "pass yoro/google"
```

> Use your full `@gmail.com` (or Workspace) address as `username`, and substitute it into
> the contacts URL as well.

## iCloud

iCloud also splits calendars and contacts across two hosts, needing **two** sources.
Authenticate with an [app-specific password](https://support.apple.com/en-us/102654)
generated at [appleid.apple.com](https://appleid.apple.com/) — not your Apple ID password.

```toml
[[sources]]
name = "icloud-calendar"
type = "dav"
url  = "https://caldav.icloud.com/"
username = "you@icloud.com"
password_command = "pass yoro/icloud"

[[sources]]
name = "icloud-contacts"
type = "dav"
url  = "https://contacts.icloud.com/"
username = "you@icloud.com"
password_command = "pass yoro/icloud"
```

> `username` is your full Apple ID email.

## Fastmail

Fastmail likewise serves CalDAV and CardDAV from separate hosts → **two** sources. Create
an **app password** under **Settings → Privacy & Security → Connected apps & API tokens →
Manage app passwords** (the default *Mail, Contacts & Calendars* scope is fine); your
normal or 2-step password will be rejected.

```toml
[[sources]]
name = "fastmail-calendar"
type = "dav"
url  = "https://caldav.fastmail.com/"
username = "you@fastmail.com"
password_command = "pass yoro/fastmail"

[[sources]]
name = "fastmail-contacts"
type = "dav"
url  = "https://carddav.fastmail.com/"
username = "you@fastmail.com"
password_command = "pass yoro/fastmail"
```

> `username` is your full Fastmail address, including the domain.

---

## Want an offline copy?

Pointing a `dav` source at a server browses it **live** — no sync needed. If you instead
want a local mirror (for offline use, faster startup, backups, or to share data with
`khal`/`khard`/`mutt`), run [vdirsyncer](vdirsyncer.md) to mirror the account into
`~/.local/share/calendars` / `~/.local/share/contacts`, then add it as a `local` source.
Yoro never syncs sources to each other — that's vdirsyncer's job, by design.
