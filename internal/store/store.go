// Package store loads collections from a Backend, indexes them in memory, and
// exposes a read-only, recurrence-aware facade to the UI. The Backend boundary
// is the seam for a future CalDAV/CardDAV implementation; the UI depends only
// on Store, never on a concrete backend.
package store

import (
	"context"

	"github.com/zackb/yoro/internal/model"
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
	// Load scans and parses everything from the backend.
	Load(ctx context.Context) error
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
