package vcard

import "testing"

const card3 = `BEGIN:VCARD
VERSION:3.0
PRODID:-//Example Corp//Test Fixture//EN
N:Doe;Jane;;;
FN:Jane Doe
EMAIL;type=INTERNET;type=HOME;type=pref:jane.doe@example.com
TEL;type=OTHER;type=VOICE;type=pref:5550100100
TEL;type=CELL;type=VOICE:+15550100200
BDAY:1980-05-30
UID:00000000-0000-0000-0000-000000000001
REV:2015-12-08T00:09:07Z
END:VCARD
`

func TestParseCard(t *testing.T) {
	cs, err := Parse([]byte(card3), "icloud/card")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("want 1 contact, got %d", len(cs))
	}
	c := cs[0]
	if c.FN != "Jane Doe" {
		t.Errorf("FN: %q", c.FN)
	}
	if c.CollectionID != "icloud/card" {
		t.Errorf("collection: %q", c.CollectionID)
	}
	if c.Name.Family != "Doe" || c.Name.Given != "Jane" {
		t.Errorf("structured name: %+v", c.Name)
	}
	if len(c.Emails) != 1 || c.Emails[0].Value != "jane.doe@example.com" {
		t.Fatalf("emails: %+v", c.Emails)
	}
	// "voice"/"internet" should be filtered, meaningful labels kept.
	if !hasType(c.Emails[0].Types, "home") || !hasType(c.Emails[0].Types, "pref") {
		t.Errorf("email types not normalized: %v", c.Emails[0].Types)
	}
	for _, ty := range c.Emails[0].Types {
		if ty == "internet" || ty == "voice" {
			t.Errorf("noise type %q not filtered", ty)
		}
	}
	if len(c.Phones) != 2 {
		t.Fatalf("phones: %+v", c.Phones)
	}
	if c.Birthday == nil || c.Birthday.Format("2006-01-02") != "1980-05-30" {
		t.Errorf("birthday: %v", c.Birthday)
	}
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}
