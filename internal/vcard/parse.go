// Package vcard parses vCard (.vcf) data into Yoro's domain model.
package vcard

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"time"

	govcard "github.com/emersion/go-vcard"

	"github.com/zackb/yoro/internal/model"
)

// Parse decodes a single .vcf file into one or more contacts. collectionID is
// stamped onto each contact.
func Parse(data []byte, collectionID string) ([]model.Contact, error) {
	var out []model.Contact
	dec := govcard.NewDecoder(bytes.NewReader(data))
	for {
		card, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, err
		}
		out = append(out, parseCard(card, collectionID, data))
	}
	return out, nil
}

func parseCard(card govcard.Card, collectionID string, raw []byte) model.Contact {
	c := model.Contact{
		CollectionID: collectionID,
		UID:          card.Value(govcard.FieldUID),
		FN:           card.PreferredValue(govcard.FieldFormattedName),
		Org:          card.Value(govcard.FieldOrganization),
		Title:        card.Value(govcard.FieldTitle),
		Note:         card.Value(govcard.FieldNote),
		Rev:          card.Value("REV"),
		Raw:          raw,
	}
	if n := card.Name(); n != nil {
		c.Name = model.StructuredName{
			Family:     n.FamilyName,
			Given:      n.GivenName,
			Additional: n.AdditionalName,
			Prefix:     n.HonorificPrefix,
			Suffix:     n.HonorificSuffix,
		}
	}
	if c.FN == "" {
		c.FN = displayFallback(c.Name)
	}
	c.Emails = typedValues(card[govcard.FieldEmail])
	c.Phones = typedValues(card[govcard.FieldTelephone])

	if b := card.Value(govcard.FieldBirthday); b != "" {
		if t, ok := parseDate(b); ok {
			c.Birthday = &t
		}
	}
	c.Photo = decodePhoto(card.Get(govcard.FieldPhoto))
	return c
}

func typedValues(fields []*govcard.Field) []model.TypedValue {
	var out []model.TypedValue
	for _, f := range fields {
		if f == nil || strings.TrimSpace(f.Value) == "" {
			continue
		}
		out = append(out, model.TypedValue{
			Value: f.Value,
			Types: normalizeTypes(f.Params.Types()),
		})
	}
	return out
}

// normalizeTypes lowercases and drops noise like "voice"/"internet" while
// keeping meaningful labels (home, work, cell, pref, ...).
func normalizeTypes(types []string) []string {
	var out []string
	for _, t := range types {
		t = strings.ToLower(strings.TrimSpace(t))
		switch t {
		case "", "voice", "internet":
			continue
		}
		out = append(out, t)
	}
	return out
}

func displayFallback(n model.StructuredName) string {
	parts := []string{}
	for _, p := range []string{n.Given, n.Family} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

func decodePhoto(f *govcard.Field) []byte {
	if f == nil {
		return nil
	}
	enc := strings.ToLower(strings.Join(f.Params["ENCODING"], ""))
	if enc != "b" && enc != "base64" {
		return nil // URI reference or unsupported; not embedded data
	}
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, f.Value)
	if data, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return data
	}
	return nil
}

// parseDate accepts common vCard date encodings.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "20060102", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
