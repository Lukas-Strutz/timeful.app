<template>
  <div
    class="tw-flex tw-min-h-screen tw-flex-col tw-items-center tw-bg-light-gray tw-px-4"
  >
    <div
      class="tw-flex tw-w-full tw-max-w-md tw-flex-1 tw-flex-col tw-items-center tw-justify-center tw-text-center"
    >
      <Logo type="timeful" class="tw-mb-8" />

      <h1
        class="tw-mb-3 tw-text-3xl tw-font-medium tw-text-very-dark-gray sm:tw-text-4xl"
      >
        Find a time to meet
      </h1>
      <p class="tw-mb-10 tw-text-base tw-text-dark-gray sm:tw-text-lg">
        Collect everyone's availability and see where it overlaps.
      </p>

      <div class="tw-flex tw-flex-col tw-items-center tw-gap-3">
        <v-btn
          class="tw-rounded-lg tw-bg-green tw-px-10 tw-text-base"
          dark
          large
          @click="authUser ? openDashboard() : (newDialog = true)"
        >
          {{ authUser ? "Open dashboard" : "Create event" }}
        </v-btn>
        <v-btn v-if="!authUser" text :to="{ name: 'sign-in' }">Sign in</v-btn>
      </div>
    </div>

    <router-link
      class="tw-py-6 tw-text-xs tw-font-medium tw-text-gray"
      :to="{ name: 'privacy-policy' }"
    >
      Privacy Policy
    </router-link>

    <NewDialog v-model="newDialog" no-tabs @signIn="signIn" />
  </div>
</template>

<script>
import { mapState } from "vuex"
import Logo from "@/components/Logo.vue"
import NewDialog from "@/components/NewDialog.vue"

export default {
  name: "Landing",

  metaInfo: {
    title: "Timeful - Find a time to meet",
  },

  components: {
    Logo,
    NewDialog,
  },

  data: () => ({
    newDialog: false,
  }),

  computed: {
    ...mapState(["authUser"]),
  },

  methods: {
    signIn() {
      this.$router.push({ name: "sign-in" })
    },
    openDashboard() {
      this.$router.push({ name: "home" })
    },
  },
}
</script>
