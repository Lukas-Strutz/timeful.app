package calendar

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-ical"
	"github.com/jonyTF/go-webdav"
	"github.com/jonyTF/go-webdav/caldav"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"schej.it/server/logger"
	"schej.it/server/models"
	"schej.it/server/utils"
)

const appleCaldavEndpoint = "https://caldav.icloud.com"

type AppleCalendar struct {
	models.AppleCalendarAuth
}

func (calendar *AppleCalendar) GetCalendarList() (map[string]models.SubCalendar, error) {
	webdavClient, caldavClient, err := calendar.getClients()
	if err != nil {
		return nil, err
	}

	principal, err := webdavClient.FindCurrentUserPrincipal(context.Background())
	if err != nil {
		return nil, err
	}

	calendarHomeSet, err := caldavClient.FindCalendarHomeSet(context.Background(), principal)
	if err != nil {
		return nil, err
	}

	calendars, err := caldavClient.FindCalendars(context.Background(), calendarHomeSet)
	if err != nil {
		return nil, err
	}

	// Only include calendars that support VEVENT
	filteredCalendars := make(map[string]models.SubCalendar)
	for _, calendar := range calendars {
		for _, supportedComponent := range calendar.SupportedComponentSet {
			if supportedComponent == "VEVENT" {
				filteredCalendars[calendar.Path] = models.SubCalendar{
					Name:    calendar.Name,
					Enabled: utils.TruePtr(),
				}
				break
			}
		}
	}

	return filteredCalendars, nil
}

func (calendar *AppleCalendar) GetCalendarEvents(calendarId string, timeMin time.Time, timeMax time.Time) ([]models.CalendarEvent, error) {
	webdavClient, caldavClient, err := calendar.getClients()
	if err != nil {
		return nil, err
	}

	// Get events. Expand makes the server return one VEVENT per occurrence of
	// recurring events, with times converted to UTC.
	objects, err := caldavClient.QueryCalendar(context.Background(), calendarId, &caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name: "VCALENDAR",
			Comps: []caldav.CalendarCompRequest{{
				Name: "VEVENT",
				Props: []string{
					ical.PropSummary,
					ical.PropUID,
					ical.PropDateTimeStart,
					ical.PropDateTimeEnd,
					ical.PropDuration,
					ical.PropRecurrenceID,
					ical.PropTransparency,
					ical.PropStatus,
					ical.PropOrganizer,
					ical.PropAttendee,
				},
			}},
			Expand: &caldav.CalendarExpandRequest{
				Start: timeMin,
				End:   timeMax,
			},
		},
		CompFilter: caldav.CompFilter{
			Name: "VCALENDAR",
			Comps: []caldav.CompFilter{{
				Name:  "VEVENT",
				Start: timeMin,
				End:   timeMax,
			}},
		},
	})
	if err != nil {
		return nil, err
	}

	// Only look up the account's own addresses if an event has attendees
	var once sync.Once
	var selfAddresses map[string]bool
	isSelf := func(address string) bool {
		once.Do(func() {
			selfAddresses = calendar.getSelfAddresses(webdavClient)
		})
		return selfAddresses[normalizeCalendarAddress(address)]
	}

	return eventsFromCalendarObjects(objects, calendarId, isSelf), nil
}

// eventsFromCalendarObjects converts the VEVENTs of the given CalDAV objects
// into calendar events. isSelf reports whether a calendar user address (e.g.
// "mailto:me@icloud.com") belongs to the calendar's owner.
func eventsFromCalendarObjects(objects []caldav.CalendarObject, calendarId string, isSelf func(string) bool) []models.CalendarEvent {
	calendarEvents := make([]models.CalendarEvent, 0)
	for _, object := range objects {
		if object.Data == nil {
			continue
		}

		vevents := make([]*ical.Component, 0, len(object.Data.Children))
		for _, child := range object.Data.Children {
			if child.Name == ical.CompEvent {
				vevents = append(vevents, child)
			}
		}

		for _, vevent := range vevents {
			calendarEvent, ok := eventFromVevent(vevent, calendarId, isSelf)
			if !ok {
				continue
			}

			// Occurrences of a recurring event share a UID, but event ids must be unique
			if len(vevents) > 1 || vevent.Props.Get(ical.PropRecurrenceID) != nil {
				calendarEvent.Id = fmt.Sprintf("%s_%s", calendarEvent.Id, calendarEvent.StartDate.Time().UTC().Format("20060102T150405Z"))
			}

			calendarEvents = append(calendarEvents, calendarEvent)
		}
	}

	return calendarEvents
}

// eventFromVevent converts a single VEVENT. ok is false if the event should be
// skipped (cancelled, or missing / unparsable times).
func eventFromVevent(vevent *ical.Component, calendarId string, isSelf func(string) bool) (calendarEvent models.CalendarEvent, ok bool) {
	if status := vevent.Props.Get(ical.PropStatus); status != nil && strings.EqualFold(status.Value, string(ical.EventCancelled)) {
		return models.CalendarEvent{}, false
	}

	dtStart := vevent.Props.Get(ical.PropDateTimeStart)
	if dtStart == nil {
		return models.CalendarEvent{}, false
	}

	// Handles DTEND, DURATION, and all day events without an end
	event := ical.Event{Component: vevent}
	startTime, err := event.DateTimeStart(time.UTC)
	if err != nil {
		logger.StdErr.Printf("Skipping Apple Calendar event with invalid start %q: %v\n", dtStart.Value, err)
		return models.CalendarEvent{}, false
	}
	endTime, err := event.DateTimeEnd(time.UTC)
	if err != nil {
		logger.StdErr.Printf("Skipping Apple Calendar event with invalid end: %v\n", err)
		return models.CalendarEvent{}, false
	}

	allDay := dtStart.ValueType() == ical.ValueDate || (dtStart.ValueType() == ical.ValueDefault && len(dtStart.Value) == len("20060102"))

	return models.CalendarEvent{
		Id:         propValue(vevent, ical.PropUID),
		CalendarId: calendarId,
		Summary:    propValue(vevent, ical.PropSummary),
		StartDate:  primitive.NewDateTimeFromTime(startTime),
		EndDate:    primitive.NewDateTimeFromTime(endTime),
		Free:       isVeventFree(vevent, isSelf),
		AllDay:     allDay,
	}, true
}

// isVeventFree mirrors the Google Calendar rules: an event doesn't block time
// if it is marked as free (TRANSP:TRANSPARENT, which Apple Calendar uses for
// "Show As: Free" and by default for all day events), or if it's an invitation
// the owner hasn't accepted.
func isVeventFree(vevent *ical.Component, isSelf func(string) bool) bool {
	if transp := vevent.Props.Get(ical.PropTransparency); transp != nil && strings.EqualFold(transp.Value, "TRANSPARENT") {
		return true
	}

	attendees := vevent.Props.Values(ical.PropAttendee)
	if len(attendees) == 0 {
		return false
	}

	// The organizer's own events always block time
	if organizer := vevent.Props.Get(ical.PropOrganizer); organizer != nil && isSelf(organizer.Value) {
		return false
	}

	for _, attendee := range attendees {
		if isSelf(attendee.Value) {
			return !strings.EqualFold(attendee.Params.Get(ical.ParamParticipationStatus), "ACCEPTED")
		}
	}

	return false
}

func propValue(component *ical.Component, name string) string {
	if prop := component.Props.Get(name); prop != nil {
		return prop.Value
	}
	return ""
}

// normalizeCalendarAddress lowercases a calendar user address and strips the
// "mailto:" scheme, so "MAILTO:Me@iCloud.com" and "me@icloud.com" compare equal
func normalizeCalendarAddress(address string) string {
	address = strings.ToLower(strings.TrimSpace(address))
	return strings.TrimPrefix(address, "mailto:")
}

type appleSelfAddresses struct {
	addresses map[string]bool
	fetchedAt time.Time
}

// appleSelfAddressCache caches each Apple account's calendar-user-address-set
// (its email aliases), keyed by Apple ID
var appleSelfAddressCache sync.Map

const appleSelfAddressCacheTTL = time.Hour

// getSelfAddresses returns the account's own calendar user addresses
// (normalized), which identify the owner among an event's attendees. Falls
// back to just the Apple ID if they can't be fetched.
func (calendar *AppleCalendar) getSelfAddresses(webdavClient *webdav.Client) map[string]bool {
	if cached, ok := appleSelfAddressCache.Load(calendar.Email); ok {
		entry := cached.(appleSelfAddresses)
		if time.Since(entry.fetchedAt) < appleSelfAddressCacheTTL {
			return entry.addresses
		}
	}

	addresses := map[string]bool{normalizeCalendarAddress(calendar.Email): true}

	fetched, err := calendar.fetchCalendarUserAddresses(webdavClient)
	if err != nil {
		logger.StdErr.Println("Failed to fetch Apple calendar-user-address-set:", err)
		return addresses
	}
	for _, address := range fetched {
		addresses[normalizeCalendarAddress(address)] = true
	}

	appleSelfAddressCache.Store(calendar.Email, appleSelfAddresses{addresses: addresses, fetchedAt: time.Now()})
	return addresses
}

// fetchCalendarUserAddresses PROPFINDs the principal's
// calendar-user-address-set (RFC 6638), which go-webdav doesn't expose
func (calendar *AppleCalendar) fetchCalendarUserAddresses(webdavClient *webdav.Client) ([]string, error) {
	principal, err := webdavClient.FindCurrentUserPrincipal(context.Background())
	if err != nil {
		return nil, err
	}

	httpClient, err := calendar.getHttpClient()
	if err != nil {
		return nil, err
	}

	body := `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-user-address-set/></d:prop></d:propfind>`
	req, err := http.NewRequest("PROPFIND", appleCaldavEndpoint+principal, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PROPFIND calendar-user-address-set returned status %d", resp.StatusCode)
	}

	return parseCalendarUserAddressSet(resp.Body)
}

// parseCalendarUserAddressSet extracts the hrefs from a PROPFIND multistatus
// response for calendar-user-address-set
func parseCalendarUserAddressSet(body io.Reader) ([]string, error) {
	var multistatus struct {
		Hrefs []string `xml:"response>propstat>prop>calendar-user-address-set>href"`
	}
	if err := xml.NewDecoder(body).Decode(&multistatus); err != nil {
		return nil, err
	}
	return multistatus.Hrefs, nil
}

func (calendar *AppleCalendar) getHttpClient() (webdav.HTTPClient, error) {
	decryptedPassword, err := utils.Decrypt(calendar.Password)
	if err != nil {
		return nil, err
	}
	return webdav.HTTPClientWithBasicAuth(nil, calendar.Email, decryptedPassword), nil
}

func (calendar *AppleCalendar) getClients() (*webdav.Client, *caldav.Client, error) {
	httpClient, err := calendar.getHttpClient()
	if err != nil {
		return nil, nil, err
	}

	webdavClient, err := webdav.NewClient(httpClient, appleCaldavEndpoint)
	if err != nil {
		return nil, nil, err
	}

	caldavClient, err := caldav.NewClient(httpClient, appleCaldavEndpoint)
	if err != nil {
		return nil, nil, err
	}

	return webdavClient, caldavClient, nil
}

func parseTimeWithTZ(prop *ical.Prop) (time.Time, error) {
	timeStr := prop.Value
	tzID := prop.Params.Get("TZID")

	var t time.Time
	var err error

	if tzID != "" {
		loc, err := time.LoadLocation(tzID)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid timezone: %v", err)
		}
		//lint:ignore SA4006 err is in fact used later in the code
		t, err = time.ParseInLocation("20060102T150405", timeStr, loc)
	} else {
		t, err = time.Parse("20060102T150405Z", timeStr)
	}

	if err != nil {
		return time.Time{}, fmt.Errorf("unable to parse time: %v", err)
	}

	return t, nil
}
