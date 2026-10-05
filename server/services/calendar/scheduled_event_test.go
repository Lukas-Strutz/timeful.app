package calendar

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"schej.it/server/models"
	"schej.it/server/utils"
)

func testScheduledEventDetails() ScheduledEventDetails {
	return ScheduledEventDetails{
		UID:         "timeful-abc@time.example.com",
		Summary:     "Team dinner, Friday; bring snacks",
		Description: "Responded (2): Ann, Bob\n\nScheduled with Timeful: https://time.example.com/e/abc",
		URL:         "https://time.example.com/e/abc",
		Start:       time.Date(2026, 10, 16, 18, 0, 0, 0, time.FixedZone("CEST", 2*60*60)),
		End:         time.Date(2026, 10, 16, 20, 0, 0, 0, time.FixedZone("CEST", 2*60*60)),
	}
}

func TestEncodeScheduledEventIcs_RoundTrips(t *testing.T) {
	ics, err := EncodeScheduledEventIcs(testScheduledEventDetails())
	if err != nil {
		t.Fatal(err)
	}
	text := string(ics)
	for _, want := range []string{"METHOD:PUBLISH", "DTSTART:20261016T160000Z", "DTEND:20261016T180000Z", "UID:timeful-abc@time.example.com"} {
		if !strings.Contains(text, want) {
			t.Errorf("ics is missing %q:\n%s", want, text)
		}
	}

	cal, err := ical.NewDecoder(strings.NewReader(text)).Decode()
	if err != nil {
		t.Fatalf("generated ics doesn't parse: %v", err)
	}
	events := cal.Events()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	summary, _ := events[0].Props.Text(ical.PropSummary)
	if summary != "Team dinner, Friday; bring snacks" {
		t.Errorf("summary = %q (commas/semicolons must survive escaping)", summary)
	}
	description, _ := events[0].Props.Text(ical.PropDescription)
	if !strings.Contains(description, "Ann, Bob\n\nScheduled") {
		t.Errorf("description = %q", description)
	}
}

// fakeCaldavServer points the Apple endpoint at a local server for the test
func fakeCaldavServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := appleCaldavEndpoint
	appleCaldavEndpoint = server.URL
	t.Cleanup(func() { appleCaldavEndpoint = original })
}

func testAppleCalendar(t *testing.T) *AppleCalendar {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	encrypted, err := utils.Encrypt("app-password")
	if err != nil {
		t.Fatal(err)
	}
	return &AppleCalendar{AppleCalendarAuth: models.AppleCalendarAuth{Email: "me@icloud.com", Password: encrypted}}
}

func TestApplePutEvent(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotUser, gotPassword string
	fakeCaldavServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotUser, gotPassword, _ = r.BasicAuth()
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		// A weak ETag, which go-webdav's PutCalendarObject can't parse
		w.Header().Set("ETag", `W/"123"`)
		w.WriteHeader(http.StatusCreated)
	})

	objectId, err := testAppleCalendar(t).PutEvent("/123/calendars/home/", "timeful-abc.ics", testScheduledEventDetails())
	if err != nil {
		t.Fatal(err)
	}
	if objectId != "/123/calendars/home/timeful-abc.ics" {
		t.Errorf("objectId = %q", objectId)
	}
	if gotMethod != http.MethodPut || gotPath != "/123/calendars/home/timeful-abc.ics" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotUser != "me@icloud.com" || gotPassword != "app-password" {
		t.Errorf("basic auth = %q / %q", gotUser, gotPassword)
	}
	if !strings.Contains(gotBody, "BEGIN:VEVENT") || strings.Contains(gotBody, "METHOD:") {
		t.Errorf("body should be a VEVENT without METHOD (not allowed in CalDAV objects):\n%s", gotBody)
	}
}

func TestApplePutEvent_ErrorStatus(t *testing.T) {
	fakeCaldavServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := testAppleCalendar(t).PutEvent("/123/calendars/shared/", "timeful-abc.ics", testScheduledEventDetails()); err == nil {
		t.Error("expected an error for a 403 response")
	}
}

func TestAppleDeleteEvent(t *testing.T) {
	tests := []struct {
		status  int
		wantErr bool
	}{
		{http.StatusNoContent, false},
		{http.StatusNotFound, false}, // already deleted in Apple Calendar
		{http.StatusInternalServerError, true},
	}
	for _, tt := range tests {
		var gotMethod, gotPath string
		fakeCaldavServer(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			w.WriteHeader(tt.status)
		})
		err := testAppleCalendar(t).DeleteEvent("/123/calendars/home/timeful-abc.ics")
		if (err != nil) != tt.wantErr {
			t.Errorf("status %d: err = %v, wantErr %v", tt.status, err, tt.wantErr)
		}
		if gotMethod != http.MethodDelete || gotPath != "/123/calendars/home/timeful-abc.ics" {
			t.Errorf("request = %s %s", gotMethod, gotPath)
		}
	}
}

func TestGetCalendarEventWriter(t *testing.T) {
	apple := models.CalendarAccount{CalendarType: models.AppleCalendarType, AppleCalendarAuth: &models.AppleCalendarAuth{}}
	if GetCalendarEventWriter(apple) == nil {
		t.Error("Apple accounts should be writable")
	}
	for _, calendarType := range []models.CalendarType{models.GoogleCalendarType, models.OutlookCalendarType, models.ICSCalendarType} {
		if GetCalendarEventWriter(models.CalendarAccount{CalendarType: calendarType}) != nil {
			t.Errorf("%s accounts aren't writable yet", calendarType)
		}
	}
}
