package ical

import (
	"bytes"
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
	if e.Summary != "" {
		ev.Props.SetText(goical.PropSummary, e.Summary)
	}
	if e.Location != "" {
		ev.Props.SetText(goical.PropLocation, e.Location)
	}
	if e.Description != "" {
		ev.Props.SetText(goical.PropDescription, e.Description)
	}
	if e.AllDay {
		ev.Props.SetDate(goical.PropDateTimeStart, e.Start)
		ev.Props.SetDate(goical.PropDateTimeEnd, e.End)
	} else {
		ev.Props.SetDateTime(goical.PropDateTimeStart, e.Start.UTC())
		ev.Props.SetDateTime(goical.PropDateTimeEnd, e.End.UTC())
	}
	cal.Children = append(cal.Children, ev.Component)
	return cal
}

// Marshal encodes a calendar to iCalendar bytes.
func Marshal(cal *goical.Calendar) ([]byte, error) {
	var buf bytes.Buffer
	if err := goical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
