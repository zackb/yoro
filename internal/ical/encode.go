package ical

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	goical "github.com/emersion/go-ical"

	"github.com/zackb/yoro/internal/model"
)

// prodID identifies Yoro as the producer of generated calendars.
const prodID = "-//yoro//yoro//EN"

// BuildEvent constructs a VCALENDAR wrapping a single VEVENT from e. The caller
// must set e.UID. DTSTAMP is set to now. Text fields are escaped by go-ical (the
// inverse of unescapeText). Timed events are written in UTC (portable "Z" form,
// avoiding a TZID=Local that DAV servers reject); all-day events use bare DATE
// values.
func BuildEvent(e model.Event) *goical.Calendar {
	cal := goical.NewCalendar()
	cal.Props.SetText(goical.PropProductID, prodID)
	cal.Props.SetText(goical.PropVersion, "2.0")

	ev := goical.NewEvent()
	ev.Props.SetText(goical.PropUID, e.UID)
	ev.Props.SetDateTime(goical.PropDateTimeStamp, time.Now().UTC())
	applyEventProps(ev.Props, e)
	cal.Children = append(cal.Children, ev.Component)
	return cal
}

// UpdateEvent decodes raw (the event's original bytes), mutates the VEVENT whose
// UID matches e in place — preserving unmodeled properties and any sibling
// components — and returns the calendar. SEQUENCE is bumped and DTSTAMP/
// LAST-MODIFIED set to now. The caller re-encodes (local) or PUTs it (DAV).
func UpdateEvent(raw []byte, e model.Event) (*goical.Calendar, error) {
	cal, err := goical.NewDecoder(bytes.NewReader(raw)).Decode()
	if err != nil {
		return nil, err
	}
	var target *goical.Component
	for _, child := range cal.Children {
		if child.Name == goical.CompEvent && text(child, goical.PropUID) == e.UID {
			target = child
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("ical: event %q not found for update", e.UID)
	}
	applyEventProps(target.Props, e)
	seq := 0
	if s := text(target, goical.PropSequence); s != "" {
		seq, _ = strconv.Atoi(s)
	}
	target.Props.SetText(goical.PropSequence, strconv.Itoa(seq+1))
	now := time.Now().UTC()
	target.Props.SetDateTime(goical.PropDateTimeStamp, now)
	target.Props.SetDateTime("LAST-MODIFIED", now)
	return cal, nil
}

// applyEventProps sets the editable VEVENT properties from e. Empty optional
// fields (location, description) are left untouched so an update preserves
// values the form doesn't expose. Timed starts are written in UTC; all-day uses
// bare DATE values.
func applyEventProps(props goical.Props, e model.Event) {
	if e.Summary != "" {
		props.SetText(goical.PropSummary, e.Summary)
	}
	if e.Location != "" {
		props.SetText(goical.PropLocation, e.Location)
	}
	if e.Description != "" {
		props.SetText(goical.PropDescription, e.Description)
	}
	if e.AllDay {
		props.SetDate(goical.PropDateTimeStart, e.Start)
		props.SetDate(goical.PropDateTimeEnd, e.End)
	} else {
		props.SetDateTime(goical.PropDateTimeStart, e.Start.UTC())
		props.SetDateTime(goical.PropDateTimeEnd, e.End.UTC())
	}
}

// Marshal encodes a calendar to iCalendar bytes.
func Marshal(cal *goical.Calendar) ([]byte, error) {
	var buf bytes.Buffer
	if err := goical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
