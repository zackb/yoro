package vcard

import (
	"bytes"

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

// Marshal encodes a card to vCard bytes.
func Marshal(card govcard.Card) ([]byte, error) {
	var buf bytes.Buffer
	if err := govcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
