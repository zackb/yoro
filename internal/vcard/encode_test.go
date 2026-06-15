package vcard

import (
	"testing"

	"github.com/zackb/yoro/internal/model"
)

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
