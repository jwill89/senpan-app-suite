<script setup lang="ts">
/**
 * Public Stamp Rally list: the rallies currently open to self-service sign-up.
 *
 * Only rallies an admin has opted in (and that are open + inside their availability
 * window) appear here - the server decides that, this view just renders what it is
 * given. Picking one opens its sign-up page, which is directly linkable so staff can
 * post a rally's sign-up URL straight into Discord.
 */
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { assetUrl } from '@/lib/assets'
import type { SignupRally } from '@/types/api'

const router = useRouter()
const store = useStampRalliesStore()

onMounted(() => store.loadSignupRallies())

function openRally(r: SignupRally): void {
  void router.push({ name: 'stamp-rally-signup', params: { id: String(r.id) } })
}

function goHome(): void {
  void router.push({ name: 'home' })
}

function goLookup(): void {
  void router.push({ name: 'stamp-lookup' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="goHome">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Home
      </button>
      <h2><font-awesome-icon :icon="['fad', 'stamp']" /> Stamp Rallies</h2>
      <span></span>
    </div>

    <div class="tab-body content-container">
      <LoadingSpinner v-if="store.signupLoading" block label="Loading stamp rallies..." />

      <template v-else-if="store.signupRallies.length">
        <div class="card-grid card-grid--center">
          <div
            v-for="r in store.signupRallies"
            :key="r.id"
            class="media-card"
            role="button"
            tabindex="0"
            @click="openRally(r)"
            @keydown.enter="openRally(r)"
            @keydown.space.prevent="openRally(r)"
          >
            <img
              v-if="r.card_image"
              :src="assetUrl(r.card_image)"
              class="media-card-image"
              alt="Stamp card"
            />
            <div class="media-card-body">
              <h3>{{ r.title }}</h3>
              <p v-if="r.garapon_title" class="text-sm text-muted">
                <font-awesome-icon :icon="['fad', 'circle-dot']" /> Includes a
                {{ r.garapon_title }} garapon draw
              </p>
            </div>
          </div>
        </div>
      </template>

      <EmptyState
        v-else
        :icon="['fad', 'stamp']"
        text="No stamp rallies are open for sign-up."
        hint="Check back soon - rallies appear here when sign-ups go live."
      />

      <!-- Always offered, even with nothing open: someone hunting for a lost link
           is far more likely to arrive here than to have kept the lookup URL. -->
      <p class="stamp-signup-lookup-line">
        Already signed up and lost your link?
        <button class="link-btn" @click="goLookup">Find my links</button>
      </p>
    </div>
  </div>
</template>
