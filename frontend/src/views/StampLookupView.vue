<script setup lang="ts">
/**
 * Public "find my links" lookup: a participant who lost their card link enters the
 * name they signed up with and gets it back.
 *
 * The match is the whole name, case-insensitively - the server does no prefix or
 * substring search, so this cannot be used to fish for other people's links. A miss
 * and an unknown name are indistinguishable by design, so the empty state says the
 * name matched nothing rather than claiming the person doesn't exist.
 */
import { onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import WorldPicker from '@/components/common/ui/WorldPicker.vue'
import { participantLabel } from '@/lib/participant'

const router = useRouter()
const store = useStampRalliesStore()

const name = ref('')
const world = ref('')

// Results are a transient answer to a question just asked; leaving the page must
// not leave someone else's links on screen when it is re-entered.
onUnmounted(() => store.resetLookup())

function submit(): void {
  void store.lookupLinks(name.value, world.value)
}

function back(): void {
  void router.push({ name: 'stamp-rallies' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="back">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Back
      </button>
      <h2><font-awesome-icon :icon="['fad', 'magnifying-glass']" /> Find My Links</h2>
      <span></span>
    </div>

    <div class="tab-body stamp-lookup-body">
      <p class="text-muted mb-16">
        Enter the character name and world you signed up with and we'll hand your stamp card back.
        It has to be the <strong>exact</strong> name you used - spelling and spacing included -
        though capitalization doesn't matter.
      </p>

      <form class="stamp-lookup-form" @submit.prevent="submit">
        <input
          v-model="name"
          placeholder="Firstname Lastname"
          maxlength="60"
          autocomplete="off"
          aria-label="The name you signed up with"
          :disabled="store.lookupLoading"
        />
        <!-- Picked, not typed: the lookup matches the world exactly, so a typo
             here would report that a real sign-up does not exist. -->
        <WorldPicker
          v-model="world"
          label=""
          placeholder="World..."
          :disabled="store.lookupLoading"
        />
        <button
          class="btn-confirm"
          type="submit"
          :disabled="store.lookupLoading || !name.trim() || !world"
        >
          <LoadingSpinner v-if="store.lookupLoading" label="Searching..." />
          <template v-else
            ><font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Search</template
          >
        </button>
      </form>

      <!-- Results (null = nothing searched yet, so nothing is claimed either way) -->
      <template v-if="store.lookupResults">
        <div v-if="store.lookupResults.length" class="stamp-lookup-results">
          <div v-for="entry in store.lookupResults" :key="entry.rally_id" class="card card--padded">
            <h3 class="mb-8">
              {{ entry.rally_title }}
              <span v-if="entry.completed" class="badge badge--success">Complete</span>
            </h3>
            <!-- Which record this is. A card issued before worlds had their own
                 field, or one staff typed by hand, may be spelled differently from
                 what was just searched for. -->
            <p class="text-muted text-sm mb-8">
              Held by
              <strong>{{ participantLabel(entry.participant_name, entry.world) }}</strong>
            </p>

            <div class="stamp-signup-link">
              <span class="field-label">Stamp card</span>
              <a :href="store.stampCardUrl(entry.card_token)" class="stamp-signup-link-url">
                {{ store.stampCardUrl(entry.card_token) }}
              </a>
              <button
                class="btn-view btn-sm"
                @click="store.copyLink(store.stampCardUrl(entry.card_token))"
              >
                <font-awesome-icon :icon="['fas', 'copy']" /> Copy
              </button>
            </div>

            <!--
              Deliberately states the draws rather than linking them. A draw cannot
              be undone and the link is the whole capability, so a lookup keyed on a
              character name - which anyone can read off an entrant list - must not
              hand it out. Saying the draws are still there is what a participant on
              a borrowed device actually needs to know.
            -->
            <p v-if="entry.garapon_title" class="stamp-lookup-draws">
              <font-awesome-icon :icon="['fas', 'ticket']" />
              <span>
                <strong>{{ entry.garapon_title }}</strong> &mdash;
                {{ entry.garapon_draws_left || 0 }}
                {{ entry.garapon_draws_left === 1 ? 'draw' : 'draws' }} left.
              </span>
              <span class="text-muted">
                Your drawing link was saved on the device you signed up with. Ask a staff member if
                you need it again.
              </span>
            </p>
          </div>
        </div>

        <div v-else class="form-alert form-alert-warning" role="status">
          <font-awesome-icon :icon="['fas', 'triangle-exclamation']" class="form-alert-icon" />
          <span>
            No open stamp rally has a sign-up under that name. Check the spelling and spacing
            against what you entered, or ask a staff member if you're stuck.
          </span>
        </div>
      </template>
    </div>
  </div>
</template>
