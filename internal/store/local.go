package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zackb/yoro/internal/ical"
	"github.com/zackb/yoro/internal/model"
	"github.com/zackb/yoro/internal/vcard"
)

// Local is a read-only Backend over the vdirsyncer/khal on-disk layout. A
// collection is any directory that directly contains .ics (calendar) or .vcf
// (address book) files. Per-collection "displayname" and "color" sibling files
// are honored. Collection IDs are namespaced with the source id so they stay
// unique when several sources are browsed together.
type Local struct {
	sourceID     string
	calendarsDir string
	contactsDir  string
}

// NewLocal constructs a filesystem backend for the given source id over the
// calendar and contact roots.
func NewLocal(sourceID, calendarsDir, contactsDir string) *Local {
	return &Local{sourceID: sourceID, calendarsDir: calendarsDir, contactsDir: contactsDir}
}

// Local is a read/write backend.
var _ WriteBackend = (*Local)(nil)

func (l *Local) Collections(ctx context.Context) ([]model.Collection, error) {
	var cols []model.Collection
	cals, err := discover(l.calendarsDir, ".ics", model.KindCalendar, l.sourceID)
	if err != nil {
		return nil, err
	}
	cols = append(cols, cals...)
	books, err := discover(l.contactsDir, ".vcf", model.KindAddressBook, l.sourceID)
	if err != nil {
		return nil, err
	}
	cols = append(cols, books...)
	return cols, nil
}

func (l *Local) Events(ctx context.Context, colID string) ([]model.Event, error) {
	dir := l.dirFor(colID, model.KindCalendar)
	var events []model.Event
	err := eachFile(dir, ".ics", func(path string, data []byte) error {
		f, err := ical.Parse(data, colID)
		if err != nil {
			return nil // skip malformed files
		}
		for i := range f.Events {
			f.Events[i].Path = path
		}
		events = append(events, f.Events...)
		return nil
	})
	return events, err
}

func (l *Local) Todos(ctx context.Context, colID string) ([]model.Todo, error) {
	dir := l.dirFor(colID, model.KindCalendar)
	var todos []model.Todo
	err := eachFile(dir, ".ics", func(path string, data []byte) error {
		f, err := ical.Parse(data, colID)
		if err != nil {
			return nil
		}
		todos = append(todos, f.Todos...)
		return nil
	})
	return todos, err
}

func (l *Local) Contacts(ctx context.Context, colID string) ([]model.Contact, error) {
	dir := l.dirFor(colID, model.KindAddressBook)
	var contacts []model.Contact
	err := eachFile(dir, ".vcf", func(path string, data []byte) error {
		cs, err := vcard.Parse(data, colID)
		if err != nil {
			return nil
		}
		for i := range cs {
			cs[i].Path = path
		}
		contacts = append(contacts, cs...)
		return nil
	})
	return contacts, err
}

// PutEvent writes a new or replacement .ics file for the event, named by UID.
func (l *Local) PutEvent(ctx context.Context, colID string, e model.Event) error {
	data, err := ical.Marshal(ical.BuildEvent(e))
	if err != nil {
		return err
	}
	return writeAtomic(l.dirFor(colID, model.KindCalendar), e.UID+".ics", data)
}

// PutContact writes a new or replacement .vcf file for the contact, named by UID.
func (l *Local) PutContact(ctx context.Context, colID string, c model.Contact) error {
	data, err := vcard.Marshal(vcard.BuildContact(c))
	if err != nil {
		return err
	}
	return writeAtomic(l.dirFor(colID, model.KindAddressBook), c.UID+".vcf", data)
}

// UpdateEvent rewrites the event's original file in place, mutating only the
// matching VEVENT so unmodeled properties and sibling components survive.
func (l *Local) UpdateEvent(ctx context.Context, colID string, e model.Event) error {
	cal, err := ical.UpdateEvent(e.Raw, e)
	if err != nil {
		return err
	}
	data, err := ical.Marshal(cal)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Dir(e.Path), filepath.Base(e.Path), data)
}

// UpdateContact rewrites the contact's original file in place.
func (l *Local) UpdateContact(ctx context.Context, colID string, c model.Contact) error {
	card, err := vcard.UpdateContact(c.Raw, c)
	if err != nil {
		return err
	}
	data, err := vcard.Marshal(card)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Dir(c.Path), filepath.Base(c.Path), data)
}

// DeleteEvent and DeleteContact are not yet implemented (create-only milestone).
func (l *Local) DeleteEvent(ctx context.Context, colID, uid string) error {
	return errNotImplemented
}

func (l *Local) DeleteContact(ctx context.Context, colID, uid string) error {
	return errNotImplemented
}

var errNotImplemented = errors.New("local: delete not implemented")

// writeAtomic writes data to name within dir via a temp file + rename, so a
// reader never observes a half-written file. The collection dir is created if
// missing.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".yoro-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// dirFor resolves a source-namespaced collection ID back to an absolute
// directory by stripping the source+domain prefix and joining onto the right root.
func (l *Local) dirFor(colID string, kind model.Kind) string {
	root := l.calendarsDir
	if kind == model.KindAddressBook {
		root = l.contactsDir
	}
	rel := strings.TrimPrefix(model.NativeID(l.sourceID, colID), domainDir(kind)+"/")
	return filepath.Join(root, rel)
}

// domainDir is the path segment that distinguishes calendar collections from
// address books in a collection ID, so a calendar and an address book that share
// a directory basename (e.g. both "icloud") don't collapse to the same ID.
func domainDir(kind model.Kind) string {
	if kind == model.KindAddressBook {
		return "contacts"
	}
	return "calendars"
}

// discover walks root and returns every directory that directly contains a file
// with the given extension, namespacing each collection ID with sourceID.
func discover(root, ext string, kind model.Kind, sourceID string) ([]model.Collection, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, nil // missing root is not fatal; just no collections
	}
	seen := map[string]bool{}
	var cols []model.Collection
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ext) {
			return nil
		}
		dir := filepath.Dir(path)
		if seen[dir] {
			return nil
		}
		seen[dir] = true
		rel, _ := filepath.Rel(root, dir)
		cols = append(cols, model.Collection{
			ID:     model.NamespaceID(sourceID, domainDir(kind)+"/"+rel),
			Source: sourceID,
			Name:   collectionName(dir),
			Color:  model.ParseColor(readMeta(dir, "color")),
			Kind:   kind,
			Path:   dir,
		})
		return nil
	})
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	return cols, err
}

// collectionName prefers the displayname sibling file, then a meaningful
// directory basename (treating generic "card" as the account name).
func collectionName(dir string) string {
	if name := readMeta(dir, "displayname"); name != "" {
		return name
	}
	base := filepath.Base(dir)
	if base == "card" {
		return filepath.Base(filepath.Dir(dir))
	}
	return base
}

// readMeta reads a one-line metadata sibling file, trimming whitespace.
func readMeta(dir, name string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// eachFile invokes fn with the absolute path and bytes of every file in dir with
// the given ext. The path lets callers stamp a write-back locator on each item.
func eachFile(dir, ext string, fn func(path string, data []byte) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // empty/missing collection
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ext) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := fn(path, data); err != nil {
			return err
		}
	}
	return nil
}
