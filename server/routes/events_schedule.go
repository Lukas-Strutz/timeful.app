package routes

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"schej.it/server/db"
	"schej.it/server/errs"
	"schej.it/server/logger"
	"schej.it/server/models"
	"schej.it/server/responses"
	"schej.it/server/services/calendar"
	"schej.it/server/utils"
)

// Result of adding a scheduled event to the owner's calendar
type calendarWriteResult struct {
	// "written", "failed", or "none" (no calendar configured)
	Status       string `json:"status"`
	CalendarName string `json:"calendarName,omitempty"`
	Error        string `json:"error,omitempty"`
}

const (
	calendarWriteWritten = "written"
	calendarWriteFailed  = "failed"
	calendarWriteNone    = "none"
)

// @Summary Schedules an event for the given time
// @Description Saves the scheduled time so every respondent can see it, and adds it to the owner's schedule calendar if one is set
// @Tags events
// @Accept json
// @Produce json
// @Param eventId path string true "Event ID"
// @Param payload body object{startDate=string,endDate=string} true "Start and end of the scheduled time"
// @Success 200 {object} object{scheduledEvent=models.CalendarEvent,calendarWrite=object}
// @Router /events/{eventId}/schedule [post]
func scheduleEvent(c *gin.Context) {
	payload := struct {
		StartDate time.Time `json:"startDate" binding:"required"`
		EndDate   time.Time `json:"endDate" binding:"required"`
	}{}
	if err := c.BindJSON(&payload); err != nil {
		return
	}
	if !payload.EndDate.After(payload.StartDate) {
		c.Status(http.StatusBadRequest)
		return
	}

	event, owner, ok := getEventForScheduling(c)
	if !ok {
		return
	}

	event.ScheduledEvent = &models.CalendarEvent{
		Summary:   event.Name,
		StartDate: primitive.NewDateTimeFromTime(payload.StartDate),
		EndDate:   primitive.NewDateTimeFromTime(payload.EndDate),
	}
	calendarWrite := writeScheduledEventToCalendar(event, owner)

	update := bson.M{"$set": bson.M{"scheduledEvent": event.ScheduledEvent}}
	if event.ScheduledEventCalendarRef != nil {
		update["$set"].(bson.M)["scheduledEventCalendarRef"] = event.ScheduledEventCalendarRef
	} else {
		update["$unset"] = bson.M{"scheduledEventCalendarRef": ""}
	}
	if _, err := db.EventsCollection.UpdateByID(context.Background(), event.Id, update); err != nil {
		logger.StdErr.Panicln(err)
	}

	c.JSON(http.StatusOK, gin.H{
		"scheduledEvent": event.ScheduledEvent,
		"calendarWrite":  calendarWrite,
	})
}

// @Summary Clears the scheduled time of an event
// @Description Also removes the event from the owner's calendar if it was added there
// @Tags events
// @Produce json
// @Param eventId path string true "Event ID"
// @Success 200 {object} object{calendarWrite=object}
// @Router /events/{eventId}/schedule [delete]
func unscheduleEvent(c *gin.Context) {
	event, owner, ok := getEventForScheduling(c)
	if !ok {
		return
	}

	calendarWrite := calendarWriteResult{Status: calendarWriteNone}
	if event.ScheduledEventCalendarRef != nil {
		calendarWrite = deleteScheduledEventFromCalendar(owner, event.ScheduledEventCalendarRef)
	}

	_, err := db.EventsCollection.UpdateByID(context.Background(), event.Id, bson.M{
		"$unset": bson.M{"scheduledEvent": "", "scheduledEventCalendarRef": ""},
	})
	if err != nil {
		logger.StdErr.Panicln(err)
	}

	c.JSON(http.StatusOK, gin.H{"calendarWrite": calendarWrite})
}

// @Summary Downloads the scheduled event as an .ics file
// @Tags events
// @Produce text/calendar
// @Param eventId path string true "Event ID"
// @Success 200
// @Router /events/{eventId}/scheduled.ics [get]
func getScheduledEventIcs(c *gin.Context) {
	event := db.GetEventByEitherId(c.Param("eventId"))
	if event == nil || utils.Coalesce(event.IsDeleted) {
		c.JSON(http.StatusNotFound, responses.Error{Error: errs.EventNotFound})
		return
	}
	if event.ScheduledEvent == nil {
		c.JSON(http.StatusNotFound, responses.Error{Error: errs.EventNotScheduled})
		return
	}

	// Anyone with the link can download this, so don't list respondents
	ics, err := calendar.EncodeScheduledEventIcs(scheduledEventDetails(event, false))
	if err != nil {
		logger.StdErr.Panicln(err)
	}

	c.Header("Content-Disposition", icsContentDisposition(event.Name))
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", ics)
}

// getEventForScheduling loads the event from the eventId param and checks
// that the current user may schedule it, with the same rules as editing:
// events with an owner can only be scheduled by the owner, guest events by
// anyone. Returns the owner (nil for guest events). If ok is false, an error
// response has been sent.
func getEventForScheduling(c *gin.Context) (event *models.Event, owner *models.User, ok bool) {
	event = db.GetEventByEitherId(c.Param("eventId"))
	if event == nil || utils.Coalesce(event.IsDeleted) {
		c.JSON(http.StatusNotFound, responses.Error{Error: errs.EventNotFound})
		return nil, nil, false
	}

	if event.OwnerId == primitive.NilObjectID {
		return event, nil, true
	}

	userId, signedIn := sessions.Default(c).Get("userId").(string)
	if !signedIn || utils.StringToObjectID(userId) != event.OwnerId {
		c.JSON(http.StatusForbidden, responses.Error{Error: errs.UserNotEventOwner})
		return nil, nil, false
	}

	owner = db.GetUserById(userId)
	if owner == nil {
		c.JSON(http.StatusUnauthorized, responses.Error{Error: errs.UserDoesNotExist})
		return nil, nil, false
	}

	return event, owner, true
}

// writeScheduledEventToCalendar adds event.ScheduledEvent to the owner's
// schedule calendar, or updates the copy it wrote before, and updates
// event.ScheduledEventCalendarRef accordingly. If the schedule calendar
// changed since the last write, the old copy is removed.
func writeScheduledEventToCalendar(event *models.Event, owner *models.User) calendarWriteResult {
	if owner == nil {
		return calendarWriteResult{Status: calendarWriteNone}
	}

	ref := event.ScheduledEventCalendarRef
	var accountKey, calendarId string
	if target := owner.ScheduleCalendar; target != nil && isValidScheduleCalendar(owner, target.CalendarAccountKey, target.CalendarId) {
		accountKey, calendarId = target.CalendarAccountKey, target.CalendarId
	} else if ref != nil && isValidScheduleCalendar(owner, ref.CalendarAccountKey, ref.CalendarId) {
		// No (valid) schedule calendar anymore: keep the existing copy in sync
		accountKey, calendarId = ref.CalendarAccountKey, ref.CalendarId
	} else {
		event.ScheduledEventCalendarRef = nil
		return calendarWriteResult{Status: calendarWriteNone}
	}

	account := owner.CalendarAccounts[accountKey]
	calendarName := (*account.SubCalendars)[calendarId].Name

	// Moved to a different calendar: remove the old copy first
	if ref != nil && (ref.CalendarAccountKey != accountKey || ref.CalendarId != calendarId) {
		deleteScheduledEventFromCalendar(owner, ref)
		event.ScheduledEventCalendarRef = nil
	}

	writer := calendar.GetCalendarEventWriter(account)
	objectId, err := writer.PutEvent(calendarId, scheduledEventFileName(event), scheduledEventDetails(event, true))
	if err != nil {
		logger.StdErr.Printf("Failed to add scheduled event %s to calendar: %v\n", event.Id.Hex(), err)
		return calendarWriteResult{Status: calendarWriteFailed, CalendarName: calendarName, Error: err.Error()}
	}

	event.ScheduledEventCalendarRef = &models.ScheduledEventCalendarRef{
		CalendarAccountKey: accountKey,
		CalendarId:         calendarId,
		ObjectId:           objectId,
	}
	return calendarWriteResult{Status: calendarWriteWritten, CalendarName: calendarName}
}

// deleteScheduledEventFromCalendar removes a previously written scheduled
// event from the owner's calendar
func deleteScheduledEventFromCalendar(owner *models.User, ref *models.ScheduledEventCalendarRef) calendarWriteResult {
	if owner == nil {
		return calendarWriteResult{Status: calendarWriteNone}
	}
	account, ok := owner.CalendarAccounts[ref.CalendarAccountKey]
	if !ok {
		// The calendar account was disconnected; nothing we can do
		return calendarWriteResult{Status: calendarWriteNone}
	}
	writer := calendar.GetCalendarEventWriter(account)
	if writer == nil {
		return calendarWriteResult{Status: calendarWriteNone}
	}

	calendarName := ""
	if account.SubCalendars != nil {
		calendarName = (*account.SubCalendars)[ref.CalendarId].Name
	}
	if err := writer.DeleteEvent(ref.ObjectId); err != nil {
		logger.StdErr.Printf("Failed to remove scheduled event %s from calendar: %v\n", ref.ObjectId, err)
		return calendarWriteResult{Status: calendarWriteFailed, CalendarName: calendarName, Error: err.Error()}
	}
	return calendarWriteResult{Status: calendarWriteWritten, CalendarName: calendarName}
}

// isValidScheduleCalendar returns whether the user has the given calendar and
// its provider supports adding events
func isValidScheduleCalendar(user *models.User, calendarAccountKey string, calendarId string) bool {
	account, ok := user.CalendarAccounts[calendarAccountKey]
	if !ok || account.SubCalendars == nil || calendar.GetCalendarEventWriter(account) == nil {
		return false
	}
	_, ok = (*account.SubCalendars)[calendarId]
	return ok
}

// scheduledEventDetails builds the calendar entry for the event's scheduled
// time. Respondent names are only included for the owner's own calendar.
func scheduledEventDetails(event *models.Event, includeRespondents bool) calendar.ScheduledEventDetails {
	eventUrl := fmt.Sprintf("%s/e/%s", utils.GetBaseUrl(), event.GetId())

	var description []string
	if event.Description != nil && strings.TrimSpace(*event.Description) != "" {
		description = append(description, strings.TrimSpace(*event.Description))
	}
	if includeRespondents {
		if names := getRespondentNames(event); len(names) > 0 {
			description = append(description, fmt.Sprintf("Responded (%d): %s", len(names), strings.Join(names, ", ")))
		}
	}
	description = append(description, "Scheduled with Timeful: "+eventUrl)

	return calendar.ScheduledEventDetails{
		UID:         scheduledEventUID(event),
		Summary:     event.Name,
		Description: strings.Join(description, "\n\n"),
		URL:         eventUrl,
		Start:       event.ScheduledEvent.StartDate.Time(),
		End:         event.ScheduledEvent.EndDate.Time(),
	}
}

// getRespondentNames returns the display names of everyone who responded
func getRespondentNames(event *models.Event) []string {
	names := make([]string, 0)
	for _, eventResponse := range db.GetEventResponses(event.Id.Hex()) {
		name := ""
		if eventResponse.Response != nil {
			name = strings.TrimSpace(eventResponse.Response.Name)
		}
		if name == "" && eventResponse.UserId != "" {
			if user := db.GetUserById(eventResponse.UserId); user != nil {
				name = strings.TrimSpace(user.FirstName + " " + user.LastName)
			}
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// scheduledEventUID is stable per event, so calendars treat a rescheduled or
// re-imported event as the same one
func scheduledEventUID(event *models.Event) string {
	host := "timeful.app"
	if baseUrl, err := url.Parse(utils.GetBaseUrl()); err == nil && baseUrl.Hostname() != "" {
		host = baseUrl.Hostname()
	}
	return fmt.Sprintf("timeful-%s@%s", event.Id.Hex(), host)
}

func scheduledEventFileName(event *models.Event) string {
	return fmt.Sprintf("timeful-%s.ics", event.Id.Hex())
}

var icsFileNameUnsafeChars = regexp.MustCompile(`[^\p{L}\p{N} ._-]+`)
var nonAsciiChars = regexp.MustCompile(`[^\x20-\x7E]+`)

// icsContentDisposition returns the Content-Disposition header for downloading
// the event as "<event name>.ics", with an ASCII fallback for old browsers
func icsContentDisposition(eventName string) string {
	name := []rune(strings.TrimSpace(icsFileNameUnsafeChars.ReplaceAllString(eventName, "")))
	if len(name) > 80 {
		name = name[:80]
	}
	fileName := strings.TrimSpace(string(name))
	if fileName == "" {
		fileName = "event"
	}

	asciiFileName := strings.TrimSpace(nonAsciiChars.ReplaceAllString(fileName, ""))
	if asciiFileName == "" {
		asciiFileName = "event"
	}

	return fmt.Sprintf(`attachment; filename="%s.ics"; filename*=UTF-8''%s.ics`, asciiFileName, url.PathEscape(fileName))
}
