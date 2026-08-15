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

const router = useRouter()
const store = useStampRalliesStore()

const name = ref('')

// Results are a transient answer to a question just asked; leaving the page must
// not leave someone else's links on screen when it is re-entered.
onUnmounted(() => store.resetLookup())

function submit(): void {
  void store.lookupLinks(name.value)
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
        Enter the character name you signed up with and we'll hand your links back. It has to be the
        <strong>exact</strong> name you used - spelling and spacing included - though capitalization
        doesn't matter.
      </p>

      <form class="stamp-lookup-form" @submit.prevent="submit">
        <input
          v-model="name"
          placeholder="Firstname Lastname @ World"
          maxlength="60"
          autocomplete="off"
          aria-label="The name you signed up with"
          :disabled="store.lookupLoading"
        />
        <button class="btn-confirm" type="submit" :disabled="store.lookupLoading || !name.trim()">
          <LoadingSpinner v-if="store.lookupLoading" label="Searching..." />
          <template v-else
            ><font-awesome-icon :icon="['fas', 'magnifying-glass']" /> Search</template
          >
        </button>
      </form>

      <!-- Results (null = nothing searched yet, so nothing is claimed either way) -->
      <template v-if="store.lookupResults">
        <div v-if="store.lookupResults.length" class="stamp-lookup-results">
          <div v-for="entry in store.lookupResults" :key="entry.rally_id" class="card">
            <h3 class="mb-8">
              {{ entry.rally_title }}
              <span v-if="entry.completed" class="badge badge--success">Complete</span>
            </h3>

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

            <div v-if="entry.garapon_token" class="stamp-signup-link">
              <span class="field-label">{{ entry.garapon_title }} draw</span>
              <a :href="store.garaponUrl(entry.garapon_token)" class="stamp-signup-link-url">
                {{ store.garaponUrl(entry.garapon_token) }}
              </a>
              <button
                class="btn-view btn-sm"
                @click="store.copyLink(store.garaponUrl(entry.garapon_token))"
              >
                <font-awesome-icon :icon="['fas', 'copy']" /> Copy
              </button>
            </div>
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
