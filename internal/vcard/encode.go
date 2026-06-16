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
	applyContactFields(card, c)
	govcard.ToV4(card)
	return card
}

// UpdateContact decodes raw (the contact's original bytes) and rewrites the
// modeled fields in place from c, preserving the card's version and any
// unmodeled properties (PHOTO, CATEGORIES, X-*, …). The form owns every modeled
// field, so a blank field clears its property and the email/phone/address lists
// are replaced wholesale. REV is set to now.
func UpdateContact(raw []byte, c model.Contact) (govcard.Card, error) {
	card, err := govcard.NewDecoder(bytes.NewReader(raw)).Decode()
	if err != nil {
		return nil, err
	}
	if card.Value(govcard.FieldUID) != c.UID {
		return nil, fmt.Errorf("vcard: UID mismatch updating %q", c.UID)
	}
	applyContactFields(card, c)
	card.SetValue(govcard.FieldRevision, time.Now().UTC().Format("20060102T150405Z"))
	return card, nil
}

// applyContactFields writes the modeled fields of c onto card, deleting then
// re-setting each managed key so the form is the source of truth. Unmanaged
// properties are left untouched. FN is derived from the structured name when not
// set explicitly. UID and REV are managed by the callers.
func applyContactFields(card govcard.Card, c model.Contact) {
	fn := c.FN
	if fn == "" {
		fn = displayFallback(c.Name)
	}
	setOrDel(card, govcard.FieldFormattedName, fn)

	delete(card, govcard.FieldName)
	n := c.Name
	if n.Family != "" || n.Given != "" || n.Additional != "" || n.Prefix != "" || n.Suffix != "" {
		card.AddName(&govcard.Name{
			FamilyName:      n.Family,
			GivenName:       n.Given,
			AdditionalName:  n.Additional,
			HonorificPrefix: n.Prefix,
			HonorificSuffix: n.Suffix,
		})
	}

	setOrDel(card, govcard.FieldNickname, c.Nickname)
	setOrDel(card, govcard.FieldOrganization, c.Org)
	setOrDel(card, govcard.FieldTitle, c.Title)
	setOrDel(card, govcard.FieldRole, c.Role)
	setOrDel(card, govcard.FieldURL, c.URL)
	setOrDel(card, govcard.FieldNote, c.Note)
	setOrDel(card, govcard.FieldBirthday, formatDate(c.Birthday))
	setOrDel(card, govcard.FieldAnniversary, formatDate(c.Anniversary))

	replaceTyped(card, govcard.FieldEmail, c.Emails)
	replaceTyped(card, govcard.FieldTelephone, c.Phones)

	delete(card, govcard.FieldAddress)
	for _, a := range c.Addresses {
		if !a.Empty() {
			card.AddAddress(buildAddress(a))
		}
	}
}

// replaceTyped clears key, then re-adds one field per non-empty typed value, so
// the model's list is the card's source of truth.
func replaceTyped(card govcard.Card, key string, vals []model.TypedValue) {
	delete(card, key)
	for _, v := range vals {
		if v.Value != "" {
			card.Add(key, typedField(v))
		}
	}
}

// setOrDel writes a single-valued field when val is non-empty, or removes it.
func setOrDel(card govcard.Card, key, val string) {
	if val == "" {
		delete(card, key)
		return
	}
	card.SetValue(key, val)
}

func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// typedField builds a vCard field carrying its TYPE labels (HOME/WORK/…).
func typedField(v model.TypedValue) *govcard.Field {
	f := &govcard.Field{Value: v.Value}
	if len(v.Types) > 0 {
		f.Params = govcard.Params{govcard.ParamType: append([]string(nil), v.Types...)}
	}
	return f
}

// buildAddress builds a go-vcard Address carrying its TYPE labels.
func buildAddress(a model.Address) *govcard.Address {
	adr := &govcard.Address{
		PostOfficeBox:   a.POBox,
		ExtendedAddress: a.Extended,
		StreetAddress:   a.Street,
		Locality:        a.Locality,
		Region:          a.Region,
		PostalCode:      a.PostalCode,
		Country:         a.Country,
	}
	if len(a.Types) > 0 {
		adr.Field = &govcard.Field{Params: govcard.Params{govcard.ParamType: append([]string(nil), a.Types...)}}
	}
	return adr
}

// Marshal encodes a card to vCard bytes.
func Marshal(card govcard.Card) ([]byte, error) {
	var buf bytes.Buffer
	if err := govcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
