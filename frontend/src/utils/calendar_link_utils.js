import { serverURL } from "@/constants"

/** Returns the link to a Timeful event on this instance */
export const getEventUrl = (event) =>
  `${window.location.origin}/e/${event.shortId ?? event._id}`

/** Returns the url to download the event's scheduled time as an .ics file */
export const getScheduledEventIcsUrl = (event) =>
  `${serverURL}/events/${event._id}/scheduled.ics`

/**
 * Returns a Google Calendar url that opens a prefilled "create event" page.
 * `emails` are added as guests.
 */
export const getGoogleCalendarEventUrl = ({
  name,
  startDate,
  endDate,
  details,
  timezone,
  emails = [],
}) => {
  // Format start and end date to be in the format required by gcal (remove -, :, and .000)
  const start = startDate.toISOString().replace(/([-:]|\.000)/g, "")
  const end = endDate.toISOString().replace(/([-:]|\.000)/g, "")

  let url = `https://calendar.google.com/calendar/render?action=TEMPLATE&text=${encodeURIComponent(
    name
  )}&dates=${start}/${end}&details=${encodeURIComponent(details)}`
  if (timezone) url += `&ctz=${timezone}`
  if (emails.length > 0) url += `&add=${encodeURIComponent(emails.join(","))}`
  return url
}

/** Returns an Outlook url that opens a prefilled "create event" page */
export const getOutlookCalendarEventUrl = ({
  name,
  startDate,
  endDate,
  details,
  location = "",
  timezone,
}) => {
  let url = `https://outlook.live.com/calendar/0/deeplink/compose?subject=${encodeURIComponent(
    name
  )}&body=${encodeURIComponent(
    details
  )}&startdt=${startDate.toISOString()}&enddt=${endDate.toISOString()}&location=${encodeURIComponent(
    location
  )}&path=/calendar/action/compose`
  if (timezone) url += `&timezone=${timezone}`
  return url
}

/**
 * Returns the name of the calendar that the user's scheduled events are
 * automatically added to, or null if none is set
 */
export const getScheduleCalendarName = (user) => {
  const scheduleCalendar = user?.scheduleCalendar
  if (!scheduleCalendar) return null
  const account = user.calendarAccounts?.[scheduleCalendar.calendarAccountKey]
  return account?.subCalendars?.[scheduleCalendar.calendarId]?.name ?? null
}
