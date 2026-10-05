package calendar

import (
	"bytes"
	"time"

	"github.com/emersion/go-ical"
	"schej.it/server/models"
)

// ScheduledEventDetails describes a scheduled Timeful event to be written to a
// calendar or exported as an .ics file
type ScheduledEventDetails struct {
	// Stable across reschedules, so calendars treat updates as the same event
	UID         string
	Summary     string
	Description string
	URL         string
	Start       time.Time
	End         time.Time
}

// CalendarEventWriter is implemented by calendar providers that can add
// scheduled events to one of the user's calendars
type CalendarEventWriter interface {
	// PutEvent creates the event in calendarId, or replaces it if an event
	// with the same fileName exists, and returns an id for DeleteEvent
	PutEvent(calendarId string, fileName string, details ScheduledEventDetails) (objectId string, err error)
	// DeleteEvent removes an event created by PutEvent. Deleting an event
	// that no longer exists is not an error.
	DeleteEvent(objectId string) error
}

// GetCalendarEventWriter returns the writer for the given calendar account, or
// nil if its provider doesn't support writing events
func GetCalendarEventWriter(calendarAccount models.CalendarAccount) CalendarEventWriter {
	switch calendarAccount.CalendarType {
	case models.AppleCalendarType:
		if calendarAccount.AppleCalendarAuth == nil {
			return nil
		}
		return &AppleCalendar{
			AppleCalendarAuth: *calendarAccount.AppleCalendarAuth,
		}
	}
	return nil
}

// NewScheduledEventCalendar builds an iCalendar object containing the given
// event. Times are written in UTC.
func NewScheduledEventCalendar(details ScheduledEventDetails) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Timeful//Timeful//EN")
	cal.Props.SetText(ical.PropCalendarScale, "GREGORIAN")

	now := time.Now().UTC()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, details.UID)
	event.Props.SetDateTime(ical.PropDateTimeStamp, now)
	event.Props.SetDateTime(ical.PropLastModified, now)
	event.Props.SetDateTime(ical.PropDateTimeStart, details.Start.UTC())
	event.Props.SetDateTime(ical.PropDateTimeEnd, details.End.UTC())
	event.Props.SetText(ical.PropSummary, details.Summary)
	if details.Description != "" {
		event.Props.SetText(ical.PropDescription, details.Description)
	}
	if details.URL != "" {
		urlProp := ical.NewProp(ical.PropURL)
		urlProp.SetValueType(ical.ValueURI)
		urlProp.Value = details.URL
		event.Props.Set(urlProp)
	}
	event.Props.SetText(ical.PropTransparency, "OPAQUE")
	cal.Children = append(cal.Children, event.Component)

	return cal
}

// EncodeScheduledEventIcs returns the given event as an .ics file
func EncodeScheduledEventIcs(details ScheduledEventDetails) ([]byte, error) {
	cal := NewScheduledEventCalendar(details)
	// PUBLISH lets calendar apps import the file as a plain event
	cal.Props.SetText(ical.PropMethod, "PUBLISH")

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
