package vcard

import (
	"strings"
	"testing"
	"time"

	"github.com/zackb/yoro/internal/model"
)

// TestUpdateContactPreservesFidelity confirms an update rewrites the modeled
// fields wholesale (the form owns them) while preserving truly unmodeled
// properties (PHOTO, X-*) and setting REV.
func TestUpdateContactPreservesFidelity(t *testing.T) {
	const raw = `BEGIN:VCARD
VERSION:3.0
UID:c-1
FN:Old Name
EMAIL;TYPE=HOME:old@example.com
EMAIL;TYPE=WORK:work@example.com
TEL:+15550000
TITLE:Former Title
NOTE:old note
PHOTO;ENCODING=b;TYPE=JPEG:aGVsbG8=
X-CUSTOM:hello
END:VCARD
`
	card, err := UpdateContact([]byte(raw), model.Contact{
		UID:    "c-1",
		FN:     "New Name",
		Emails: []model.TypedValue{{Value: "new@example.com", Types: []string{"home"}}},
		Note:   "new note",
		// Title omitted -> should be cleared.
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PHOTO") {
		t.Errorf("unmodeled PHOTO dropped:\n%s", data)
	}
	if !strings.Contains(string(data), "X-CUSTOM:hello") {
		t.Errorf("unmodeled X- property dropped:\n%s", data)
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
	if len(c.Emails) != 1 || c.Emails[0].Value != "new@example.com" {
		t.Errorf("emails not replaced wholesale: %+v", c.Emails)
	}
	if c.Note != "new note" {
		t.Errorf("note: %q", c.Note)
	}
	if c.Title != "" {
		t.Errorf("omitted TITLE not cleared: %q", c.Title)
	}
	if c.Rev == "" {
		t.Error("REV not set on update")
	}
}

// TestBuildContactAllFields round-trips the full set of modeled vCard fields.
func TestBuildContactAllFields(t *testing.T) {
	bday := time.Date(1990, 5, 1, 0, 0, 0, 0, time.UTC)
	anniv := time.Date(2015, 9, 12, 0, 0, 0, 0, time.UTC)
	c := model.Contact{
		UID:      "u-9",
		Name:     model.StructuredName{Prefix: "Dr.", Given: "Ada", Additional: "M", Family: "Lovelace", Suffix: "Sr."},
		FN:       "Dr. Ada M Lovelace Sr.",
		Nickname: "Countess",
		Org:      "Analytical Engine Co",
		Title:    "Mathematician",
		Role:     "Programmer",
		URL:      "https://ada.example",
		Emails: []model.TypedValue{
			{Value: "ada@home.example", Types: []string{"home"}},
			{Value: "ada@work.example", Types: []string{"work"}},
		},
		Phones: []model.TypedValue{{Value: "+15551234", Types: []string{"cell"}}},
		Addresses: []model.Address{{
			Types: []string{"home"}, Street: "1 Engine Way", Locality: "London",
			Region: "LDN", PostalCode: "EC1", Country: "UK",
		}},
		Birthday:    &bday,
		Anniversary: &anniv,
		Note:        "first programmer",
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
	if got.Name.Given != "Ada" || got.Name.Family != "Lovelace" || got.Name.Prefix != "Dr." {
		t.Errorf("structured name: %+v", got.Name)
	}
	if got.Nickname != "Countess" || got.Role != "Programmer" || got.URL != "https://ada.example" {
		t.Errorf("nickname/role/url: %q %q %q", got.Nickname, got.Role, got.URL)
	}
	if len(got.Emails) != 2 || got.Emails[1].Value != "ada@work.example" {
		t.Errorf("emails: %+v", got.Emails)
	}
	if len(got.Addresses) != 1 || got.Addresses[0].Locality != "London" || got.Addresses[0].Country != "UK" {
		t.Errorf("address: %+v", got.Addresses)
	}
	if got.Birthday == nil || !got.Birthday.Equal(bday) {
		t.Errorf("birthday: %v", got.Birthday)
	}
	if got.Anniversary == nil || !got.Anniversary.Equal(anniv) {
		t.Errorf("anniversary: %v", got.Anniversary)
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
