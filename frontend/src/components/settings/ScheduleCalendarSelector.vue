<template>
  <div class="tw-flex tw-flex-col tw-gap-3">
    <div class="tw-text-black">
      When you schedule an event, it's automatically added to this calendar,
      along with who responded. Rescheduling moves it, and unscheduling or
      deleting the event removes it again.
    </div>
    <div v-if="calendarOptions.length > 0" class="tw-max-w-md">
      <v-select
        :value="selectedValue"
        :items="items"
        :loading="saving"
        :disabled="saving"
        solo
        hide-details
        @change="save"
      />
    </div>
    <div v-else class="tw-text-sm tw-text-dark-gray">
      Add an Apple Calendar account above to choose a calendar.
    </div>
  </div>
</template>

<script>
import { mapActions, mapState } from "vuex"
import { calendarTypes } from "@/constants"
import { patch } from "@/utils"

// Calendar types that scheduled events can be written to
const writableCalendarTypes = [calendarTypes.APPLE]

export default {
  name: "ScheduleCalendarSelector",

  data() {
    return {
      saving: false,
    }
  },

  computed: {
    ...mapState(["authUser"]),
    /** All calendars that scheduled events can be added to */
    calendarOptions() {
      const options = []
      for (const [calendarAccountKey, account] of Object.entries(
        this.authUser?.calendarAccounts ?? {}
      )) {
        if (!writableCalendarTypes.includes(account.calendarType)) continue
        for (const [calendarId, subCalendar] of Object.entries(
          account.subCalendars ?? {}
        )) {
          options.push({
            text: `${subCalendar.name} (${account.email})`,
            value: this.toValue(calendarAccountKey, calendarId),
          })
        }
      }
      return options.sort((a, b) => a.text.localeCompare(b.text))
    },
    items() {
      return [
        { text: "Don't add scheduled events to a calendar", value: "" },
        ...this.calendarOptions,
      ]
    },
    selectedValue() {
      const scheduleCalendar = this.authUser?.scheduleCalendar
      if (!scheduleCalendar) return ""
      return this.toValue(
        scheduleCalendar.calendarAccountKey,
        scheduleCalendar.calendarId
      )
    },
  },

  methods: {
    ...mapActions(["showInfo", "showError", "refreshAuthUser"]),
    toValue(calendarAccountKey, calendarId) {
      return JSON.stringify([calendarAccountKey, calendarId])
    },
    async save(value) {
      const [calendarAccountKey, calendarId] = value
        ? JSON.parse(value)
        : ["", ""]
      this.saving = true
      try {
        await patch("/user/schedule-calendar", {
          calendarAccountKey,
          calendarId,
        })
        await this.refreshAuthUser()
        this.showInfo(
          calendarAccountKey
            ? "Scheduled events will be added to this calendar"
            : "Scheduled events won't be added to a calendar"
        )
      } catch (err) {
        console.error(err)
        this.showError(
          "There was a problem saving your calendar choice! Please try again later."
        )
      } finally {
        this.saving = false
      }
    },
  },
}
</script>
