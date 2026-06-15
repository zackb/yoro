// Package ical parses iCalendar (.ics) data into Yoro's domain model.
// It deliberately does not expand recurrence; the store does that in a window
// (see package store). Recurrence rules are retained verbatim on model.Event.
package ical

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"time"

	goical "github.com/emersion/go-ical"

	"github.com/zackb/yoro/internal/model"
)

// File is the parsed result of one .ics file: zero or more events and todos.
type File struct {
	Events []model.Event
	Todos  []model.Todo
}

// Parse decodes a single .ics file's bytes. collectionID is stamped onto every
// produced event/todo. Unparseable individual components are skipped rather
// than failing the whole file.
func Parse(data []byte, collectionID string) (File, error) {
	var out File
	dec := goical.NewDecoder(bytes.NewReader(data))
	for {
		cal, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Return what we have alongside the error so callers can decide.
			return out, err
		}
		for _, child := range cal.Children {
			switch child.Name {
			case goical.CompEvent:
				if ev, ok := parseEvent(child, collectionID); ok {
					out.Events = append(out.Events, ev)
				}
			case goical.CompToDo:
				out.Todos = append(out.Todos, parseTodo(child, collectionID))
			}
		}
	}
	// Attach the raw bytes to each event/todo for future round-tripping.
	for i := range out.Events {
		out.Events[i].Raw = data
	}
	for i := range out.Todos {
		out.Todos[i].Raw = data
	}
	return out, nil
}

func parseEvent(c *goical.Component, collectionID string) (model.Event, bool) {
	ev := model.Event{
		CollectionID: collectionID,
		UID:          text(c, goical.PropUID),
		Summary:      text(c, goical.PropSummary),
		Description:  text(c, goical.PropDescription),
		Location:     text(c, goical.PropLocation),
		Status:       text(c, goical.PropStatus),
		Rev:          text(c, "LAST-MODIFIED"),
	}
	if s := text(c, goical.PropSequence); s != "" {
		ev.Sequence, _ = strconv.Atoi(s)
	}

	start, allDay, ok := dateTime(c, goical.PropDateTimeStart)
	if !ok {
		return model.Event{}, false // an event without a start is unusable
	}
	ev.Start = start
	ev.AllDay = allDay

	if end, _, ok := dateTime(c, goical.PropDateTimeEnd); ok {
		ev.End = end
	} else if dur := text(c, "DURATION"); dur != "" {
		ev.End = ev.Start.Add(parseDuration(dur))
	} else if allDay {
		ev.End = ev.Start.AddDate(0, 0, 1)
	} else {
		ev.End = ev.Start
	}

	if p := c.Props.Get(goical.PropRecurrenceRule); p != nil {
		ev.RRule = strings.TrimSpace(p.Value)
	}
	ev.RDates = dateList(c, "RDATE")
	ev.ExDates = dateList(c, goical.PropExceptionDates)

	for _, p := range c.Props[goical.PropAttendee] {
		ev.Attendees = append(ev.Attendees, parseAttendee(p))
	}
	for _, child := range c.Children {
		if child.Name == goical.CompAlarm {
			ev.Alarms = append(ev.Alarms, model.Alarm{
				Trigger:     text(child, "TRIGGER"),
				Description: text(child, goical.PropDescription),
			})
		}
	}
	return ev, true
}

func parseTodo(c *goical.Component, collectionID string) model.Todo {
	td := model.Todo{
		CollectionID: collectionID,
		UID:          text(c, goical.PropUID),
		Summary:      text(c, goical.PropSummary),
		Status:       text(c, goical.PropStatus),
	}
	if due, _, ok := dateTime(c, "DUE"); ok {
		td.Due = &due
	}
	if comp, _, ok := dateTime(c, "COMPLETED"); ok {
		td.Completed = &comp
	}
	return td
}

func parseAttendee(p goical.Prop) model.Attendee {
	a := model.Attendee{
		Name:   p.Params.Get("CN"),
		Status: p.Params.Get("PARTSTAT"),
		Role:   p.Params.Get("ROLE"),
	}
	a.Email = strings.TrimPrefix(strings.ToLower(p.Value), "mailto:")
	return a
}

// text returns a property's value, or "" if absent.
func text(c *goical.Component, name string) string {
	if p := c.Props.Get(name); p != nil {
		return p.Value
	}
	return ""
}

// dateTime resolves a date or date-time property, honoring a TZID parameter and
// VALUE=DATE (all-day). Floating times fall back to local.
func dateTime(c *goical.Component, name string) (t time.Time, allDay bool, ok bool) {
	p := c.Props.Get(name)
	if p == nil {
		return time.Time{}, false, false
	}
	return parsePropTime(p)
}

func parsePropTime(p *goical.Prop) (time.Time, bool, bool) {
	v := strings.TrimSpace(p.Value)
	loc := loadLocation(p.Params.Get(goical.ParamTimezoneID))

	if p.Params.Get(goical.ParamValue) == "DATE" || len(v) == 8 {
		if t, err := time.ParseInLocation("20060102", v, loc); err == nil {
			return t, true, true
		}
	}
	// UTC (trailing Z)
	if strings.HasSuffix(v, "Z") {
		if t, err := time.ParseInLocation("20060102T150405Z", v, time.UTC); err == nil {
			return t, false, true
		}
	}
	if t, err := time.ParseInLocation("20060102T150405", v, loc); err == nil {
		return t, false, true
	}
	return time.Time{}, false, false
}

// dateList parses comma-separated RDATE/EXDATE properties (possibly repeated).
func dateList(c *goical.Component, name string) []time.Time {
	var out []time.Time
	for _, p := range c.Props[name] {
		loc := loadLocation(p.Params.Get(goical.ParamTimezoneID))
		for _, raw := range strings.Split(p.Value, ",") {
			pp := goical.Prop{Value: raw, Params: p.Params}
			_ = loc
			if t, _, ok := parsePropTime(&pp); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// loadLocation resolves an IANA TZID, falling back to UTC.
func loadLocation(tzid string) *time.Location {
	if tzid == "" {
		return time.Local
	}
	if loc, err := time.LoadLocation(tzid); err == nil {
		return loc
	}
	return time.UTC
}

// parseDuration converts an iCalendar DURATION (e.g. "PT1H30M", "P1D") to a
// time.Duration. Unsupported components are ignored.
func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	s = strings.TrimPrefix(s, "P")
	var d time.Duration
	inTime := false
	num := ""
	for _, r := range s {
		switch r {
		case 'T':
			inTime = true
		case 'W':
			d += parseNum(num) * 7 * 24 * time.Hour
			num = ""
		case 'D':
			d += parseNum(num) * 24 * time.Hour
			num = ""
		case 'H':
			d += parseNum(num) * time.Hour
			num = ""
		case 'M':
			if inTime {
				d += parseNum(num) * time.Minute
			}
			num = ""
		case 'S':
			d += parseNum(num) * time.Second
			num = ""
		default:
			num += string(r)
		}
	}
	if neg {
		d = -d
	}
	return d
}

func parseNum(s string) time.Duration {
	n, _ := strconv.Atoi(s)
	return time.Duration(n)
}
