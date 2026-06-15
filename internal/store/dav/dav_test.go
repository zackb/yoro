package dav

import (
	"testing"
	"time"

	goical "github.com/emersion/go-ical"
	govcard "github.com/emersion/go-vcard"

	"github.com/zackb/yoro/internal/ical"
	"github.com/zackb/yoro/internal/vcard"
)

// TestEncodeICalReuse confirms a parsed go-ical Calendar (as returned by the
// CalDAV client) re-encodes into bytes the existing ical parser accepts.
func TestEncodeICalReuse(t *testing.T) {
	cal := goical.NewCalendar()
	cal.Props.SetText(goical.PropProductID, "-//yoro//test//EN")
	cal.Props.SetText(goical.PropVersion, "2.0")
	ev := goical.NewEvent()
	ev.Props.SetText(goical.PropUID, "e1")
	ev.Props.SetText(goical.PropSummary, "Launch")
	ev.Props.SetDateTime(goical.PropDateTimeStamp, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	ev.Props.SetDateTime(goical.PropDateTimeStart, time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC))
	ev.Props.SetDateTime(goical.PropDateTimeEnd, time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC))
	cal.Children = append(cal.Children, ev.Component)

	data, err := encodeICal(cal)
	if err != nil {
		t.Fatalf("encodeICal: %v", err)
	}
	f, err := ical.Parse(data, "icloud\x1f/work/")
	if err != nil {
		t.Fatalf("ical.Parse: %v", err)
	}
	if len(f.Events) != 1 || f.Events[0].Summary != "Launch" {
		t.Fatalf("got %d events %+v, want 1 'Launch'", len(f.Events), f.Events)
	}
}

// TestEncodeVCardReuse confirms a parsed go-vcard Card re-encodes into bytes the
// existing vcard parser accepts.
func TestEncodeVCardReuse(t *testing.T) {
	card := govcard.Card{}
	card.SetValue(govcard.FieldUID, "u1")
	card.SetValue(govcard.FieldFormattedName, "Ada Lovelace")
	card.SetValue(govcard.FieldEmail, "ada@example.com")
	govcard.ToV4(card)

	data, err := encodeVCard(card)
	if err != nil {
		t.Fatalf("encodeVCard: %v", err)
	}
	cs, err := vcard.Parse(data, "icloud\x1f/contacts/")
	if err != nil {
		t.Fatalf("vcard.Parse: %v", err)
	}
	if len(cs) != 1 || cs[0].FN != "Ada Lovelace" {
		t.Fatalf("got %d contacts %+v, want 1 'Ada Lovelace'", len(cs), cs)
	}
	if len(cs[0].Emails) != 1 || cs[0].Emails[0].Value != "ada@example.com" {
		t.Fatalf("email not parsed: %+v", cs[0].Emails)
	}
}
