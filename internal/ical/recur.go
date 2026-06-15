package ical

import (
	"time"

	"github.com/teambition/rrule-go"

	"github.com/zackb/yoro/internal/model"
)

// Expand returns the occurrences of an event that start within [from, to).
// Non-recurring events yield at most one occurrence; recurring events are
// expanded over the window only (never infinitely). The base event's duration
// is preserved for every instance.
func Expand(e *model.Event, from, to time.Time) []model.Occurrence {
	if !e.Recurring() {
		if e.Start.Before(to) && !e.Start.Before(from) {
			return []model.Occurrence{occurrenceAt(e, e.Start)}
		}
		return nil
	}

	set := &rrule.Set{}
	set.DTStart(e.Start)
	if e.RRule != "" {
		if opt, err := rrule.StrToROption(e.RRule); err == nil {
			opt.Dtstart = e.Start
			if r, err := rrule.NewRRule(*opt); err == nil {
				set.RRule(r)
			}
		}
	}
	for _, t := range e.RDates {
		set.RDate(t)
	}
	for _, t := range e.ExDates {
		set.ExDate(t)
	}

	starts := set.Between(from, to, true)
	out := make([]model.Occurrence, 0, len(starts))
	for _, s := range starts {
		out = append(out, occurrenceAt(e, s))
	}
	return out
}

func occurrenceAt(e *model.Event, start time.Time) model.Occurrence {
	dur := e.End.Sub(e.Start)
	if dur < 0 {
		dur = 0
	}
	// Render in the viewer's local zone so a UTC- or TZID-stored instant shows the
	// right wall-clock and lands on the right agenda day. All-day events are
	// date-only and must not shift across zones.
	if !e.AllDay {
		start = start.Local()
	}
	return model.Occurrence{
		UID:          e.UID,
		CollectionID: e.CollectionID,
		Summary:      e.Summary,
		Start:        start,
		End:          start.Add(dur),
		AllDay:       e.AllDay,
		Event:        e,
	}
}
