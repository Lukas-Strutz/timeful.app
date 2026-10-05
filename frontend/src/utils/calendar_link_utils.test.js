import { afterEach, describe, expect, it, vi } from "vitest"
import {
  getEventUrl,
  getGoogleCalendarEventUrl,
  getOutlookCalendarEventUrl,
  getScheduleCalendarName,
  getScheduledEventIcsUrl,
} from "./calendar_link_utils"

const startDate = new Date("2026-10-16T16:00:00.000Z")
const endDate = new Date("2026-10-16T18:00:00.000Z")

describe("calendar links", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it("links to the event on the current origin, preferring the short id", () => {
    vi.stubGlobal("window", {
      location: { origin: "https://time.example.com" },
    })
    expect(getEventUrl({ _id: "abc", shortId: "xyz" })).toBe(
      "https://time.example.com/e/xyz"
    )
    expect(getEventUrl({ _id: "abc" })).toBe("https://time.example.com/e/abc")
  })

  it("builds the .ics download url from the event id", () => {
    expect(getScheduledEventIcsUrl({ _id: "abc", shortId: "xyz" })).toBe(
      "/api/events/abc/scheduled.ics"
    )
  })

  it("builds a Google Calendar template url", () => {
    const url = new URL(
      getGoogleCalendarEventUrl({
        name: "Team dinner & drinks",
        startDate,
        endDate,
        details: "Scheduled with Timeful",
        timezone: "Europe/Berlin",
        emails: ["a@example.com", "b@example.com"],
      })
    )
    expect(url.origin + url.pathname).toBe(
      "https://calendar.google.com/calendar/render"
    )
    expect(url.searchParams.get("text")).toBe("Team dinner & drinks")
    expect(url.searchParams.get("dates")).toBe(
      "20261016T160000Z/20261016T180000Z"
    )
    expect(url.searchParams.get("details")).toBe("Scheduled with Timeful")
    expect(url.searchParams.get("ctz")).toBe("Europe/Berlin")
    expect(url.searchParams.get("add")).toBe("a@example.com,b@example.com")
  })

  it("leaves out guests and time zone when not given", () => {
    const url = new URL(
      getGoogleCalendarEventUrl({ name: "x", startDate, endDate, details: "" })
    )
    expect(url.searchParams.has("add")).toBe(false)
    expect(url.searchParams.has("ctz")).toBe(false)
  })

  it("builds an Outlook compose url", () => {
    const url = new URL(
      getOutlookCalendarEventUrl({
        name: "Team dinner",
        startDate,
        endDate,
        details: "Scheduled with Timeful",
        location: "Berlin",
      })
    )
    expect(url.searchParams.get("subject")).toBe("Team dinner")
    expect(url.searchParams.get("body")).toBe("Scheduled with Timeful")
    expect(url.searchParams.get("startdt")).toBe(startDate.toISOString())
    expect(url.searchParams.get("enddt")).toBe(endDate.toISOString())
    expect(url.searchParams.get("location")).toBe("Berlin")
  })
})

describe("getScheduleCalendarName", () => {
  const user = {
    scheduleCalendar: {
      calendarAccountKey: "me@icloud.com_apple",
      calendarId: "/123/calendars/home/",
    },
    calendarAccounts: {
      "me@icloud.com_apple": {
        subCalendars: { "/123/calendars/home/": { name: "Home" } },
      },
    },
  }

  it("returns the name of the schedule calendar", () => {
    expect(getScheduleCalendarName(user)).toBe("Home")
  })

  it("returns null when no calendar is set or it no longer exists", () => {
    expect(getScheduleCalendarName(null)).toBe(null)
    expect(getScheduleCalendarName({ ...user, scheduleCalendar: null })).toBe(
      null
    )
    expect(getScheduleCalendarName({ ...user, calendarAccounts: {} })).toBe(
      null
    )
  })
})
