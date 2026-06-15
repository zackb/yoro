package vcard

import (
	"bytes"
	"fmt"
	"time"

	govcard "github.com/emersion/go-vcard"

	"github.com/zackb/yoro/internal/model"
)

// BuildContact constructs a vCard from c. The caller must set c.UID. The card is
// normalized to vCard 4.0 (which also fills VERSION and derives FN from N if
// needed).
func BuildContact(c model.Contact) govcard.Card {
	card := govcard.Card{}
	card.SetValue(govcard.FieldUID, c.UID)
	if c.FN != "" {
		card.SetValue(govcard.FieldFormattedName, c.FN)
	}
	if c.Name.Family != "" || c.Name.Given != "" || c.Name.Additional != "" ||
		c.Name.Prefix != "" || c.Name.Suffix != "" {
		card.AddName(&govcard.Name{
			FamilyName:      c.Name.Family,
			GivenName:       c.Name.Given,
			AdditionalName:  c.Name.Additional,
			HonorificPrefix: c.Name.Prefix,
			HonorificSuffix: c.Name.Suffix,
		})
	}
	for _, e := range c.Emails {
		card.Add(govcard.FieldEmail, typedField(e))
	}
	for _, p := range c.Phones {
		card.Add(govcard.FieldTelephone, typedField(p))
	}
	if c.Org != "" {
		card.SetValue(govcard.FieldOrganization, c.Org)
	}
	if c.Title != "" {
		card.SetValue(govcard.FieldTitle, c.Title)
	}
	if c.Note != "" {
		card.SetValue(govcard.FieldNote, c.Note)
	}
	govcard.ToV4(card)
	return card
}

// typedField builds a vCard field carrying its TYPE labels (HOME/WORK/…).
func typedField(v model.TypedValue) *govcard.Field {
	f := &govcard.Field{Value: v.Value}
	if len(v.Types) > 0 {
		f.Params = govcard.Params{govcard.ParamType: append([]string(nil), v.Types...)}
	}
	return f
}

// UpdateContact decodes raw (the contact's original bytes) and mutates it in
// place from c — preserving the card's version, unmodeled properties, and any
// additional emails/phones beyond the first. Only the formatted name and the
// first email/phone are changed; REV is set to now. A blank field is left
// untouched (not removed). The caller re-encodes (local) or PUTs it (DAV).
func UpdateContact(raw []byte, c model.Contact) (govcard.Card, error) {
	card, err := govcard.NewDecoder(bytes.NewReader(raw)).Decode()
	if err != nil {
		return nil, err
	}
	if c.FN != "" {
		card.SetValue(govcard.FieldFormattedName, c.FN)
	}
	setFirstValue(card, govcard.FieldEmail, c.Emails)
	setFirstValue(card, govcard.FieldTelephone, c.Phones)
	card.SetValue(govcard.FieldRevision, time.Now().UTC().Format("20060102T150405Z"))
	if card.Value(govcard.FieldUID) != c.UID {
		return nil, fmt.Errorf("vcard: UID mismatch updating %q", c.UID)
	}
	return card, nil
}

// setFirstValue replaces the value of the first field of key, preserving its
// params (e.g. TYPE), or adds one if none exist. A nil/empty vals leaves any
// existing fields untouched.
func setFirstValue(card govcard.Card, key string, vals []model.TypedValue) {
	if len(vals) == 0 || vals[0].Value == "" {
		return
	}
	if existing := card[key]; len(existing) > 0 {
		existing[0].Value = vals[0].Value
		return
	}
	card.SetValue(key, vals[0].Value)
}

// Marshal encodes a card to vCard bytes.
func Marshal(card govcard.Card) ([]byte, error) {
	var buf bytes.Buffer
	if err := govcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
