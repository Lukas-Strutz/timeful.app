<template>
  <div
    v-if="event.scheduledEvent"
    class="tw-mt-2 tw-flex tw-max-w-full tw-flex-col tw-gap-2 tw-rounded-md tw-border tw-border-green tw-bg-light-gray tw-p-2 tw-text-xs tw-text-very-dark-gray sm:tw-max-w-[calc(100%-236px)] sm:tw-flex-row sm:tw-items-center sm:tw-text-sm"
  >
    <div class="tw-flex tw-grow tw-items-center tw-gap-2">
      <v-icon class="tw-text-green" small>mdi-calendar-check</v-icon>
      <div>
        <span class="tw-font-medium">Scheduled:</span>
        {{ formattedTime }}
      </div>
    </div>
    <div class="tw-flex tw-flex-wrap tw-items-center tw-gap-1">
      <v-btn
        small
        outlined
        class="tw-text-very-dark-gray"
        :href="icsUrl"
        download
      >
        <v-img
          class="tw-mr-1 tw-flex-none"
          width="12"
          height="12"
          src="@/assets/apple_logo.svg"
        />
        Apple
      </v-btn>
      <v-btn
        small
        outlined
        class="tw-text-very-dark-gray"
        :href="googleUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        Google
      </v-btn>
      <v-btn
        small
        outlined
        class="tw-text-very-dark-gray"
        :href="outlookUrl"
        target="_blank"
        rel="noopener noreferrer"
      >
        Outlook
      </v-btn>
      <v-btn
        v-if="canEdit"
        small
        text
        class="tw-text-red"
        :loading="unscheduling"
        @click="unschedule"
      >
        Unschedule
      </v-btn>
    </div>
  </div>
</template>

<script>
import { mapActions } from "vuex"
import {
  _delete,
  getEventUrl,
  getScheduledEventIcsUrl,
  getGoogleCalendarEventUrl,
  getOutlookCalendarEventUrl,
  userPrefers12h,
} from "@/utils"
import { timeTypes } from "@/constants"

export default {
  name: "ScheduledEventBanner",

  props: {
    event: { type: Object, required: true },
    // Whether the current user can change the event's schedule
    canEdit: { type: Boolean, default: false },
  },

  data() {
    return {
      unscheduling: false,
    }
  },

  computed: {
    startDate() {
      return new Date(this.event.scheduledEvent.startDate)
    },
    endDate() {
      return new Date(this.event.scheduledEvent.endDate)
    },
    /** e.g. "Fri, Oct 16, 6:00 – 8:00 PM", in the viewer's time zone */
    formattedTime() {
      const prefers12h = localStorage["timeType"]
        ? localStorage["timeType"] === timeTypes.HOUR12
        : userPrefers12h()
      const format = new Intl.DateTimeFormat(undefined, {
        weekday: "short",
        month: "short",
        day: "numeric",
        hour: "numeric",
        minute: "2-digit",
        hour12: prefers12h,
      })
      return format.formatRange(this.startDate, this.endDate)
    },
    details() {
      return `\n\nThis event was scheduled with Timeful: ${getEventUrl(
        this.event
      )}`
    },
    icsUrl() {
      return getScheduledEventIcsUrl(this.event)
    },
    googleUrl() {
      return getGoogleCalendarEventUrl({
        name: this.event.name,
        startDate: this.startDate,
        endDate: this.endDate,
        details: this.details,
      })
    },
    outlookUrl() {
      return getOutlookCalendarEventUrl({
        name: this.event.name,
        startDate: this.startDate,
        endDate: this.endDate,
        details: this.details,
      })
    },
  },

  methods: {
    ...mapActions(["showInfo", "showError"]),
    async unschedule() {
      if (!confirm("Remove the scheduled time from this event?")) return

      this.unscheduling = true
      try {
        const { calendarWrite } = await _delete(
          `/events/${this.event._id}/schedule`
        )
        this.$emit("refreshEvent")
        if (calendarWrite?.status === "failed") {
          this.showError(
            `The schedule was removed, but the event couldn't be removed from "${calendarWrite.calendarName}". Please delete it there.`
          )
        } else {
          this.showInfo("The event is no longer scheduled")
        }
      } catch (err) {
        console.error(err)
        this.showError("There was a problem removing the schedule!")
      } finally {
        this.unscheduling = false
      }
    },
  },
}
</script>
