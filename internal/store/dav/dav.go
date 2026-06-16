// Package dav implements a read-only store.Backend over CalDAV and CardDAV
// using emersion/go-webdav. Calendar and address objects come back as the same
// go-ical / go-vcard types Yoro already parses, so the existing decoders are
// reused: an object is re-encoded to its on-the-wire bytes and fed through
// internal/ical or internal/vcard, preserving ETags for a future write seam.
package dav

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav/carddav"

	"github.com/zackb/yoro/internal/ical"
	"github.com/zackb/yoro/internal/model"
	"github.com/zackb/yoro/internal/vcard"
)

// DAV is a read-only CalDAV/CardDAV backend. A single endpoint may expose
// calendars, address books, or both; whichever discovery succeeds is used.
type DAV struct {
	sourceID string

	// hc and base back the raw PROPFIND fallback used to enumerate address
	// objects on servers (notably Google) that reject the addressbook-query
	// REPORT. base is the endpoint's scheme://host; PROPFIND targets are the
	// absolute collection/object paths returned by discovery.
	hc   webdav.HTTPClient
	base string

	cal      *caldav.Client
	calHome  string
	card     *carddav.Client
	cardHome string

	// calCache holds a calendar's freshly fetched+parsed objects so the store's
	// back-to-back Events()+Todos() calls cost one CalDAV REPORT, not two.
	// Events() populates it; Todos() consumes it. Entries are one-shot so stale
	// data is never served if Todos() is called without a preceding Events().
	mu       sync.Mutex
	calCache map[string]ical.File
}

// New connects to endpoint with basic auth and discovers the calendar and
// address-book home sets. Discovery failures for one protocol are non-fatal:
// the backend simply won't report that kind of collection. An error is returned
// only if neither CalDAV nor CardDAV could be reached.
func New(ctx context.Context, sourceID, endpoint, username, password string) (*DAV, error) {
	hc := webdav.HTTPClientWithBasicAuth(http.DefaultClient, username, password)
	d := &DAV{sourceID: sourceID, hc: hc, calCache: map[string]ical.File{}}
	if u, err := url.Parse(endpoint); err == nil {
		d.base = u.Scheme + "://" + u.Host
	}

	if c, err := caldav.NewClient(hc, endpoint); err == nil {
		if principal, err := c.FindCurrentUserPrincipal(ctx); err == nil {
			if home, err := c.FindCalendarHomeSet(ctx, principal); err == nil {
				d.cal, d.calHome = c, home
			}
		}
	}
	if c, err := carddav.NewClient(hc, endpoint); err == nil {
		if principal, err := c.FindCurrentUserPrincipal(ctx); err == nil {
			if home, err := c.FindAddressBookHomeSet(ctx, principal); err == nil {
				d.card, d.cardHome = c, home
			}
		}
	}

	if d.cal == nil && d.card == nil {
		return nil, fmt.Errorf("dav: no CalDAV or CardDAV collections discovered at %s", endpoint)
	}
	return d, nil
}

func (d *DAV) Collections(ctx context.Context) ([]model.Collection, error) {
	var cols []model.Collection
	if d.cal != nil {
		cals, err := d.cal.FindCalendars(ctx, d.calHome)
		if err == nil {
			for _, c := range cals {
				cols = append(cols, model.Collection{
					ID:     model.NamespaceID(d.sourceID, c.Path),
					Source: d.sourceID,
					Name:   c.Name,
					Kind:   model.KindCalendar,
				})
			}
		}
	}
	if d.card != nil {
		books, err := d.card.FindAddressBooks(ctx, d.cardHome)
		if err == nil {
			for _, b := range books {
				cols = append(cols, model.Collection{
					ID:     model.NamespaceID(d.sourceID, b.Path),
					Source: d.sourceID,
					Name:   b.Name,
					Kind:   model.KindAddressBook,
				})
			}
		}
	}
	return cols, nil
}

func (d *DAV) Events(ctx context.Context, colID string) ([]model.Event, error) {
	f, err := d.fetchCalendar(ctx, colID)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.calCache[colID] = f
	d.mu.Unlock()
	return f.Events, nil
}

func (d *DAV) Todos(ctx context.Context, colID string) ([]model.Todo, error) {
	d.mu.Lock()
	f, ok := d.calCache[colID]
	delete(d.calCache, colID)
	d.mu.Unlock()
	if !ok {
		var err error
		if f, err = d.fetchCalendar(ctx, colID); err != nil {
			return nil, err
		}
	}
	return f.Todos, nil
}

// fetchCalendar issues one CalDAV REPORT for the collection and decodes every
// object into a single File (events and todos together) via the shared parser.
func (d *DAV) fetchCalendar(ctx context.Context, colID string) (ical.File, error) {
	if d.cal == nil {
		return ical.File{}, nil
	}
	objs, err := d.cal.QueryCalendar(ctx, model.NativeID(d.sourceID, colID), &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{Name: "VCALENDAR", AllProps: true, AllComps: true},
		CompFilter:  caldav.CompFilter{Name: "VCALENDAR"},
	})
	if err != nil {
		return ical.File{}, err
	}
	var out ical.File
	for _, o := range objs {
		if o.Data == nil {
			continue
		}
		data, err := ical.Marshal(o.Data)
		if err != nil {
			continue
		}
		f, err := ical.Parse(data, colID)
		if err != nil {
			continue
		}
		for i := range f.Events {
			f.Events[i].ETag = o.ETag
			f.Events[i].Path = o.Path
		}
		out.Events = append(out.Events, f.Events...)
		out.Todos = append(out.Todos, f.Todos...)
	}
	return out, nil
}

func (d *DAV) Contacts(ctx context.Context, colID string) ([]model.Contact, error) {
	if d.card == nil {
		return nil, nil
	}
	book := model.NativeID(d.sourceID, colID)
	objs, err := d.card.QueryAddressBook(ctx, book, &carddav.AddressBookQuery{
		DataRequest: carddav.AddressDataRequest{AllProp: true},
	})
	if err != nil {
		// Google (and some others) reject the addressbook-query REPORT with 400.
		// Fall back to enumerating object hrefs with a plain PROPFIND, then
		// fetching them via addressbook-multiget, which those servers accept.
		objs, err = d.multiGetAll(ctx, book)
	}
	if err != nil {
		return nil, err
	}
	var out []model.Contact
	for _, o := range objs {
		data, err := vcard.Marshal(o.Card)
		if err != nil {
			continue
		}
		cs, err := vcard.Parse(data, colID)
		if err != nil {
			continue
		}
		for i := range cs {
			cs[i].ETag = o.ETag
			cs[i].Path = o.Path
		}
		out = append(out, cs...)
	}
	return out, nil
}

// multiGetAll enumerates an address book's object hrefs with a Depth:1 PROPFIND
// (requesting only getetag, which servers reliably return) and fetches them with
// addressbook-multiget. The multiget uses a bare address-data request: go-webdav's
// AllProp emits an <allprop/> child that Google rejects, while an empty
// address-data element already means "all properties".
func (d *DAV) multiGetAll(ctx context.Context, book string) ([]carddav.AddressObject, error) {
	hrefs, err := d.enumerate(ctx, book)
	if err != nil {
		return nil, err
	}
	if len(hrefs) == 0 {
		return nil, nil
	}
	return d.card.MultiGetAddressBook(ctx, book, &carddav.AddressBookMultiGet{
		Paths:       hrefs,
		DataRequest: carddav.AddressDataRequest{},
	})
}

// enumerate issues a Depth:1 PROPFIND for getetag against the collection and
// returns the hrefs of its member objects (those carrying an etag; the
// collection itself, which does not, is skipped).
func (d *DAV) enumerate(ctx context.Context, collection string) ([]string, error) {
	const body = `<?xml version="1.0" encoding="utf-8"?>` +
		`<d:propfind xmlns:d="DAV:"><d:prop><d:getetag/></d:prop></d:propfind>`
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", d.base+collection, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("dav: enumerate %s: %s", collection, resp.Status)
	}
	var ms struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat []struct {
				Status string `xml:"status"`
				ETag   string `xml:"prop>getetag"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}
	var hrefs []string
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if strings.Contains(ps.Status, " 200 ") && ps.ETag != "" {
				hrefs = append(hrefs, r.Href)
			}
		}
	}
	return hrefs, nil
}

// PutEvent creates or replaces a calendar object. For create the UID is fresh,
// so the object path (<collection>/<UID>.ics) is new; go-webdav issues a plain
// PUT (no If-None-Match yet). The returned ETag is recorded on the model.
func (d *DAV) PutEvent(ctx context.Context, colID string, e model.Event) error {
	if d.cal == nil {
		return fmt.Errorf("dav: source %q has no calendars", d.sourceID)
	}
	path := objectPath(model.NativeID(d.sourceID, colID), e.UID, ".ics")
	_, err := d.cal.PutCalendarObject(ctx, path, ical.BuildEvent(e))
	return err
}

// PutContact creates or replaces an address object.
func (d *DAV) PutContact(ctx context.Context, colID string, c model.Contact) error {
	if d.card == nil {
		return fmt.Errorf("dav: source %q has no address books", d.sourceID)
	}
	path := objectPath(model.NativeID(d.sourceID, colID), c.UID, ".vcf")
	_, err := d.card.PutAddressObject(ctx, path, vcard.BuildContact(c))
	return err
}

// UpdateEvent mutates the event's original object in place and PUTs it back to
// its existing href (e.Path), preserving unmodeled properties.
func (d *DAV) UpdateEvent(ctx context.Context, colID string, e model.Event) error {
	if d.cal == nil {
		return fmt.Errorf("dav: source %q has no calendars", d.sourceID)
	}
	cal, err := ical.UpdateEvent(e.Raw, e)
	if err != nil {
		return err
	}
	_, err = d.cal.PutCalendarObject(ctx, e.Path, cal)
	return err
}

// UpdateContact mutates the contact's original object in place and PUTs it back.
func (d *DAV) UpdateContact(ctx context.Context, colID string, c model.Contact) error {
	if d.card == nil {
		return fmt.Errorf("dav: source %q has no address books", d.sourceID)
	}
	card, err := vcard.UpdateContact(c.Raw, c)
	if err != nil {
		return err
	}
	_, err = d.card.PutAddressObject(ctx, c.Path, card)
	return err
}

// DeleteEvent and DeleteContact are not yet implemented (create-only milestone).
func (d *DAV) DeleteEvent(ctx context.Context, colID, uid string) error {
	return errNotImplemented
}

func (d *DAV) DeleteContact(ctx context.Context, colID, uid string) error {
	return errNotImplemented
}

var errNotImplemented = fmt.Errorf("dav: delete not implemented")

// objectPath joins a collection href and a UID-derived filename into the object
// href used for PUT, tolerating a missing trailing slash on the collection.
func objectPath(collection, uid, ext string) string {
	return strings.TrimSuffix(collection, "/") + "/" + uid + ext
}
