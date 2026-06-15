package vcard

import (
	"testing"

	"github.com/zackb/yoro/internal/model"
)

// TestUpdateContactPreservesFidelity confirms editing a contact changes the name
// and first email while preserving additional emails and unmodeled fields.
func TestUpdateContactPreservesFidelity(t *testing.T) {
	const raw = `BEGIN:VCARD
VERSION:3.0
UID:c-1
FN:Old Name
EMAIL;TYPE=HOME:old@example.com
EMAIL;TYPE=WORK:work@example.com
TEL:+15550000
NOTE:keep this note
END:VCARD
`
	card, err := UpdateContact([]byte(raw), model.Contact{
		UID:    "c-1",
		FN:     "New Name",
		Emails: []model.TypedValue{{Value: "new@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := Parse(data, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("want 1 contact, got %d", len(cs))
	}
	c := cs[0]
	if c.FN != "New Name" {
		t.Errorf("fn: %q", c.FN)
	}
	if len(c.Emails) != 2 {
		t.Fatalf("expected both emails preserved, got %+v", c.Emails)
	}
	if c.Emails[0].Value != "new@example.com" {
		t.Errorf("first email not updated: %q", c.Emails[0].Value)
	}
	if c.Emails[1].Value != "work@example.com" {
		t.Errorf("second email not preserved: %q", c.Emails[1].Value)
	}
	if c.Note != "keep this note" {
		t.Errorf("unmodeled NOTE dropped: %q", c.Note)
	}
	if c.Rev == "" {
		t.Error("REV not set on update")
	}
}

// TestBuildContactRoundTrip confirms a built contact marshals to .vcf bytes that
// Parse reads back faithfully.
func TestBuildContactRoundTrip(t *testing.T) {
	c := model.Contact{
		UID:    "u-1",
		FN:     "Ada Lovelace",
		Emails: []model.TypedValue{{Value: "ada@example.com", Types: []string{"home"}}},
		Phones: []model.TypedValue{{Value: "+15551234"}},
	}
	data, err := Marshal(BuildContact(c))
	if err != nil {
		t.Fatal(err)
	}
	cs, err := Parse(data, "col")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("want 1 contact, got %d", len(cs))
	}
	got := cs[0]
	if got.UID != "u-1" || got.FN != "Ada Lovelace" {
		t.Errorf("uid/fn: %q %q", got.UID, got.FN)
	}
	if len(got.Emails) != 1 || got.Emails[0].Value != "ada@example.com" {
		t.Errorf("emails: %+v", got.Emails)
	}
	if len(got.Phones) != 1 || got.Phones[0].Value != "+15551234" {
		t.Errorf("phones: %+v", got.Phones)
	}
}
