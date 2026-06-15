// Package store loads collections from a Backend, indexes them in memory, and
// exposes a read-only, recurrence-aware facade to the UI. The Backend boundary
// is the seam for a future CalDAV/CardDAV implementation; the UI depends only
// on Store, never on a concrete backend.
package store

import (
	"context"

	"github.com/zackb/yoro/internal/model"
	"github.com/zackb/yoro/internal/store/dav"
)

// Backend reads raw collections and their items. The local filesystem backend
// implements this today; a DAV backend will later implement WriteBackend.
type Backend interface {
	Collections(ctx context.Context) ([]model.Collection, error)
	Events(ctx context.Context, colID string) ([]model.Event, error)
	Todos(ctx context.Context, colID string) ([]model.Todo, error)
	Contacts(ctx context.Context, colID string) ([]model.Contact, error)
}

// WriteBackend adds mutation. Defined now to fix the seam; implemented by a
// future local-write or DAV backend, not in milestone 1.
type WriteBackend interface {
	Backend
	PutEvent(ctx context.Context, colID string, e model.Event) error
	DeleteEvent(ctx context.Context, colID, uid string) error
	PutContact(ctx context.Context, colID string, c model.Contact) error
	DeleteContact(ctx context.Context, colID, uid string) error
}

// Source kinds, mirroring config source types, used for provenance display.
const (
	TypeLocal = "local"
	TypeDAV   = "dav"
)

// SourceInfo identifies a configured source for provenance display in the UI.
type SourceInfo struct {
	ID   string
	Name string
	Type string // TypeLocal | TypeDAV
}

// Source pairs a backend with its identity. The store browses any number of
// sources at once; each collection records the source it came from.
type Source struct {
	SourceInfo
	Backend Backend
}

// LocalSource builds a Source backed by a local filesystem backend.
func LocalSource(id, name, calendarsDir, contactsDir string) Source {
	return Source{
		SourceInfo: SourceInfo{ID: id, Name: name, Type: TypeLocal},
		Backend:    NewLocal(id, calendarsDir, contactsDir),
	}
}

// DAVSource builds a Source backed by a read-only CalDAV/CardDAV backend,
// connecting and discovering collections eagerly so errors surface at startup.
func DAVSource(ctx context.Context, id, name, endpoint, username, password string) (Source, error) {
	b, err := dav.New(ctx, id, endpoint, username, password)
	if err != nil {
		return Source{}, err
	}
	return Source{
		SourceInfo: SourceInfo{ID: id, Name: name, Type: TypeDAV},
		Backend:    b,
	}, nil
}

// Compile-time assurance that the DAV backend satisfies Backend (read-only).
var _ Backend = (*dav.DAV)(nil)

// Domain selects what a search ranges over.
type Domain int

const (
	DomainCalendar Domain = iota
	DomainContacts
)

// Match is a single search hit.
type Match struct {
	Domain     Domain
	Collection string
	Label      string
	Index      int // position within the relevant ordered slice
}

// Store is the UI-facing facade: it caches parsed data, maintains indexes, and
// expands recurrence within a window.
type Store interface {
	// Load scans and parses everything from every source.
	Load(ctx context.Context) error
	// Sources returns the configured sources, in order, for provenance display.
	Sources() []SourceInfo
	// Collections returns all collections (calendars and address books).
	Collections() []model.Collection
	// Calendars returns just calendar collections.
	Calendars() []model.Collection
	// AddressBooks returns just address-book collections.
	AddressBooks() []model.Collection
	// Occurrences returns recurrence-expanded instances starting within window,
	// limited to enabled collections (nil enabled means "all"), sorted by start.
	Occurrences(window model.DateRange, enabled map[string]bool) []model.Occurrence
	// Contacts returns the contacts of one address book, sorted by display name.
	Contacts(colID string) []model.Contact
	// Todos returns the todos of one calendar.
	Todos(colID string) []model.Todo
	// Search ranges over the given domain.
	Search(domain Domain, query string) []Match
	// Reload re-scans a single collection (seam for future fsnotify/sync).
	Reload(ctx context.Context, colID string) error
}
