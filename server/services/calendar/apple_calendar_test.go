package calendar

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/jonyTF/go-webdav/caldav"
	"schej.it/server/logger"
	"schej.it/server/models"
)

func TestMain(m *testing.M) {
	logger.Init(io.Discard)
	os.Exit(m.Run())
}

// isSelfForTest treats me@icloud.com and my alias me@example.com as the calendar owner
func isSelfForTest(address string) bool {
	address = normalizeCalendarAddress(address)
	return address == "me@icloud.com" || address == "me@example.com"
}

// calendarObject parses an iCalendar string (lines joined with CRLF)
func calendarObject(t *testing.T, lines ...string) caldav.CalendarObject {
	t.Helper()
	data := strings.Join(append(append([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//test//EN"}, lines...), "END:VCALENDAR"), "\r\n") + "\r\n"
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		t.Fatalf("failed to parse test calendar: %v", err)
	}
	return caldav.CalendarObject{Path: "/cal/event.ics", Data: cal}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func eventsFor(t *testing.T, objects ...caldav.CalendarObject) []models.CalendarEvent {
	t.Helper()
	return eventsFromCalendarObjects(objects, "/cal/", isSelfForTest)
}

func TestEventsFromCalendarObjects_ExpandedRecurringEvent(t *testing.T) {
	// What a server returns for a weekly event when the query uses <expand>
	events := eventsFor(t, calendarObject(t,
		"BEGIN:VEVENT", "UID:weekly", "SUMMARY:Standup",
		"RECURRENCE-ID:20261005T080000Z", "DTSTART:20261005T080000Z", "DTEND:20261005T083000Z", "END:VEVENT",
		"BEGIN:VEVENT", "UID:weekly", "SUMMARY:Standup",
		"RECURRENCE-ID:20261012T080000Z", "DTSTART:20261012T080000Z", "DTEND:20261012T083000Z", "END:VEVENT",
		"BEGIN:VEVENT", "UID:weekly", "SUMMARY:Standup (moved)",
		"RECURRENCE-ID:20261019T080000Z", "DTSTART:20261019T090000Z", "DTEND:20261019T093000Z", "END:VEVENT",
	))

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (one per occurrence)", len(events))
	}
	ids := map[string]bool{}
	for _, event := range events {
		if ids[event.Id] {
			t.Errorf("duplicate event id %q", event.Id)
		}
		ids[event.Id] = true
		if event.CalendarId != "/cal/" {
			t.Errorf("calendarId = %q, want /cal/", event.CalendarId)
		}
	}
	if got, want := events[2].StartDate.Time().UTC(), mustTime(t, "2026-10-19T09:00:00Z"); !got.Equal(want) {
		t.Errorf("moved occurrence starts at %v, want %v", got, want)
	}
	if events[2].Summary != "Standup (moved)" {
		t.Errorf("summary = %q", events[2].Summary)
	}
}

func TestEventsFromCalendarObjects_SingleEventKeepsUid(t *testing.T) {
	events := eventsFor(t, calendarObject(t,
		"BEGIN:VEVENT", "UID:abc-123", "SUMMARY:Dentist",
		"DTSTART:20261007T140000Z", "DTEND:20261007T150000Z", "END:VEVENT",
	))
	if len(events) != 1 || events[0].Id != "abc-123" {
		t.Fatalf("got %+v, want one event with id abc-123", events)
	}
	if events[0].Free || events[0].AllDay {
		t.Errorf("plain timed event should be busy and not all day: %+v", events[0])
	}
}

func TestEventsFromCalendarObjects_DurationInsteadOfEnd(t *testing.T) {
	events := eventsFor(t, calendarObject(t,
		"BEGIN:VEVENT", "UID:dur", "DTSTART:20261007T140000Z", "DURATION:PT1H30M", "END:VEVENT",
	))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if got, want := events[0].EndDate.Time().UTC(), mustTime(t, "2026-10-07T15:30:00Z"); !got.Equal(want) {
		t.Errorf("end = %v, want %v", got, want)
	}
}

func TestEventsFromCalendarObjects_TimezoneAndVtimezone(t *testing.T) {
	events := eventsFor(t, calendarObject(t,
		"BEGIN:VTIMEZONE", "TZID:Europe/Berlin", "END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:tz", "DTSTART;TZID=Europe/Berlin:20261012T100000",
		"DTEND;TZID=Europe/Berlin:20261012T110000", "END:VEVENT",
	))
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (VTIMEZONE must be ignored)", len(events))
	}
	// Berlin is UTC+2 in October (CEST)
	if got, want := events[0].StartDate.Time().UTC(), mustTime(t, "2026-10-12T08:00:00Z"); !got.Equal(want) {
		t.Errorf("start = %v, want %v", got, want)
	}
}

func TestEventsFromCalendarObjects_AllDayEvents(t *testing.T) {
	events := eventsFor(t,
		calendarObject(t,
			// Apple Calendar's default all day event: "Show As: Free"
			"BEGIN:VEVENT", "UID:birthday", "DTSTART;VALUE=DATE:20261010", "DTEND;VALUE=DATE:20261011",
			"TRANSP:TRANSPARENT", "END:VEVENT",
		),
		calendarObject(t,
			// All day without DTEND lasts one day; busy because it's opaque
			"BEGIN:VEVENT", "UID:trip", "DTSTART;VALUE=DATE:20261012", "TRANSP:OPAQUE", "END:VEVENT",
		),
	)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if !events[0].AllDay || !events[0].Free {
		t.Errorf("transparent all day event should be all day and free: %+v", events[0])
	}
	if !events[1].AllDay || events[1].Free {
		t.Errorf("opaque all day event should be all day and busy: %+v", events[1])
	}
	if got, want := events[1].EndDate.Time().UTC(), mustTime(t, "2026-10-13T00:00:00Z"); !got.Equal(want) {
		t.Errorf("all day end = %v, want %v", got, want)
	}
}

func TestEventsFromCalendarObjects_SkipsCancelledAndInvalid(t *testing.T) {
	events := eventsFor(t,
		calendarObject(t, "BEGIN:VEVENT", "UID:cancelled", "STATUS:CANCELLED",
			"DTSTART:20261007T140000Z", "DTEND:20261007T150000Z", "END:VEVENT"),
		calendarObject(t, "BEGIN:VEVENT", "UID:no-start", "SUMMARY:Broken", "END:VEVENT"),
		calendarObject(t, "BEGIN:VEVENT", "UID:bad-tz", "DTSTART;TZID=Not/AZone:20261007T140000",
			"DTEND;TZID=Not/AZone:20261007T150000", "END:VEVENT"),
		calendarObject(t, "BEGIN:VEVENT", "UID:ok", "DTSTART:20261008T140000Z", "DTEND:20261008T150000Z", "END:VEVENT"),
	)
	if len(events) != 1 || events[0].Id != "ok" {
		t.Fatalf("got %+v, want only the valid event", events)
	}
}

func TestIsVeventFree_Invitations(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		wantFree bool
	}{
		{"accepted invite blocks time", []string{
			"ORGANIZER:mailto:boss@work.com",
			"ATTENDEE;PARTSTAT=ACCEPTED:mailto:me@icloud.com",
		}, false},
		{"declined invite is free", []string{
			"ORGANIZER:mailto:boss@work.com",
			"ATTENDEE;PARTSTAT=DECLINED:mailto:me@icloud.com",
		}, true},
		{"unanswered invite is free", []string{
			"ORGANIZER:mailto:boss@work.com",
			"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:me@icloud.com",
		}, true},
		{"invite to an alias address is recognized", []string{
			"ORGANIZER:mailto:boss@work.com",
			"ATTENDEE;PARTSTAT=DECLINED:MAILTO:Me@Example.com",
		}, true},
		{"own meeting blocks time even without own attendee entry", []string{
			"ORGANIZER:mailto:me@icloud.com",
			"ATTENDEE;PARTSTAT=DECLINED:mailto:friend@example.org",
		}, false},
		{"other attendees' answers don't matter", []string{
			"ORGANIZER:mailto:boss@work.com",
			"ATTENDEE;PARTSTAT=DECLINED:mailto:friend@example.org",
		}, false},
		{"transparent wins over accepted", []string{
			"TRANSP:TRANSPARENT",
			"ATTENDEE;PARTSTAT=ACCEPTED:mailto:me@icloud.com",
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := append([]string{"BEGIN:VEVENT", "UID:invite", "DTSTART:20261007T140000Z", "DTEND:20261007T150000Z"}, tt.lines...)
			lines = append(lines, "END:VEVENT")
			events := eventsFor(t, calendarObject(t, lines...))
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if events[0].Free != tt.wantFree {
				t.Errorf("free = %v, want %v", events[0].Free, tt.wantFree)
			}
		})
	}
}

func TestIsVeventFree_NoAttendeesDoesNotLookUpAddresses(t *testing.T) {
	lookups := 0
	isSelf := func(address string) bool {
		lookups++
		return false
	}
	obj := calendarObject(t, "BEGIN:VEVENT", "UID:x", "DTSTART:20261007T140000Z", "DTEND:20261007T150000Z", "END:VEVENT")
	eventsFromCalendarObjects([]caldav.CalendarObject{obj}, "/cal/", isSelf)
	if lookups != 0 {
		t.Errorf("isSelf called %d times for an event without attendees, want 0", lookups)
	}
}

func TestParseCalendarUserAddressSet(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<multistatus xmlns="DAV:">
  <response>
    <href>/123456/principal/</href>
    <propstat>
      <prop>
        <calendar-user-address-set xmlns="urn:ietf:params:xml:ns:caldav">
          <href xmlns="DAV:">mailto:me@icloud.com</href>
          <href xmlns="DAV:">mailto:me@example.com</href>
          <href xmlns="DAV:">/123456/principal/</href>
        </calendar-user-address-set>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`
	hrefs, err := parseCalendarUserAddressSet(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(hrefs) != 3 || hrefs[0] != "mailto:me@icloud.com" || hrefs[1] != "mailto:me@example.com" {
		t.Errorf("hrefs = %v", hrefs)
	}
}
