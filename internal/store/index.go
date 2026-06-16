package store

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/zackb/yoro/internal/ical"
	"github.com/zackb/yoro/internal/model"
)

// calendarLoader is an optional Backend optimization: fetch a calendar's events
// and todos in a single pass, sparing backends a redundant second fetch/parse.
type calendarLoader interface {
	CalendarItems(ctx context.Context, colID string) ([]model.Event, []model.Todo, error)
}

// calendarItems loads a calendar's events and todos, using the backend's
// single-pass loader when available and falling back to two calls otherwise.
func calendarItems(ctx context.Context, b Backend, colID string) ([]model.Event, []model.Todo, error) {
	if cl, ok := b.(calendarLoader); ok {
		return cl.CalendarItems(ctx, colID)
	}
	ev, err := b.Events(ctx, colID)
	if err != nil {
		return nil, nil, err
	}
	td, err := b.Todos(ctx, colID)
	if err != nil {
		return nil, nil, err
	}
	return ev, td, nil
}

// memStore is the default in-memory, recurrence-aware Store. It aggregates any
// number of sources, keying everything by source-namespaced collection ID.
type memStore struct {
	sources  []SourceInfo
	backends map[string]Backend // by source id

	mu          sync.RWMutex
	collections []model.Collection
	colByID     map[string]model.Collection
	events      map[string][]model.Event
	todos       map[string][]model.Todo
	contacts    map[string][]model.Contact
}

// New builds a Store over the given sources, browsed together.
func New(sources ...Source) Store {
	s := &memStore{
		backends: map[string]Backend{},
		colByID:  map[string]model.Collection{},
		events:   map[string][]model.Event{},
		todos:    map[string][]model.Todo{},
		contacts: map[string][]model.Contact{},
	}
	for _, src := range sources {
		s.sources = append(s.sources, src.SourceInfo)
		s.backends[src.ID] = src.Backend
	}
	return s
}

func (s *memStore) Sources() []SourceInfo {
	return append([]SourceInfo(nil), s.sources...)
}

func (s *memStore) Load(ctx context.Context) error {
	// Enumerate collections across all sources. A source that fails to
	// enumerate (e.g. a DAV server is unreachable) is skipped so it can't take
	// down browsing of the others.
	var cols []model.Collection
	for _, si := range s.sources {
		cs, err := s.backends[si.ID].Collections(ctx)
		if err != nil {
			continue
		}
		cols = append(cols, cs...)
	}

	// Parse every collection concurrently, bounded by CPU count. Item-load
	// failures are tolerated per collection rather than failing the whole load.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(runtime.NumCPU())

	events := map[string][]model.Event{}
	todos := map[string][]model.Todo{}
	contacts := map[string][]model.Contact{}
	var mu sync.Mutex

	for _, c := range cols {
		c := c
		b := s.backends[c.Source]
		if b == nil {
			continue
		}
		g.Go(func() error {
			switch c.Kind {
			case model.KindCalendar:
				ev, td, err := calendarItems(gctx, b, c.ID)
				if err != nil {
					return nil
				}
				mu.Lock()
				events[c.ID] = ev
				todos[c.ID] = td
				mu.Unlock()
			case model.KindAddressBook:
				ct, err := b.Contacts(gctx, c.ID)
				if err != nil {
					return nil
				}
				sortContacts(ct)
				mu.Lock()
				contacts[c.ID] = ct
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()

	byID := make(map[string]model.Collection, len(cols))
	for _, c := range cols {
		byID[c.ID] = c
	}

	s.mu.Lock()
	s.collections = cols
	s.colByID = byID
	s.events = events
	s.todos = todos
	s.contacts = contacts
	s.mu.Unlock()
	return nil
}

func (s *memStore) Reload(ctx context.Context, colID string) error {
	s.mu.RLock()
	col, ok := s.colByID[colID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	b := s.backends[col.Source]
	if b == nil {
		return nil
	}
	switch col.Kind {
	case model.KindCalendar:
		ev, td, err := calendarItems(ctx, b, colID)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.events[colID] = ev
		s.todos[colID] = td
		s.mu.Unlock()
	case model.KindAddressBook:
		ct, err := b.Contacts(ctx, colID)
		if err != nil {
			return err
		}
		sortContacts(ct)
		s.mu.Lock()
		s.contacts[colID] = ct
		s.mu.Unlock()
	}
	return nil
}

func (s *memStore) CreateEvent(ctx context.Context, colID string, e model.Event) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if e.UID == "" {
		e.UID = uuid.NewString()
	}
	e.CollectionID = colID
	if err := wb.PutEvent(ctx, colID, e); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

func (s *memStore) CreateContact(ctx context.Context, colID string, c model.Contact) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if c.UID == "" {
		c.UID = uuid.NewString()
	}
	c.CollectionID = colID
	if err := wb.PutContact(ctx, colID, c); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

func (s *memStore) UpdateEvent(ctx context.Context, colID string, e model.Event) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if e.UID == "" || e.Path == "" {
		return fmt.Errorf("store: cannot update event without UID and Path")
	}
	if err := wb.UpdateEvent(ctx, colID, e); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

func (s *memStore) UpdateContact(ctx context.Context, colID string, c model.Contact) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if c.UID == "" || c.Path == "" {
		return fmt.Errorf("store: cannot update contact without UID and Path")
	}
	if err := wb.UpdateContact(ctx, colID, c); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

func (s *memStore) DeleteEvent(ctx context.Context, colID, path string) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("store: cannot delete event without Path")
	}
	if err := wb.DeleteEvent(ctx, colID, path); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

func (s *memStore) DeleteContact(ctx context.Context, colID, path string) error {
	wb, err := s.writeBackendFor(colID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("store: cannot delete contact without Path")
	}
	if err := wb.DeleteContact(ctx, colID, path); err != nil {
		return err
	}
	return s.Reload(ctx, colID)
}

// writeBackendFor returns the writable backend owning colID, or ErrReadOnly if
// the collection is unknown or its source can't be written.
func (s *memStore) writeBackendFor(colID string) (WriteBackend, error) {
	s.mu.RLock()
	col, ok := s.colByID[colID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrReadOnly
	}
	wb, ok := s.backends[col.Source].(WriteBackend)
	if !ok {
		return nil, ErrReadOnly
	}
	return wb, nil
}

func (s *memStore) Collections() []model.Collection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Collection(nil), s.collections...)
}

func (s *memStore) Calendars() []model.Collection    { return s.filterKind(model.KindCalendar) }
func (s *memStore) AddressBooks() []model.Collection { return s.filterKind(model.KindAddressBook) }

func (s *memStore) filterKind(k model.Kind) []model.Collection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []model.Collection
	for _, c := range s.collections {
		if c.Kind == k {
			out = append(out, c)
		}
	}
	return out
}

func (s *memStore) Occurrences(window model.DateRange, enabled map[string]bool) []model.Occurrence {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []model.Occurrence
	for colID, evs := range s.events {
		if enabled != nil && !enabled[colID] {
			continue
		}
		color := s.colByID[colID].Color
		for i := range evs {
			occs := ical.Expand(&evs[i], window.From, window.To)
			for j := range occs {
				occs[j].Color = color
				out = append(out, occs[j])
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].Summary < out[j].Summary
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func (s *memStore) Contacts(colID string) []model.Contact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Contact(nil), s.contacts[colID]...)
}

func (s *memStore) Todos(colID string) []model.Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Todo(nil), s.todos[colID]...)
}

func (s *memStore) Search(domain Domain, query string) []Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch domain {
	case DomainContacts:
		return searchItems(s.collections, model.KindAddressBook, s.contacts, domain,
			func(c model.Contact) (string, bool) { return c.FN, contactMatches(c, q) })
	case DomainCalendar:
		return searchItems(s.collections, model.KindCalendar, s.events, domain,
			func(e model.Event) (string, bool) { return e.Summary, strings.Contains(strings.ToLower(e.Summary), q) })
	}
	return nil
}

// searchItems collects matches across the collections of one kind, labeling each
// hit and recording its index within that collection's ordered slice.
func searchItems[T any](cols []model.Collection, kind model.Kind, byCol map[string][]T, domain Domain, match func(T) (string, bool)) []Match {
	var matches []Match
	for _, col := range cols {
		if col.Kind != kind {
			continue
		}
		for i, it := range byCol[col.ID] {
			if label, ok := match(it); ok {
				matches = append(matches, Match{Domain: domain, Collection: col.ID, Label: label, Index: i})
			}
		}
	}
	return matches
}

func contactMatches(c model.Contact, q string) bool {
	if strings.Contains(strings.ToLower(c.FN), q) {
		return true
	}
	for _, e := range c.Emails {
		if strings.Contains(strings.ToLower(e.Value), q) {
			return true
		}
	}
	for _, p := range c.Phones {
		if strings.Contains(strings.ToLower(p.Value), q) {
			return true
		}
	}
	return false
}

func sortContacts(cs []model.Contact) {
	sort.SliceStable(cs, func(i, j int) bool {
		return strings.ToLower(sortKey(cs[i])) < strings.ToLower(sortKey(cs[j]))
	})
}

// sortKey orders named contacts alphabetically by family then given name,
// pushing nameless (org/email-only) entries to the end.
func sortKey(c model.Contact) string {
	if c.Name.Family != "" {
		return "0" + c.Name.Family + " " + c.Name.Given
	}
	if c.HasName() {
		return "0" + c.FN
	}
	return "1" + c.DisplayName() // nameless entries sort last
}
