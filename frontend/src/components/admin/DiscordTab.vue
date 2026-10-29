<script setup lang="ts">
/**
 * System -> Discord (admin-only): the built-in bot's connection, its token, its
 * configuration, and a test message.
 *
 * The token is write-only here. It is sent once, checked with Discord, and stored
 * sealed on the server; the page only ever learns whether one is set and readable.
 * `status.problem` names the one thing to fix next, so the top of the page always
 * says what to do.
 */
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import FormField from '@/components/common/ui/FormField.vue'
import FormRow from '@/components/common/ui/FormRow.vue'
import FormActions from '@/components/common/ui/FormActions.vue'
import ChannelPicker from '@/components/common/ui/ChannelPicker.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useDiscordStore, type DiscordConfigForm } from '@/stores/discord'

const discord = useDiscordStore()
const st = computed(() => discord.status)

const tokenDraft = ref('')
const testChannel = ref('')
const form = reactive<DiscordConfigForm>({
  guild_id: '',
  application_id: '',
  status_text: '',
  public_base_url: '',
  test_mode: false,
  test_channel_id: '',
})

// The form mirrors the saved configuration whenever the server reports it.
watch(
  st,
  (s) => {
    if (!s) return
    form.guild_id = s.guild_id
    form.application_id = s.application_id
    form.status_text = s.status_text
    form.public_base_url = s.public_base_url
    form.test_mode = s.test_mode
    form.test_channel_id = s.test_channel_id
  },
  { immediate: true },
)

onMounted(() => {
  if (!discord.status) void discord.loadStatus()
})

const onlineSince = computed(() =>
  st.value?.connected_at ? new Date(st.value.connected_at).toLocaleString() : '',
)

async function saveToken(): Promise<void> {
  if (await discord.saveToken(tokenDraft.value)) tokenDraft.value = ''
}

function invite(): void {
  if (st.value?.invite_url) window.open(st.value.invite_url, '_blank', 'noopener')
}

function refresh(): void {
  void discord.loadStatus()
  void discord.loadDirectory()
  void discord.loadImport()
}

/** How each import outcome reads, and the badge it wears. */
const importStatus: Record<string, { label: string; badge: string }> = {
  imported: { label: 'Imported', badge: 'badge--success' },
  other_server: { label: 'Pick a channel', badge: 'badge--warning' },
  webhook_deleted: { label: 'Webhook gone', badge: 'badge--muted' },
  error: { label: 'Not checked yet', badge: 'badge--warning' },
  pending: { label: 'Waiting', badge: 'badge--muted' },
}

const canRetry = computed(() =>
  discord.importRows.some((r) => r.status === 'error' || r.status === 'pending'),
)
const anyImported = computed(() => discord.importRows.some((r) => r.status === 'imported'))

/** An imported channel's name when the bot can see it, else its id. */
function importChannel(id: string): string {
  if (!id) return ''
  return discord.channelLabel(id) || `channel ${id}`
}
</script>

<template>
  <AdminPanel title="Discord Bot" :icon="['fab', 'discord']">
    <LoadingSpinner v-if="!st" />
    <template v-else>
      <p v-if="st.problem" class="callout callout--warning">{{ st.problem }}</p>
      <p v-if="st.dry_run" class="callout callout--warning">
        This server is running in dry-run mode: nothing it does is sent to Discord.
      </p>
      <p v-else-if="st.redirect_channel_id" class="callout callout--warning">
        This server sends every new post to channel {{ st.redirect_channel_id }}, whatever the
        system chose.
      </p>

      <section class="mb-20">
        <h4 class="section-heading">Connection</h4>
        <ul class="stack mb-12">
          <li class="stack-row">
            <span class="stack-label">Bot</span>
            <img
              v-if="st.bot_avatar_url"
              :src="st.bot_avatar_url"
              alt=""
              width="28"
              height="28"
              class="avatar"
            />
            <span>{{ st.bot_name || 'Not connected' }}</span>
            <span class="badge" :class="st.online ? 'badge--success' : 'badge--muted'">
              {{ st.online ? 'Online' : 'Offline' }}
            </span>
          </li>
          <li class="stack-row">
            <span class="stack-label">Server</span>
            <img
              v-if="st.guild_icon_url"
              :src="st.guild_icon_url"
              alt=""
              width="28"
              height="28"
              class="avatar"
            />
            <span>{{ st.guild_name || st.guild_id }}</span>
            <span v-if="st.connected && !st.in_guild" class="badge badge--warning"
              >Not invited</span
            >
            <span v-else-if="st.in_guild && !st.bot_is_admin" class="badge badge--warning">
              No Administrator
            </span>
          </li>
          <li v-if="onlineSince" class="stack-row">
            <span class="stack-label">Online since</span>
            <span>{{ onlineSince }}</span>
          </li>
          <li v-if="st.last_error && !st.online" class="stack-row">
            <span class="stack-label">Last error</span>
            <span class="text-muted">{{ st.last_error }}</span>
          </li>
        </ul>
        <FormActions align="start">
          <button type="button" class="btn-view btn-sm" :disabled="!st.invite_url" @click="invite">
            <font-awesome-icon :icon="['fad', 'user-plus']" /> Invite to server
          </button>
          <button
            type="button"
            class="btn-neutral btn-sm"
            :disabled="discord.loading"
            @click="refresh"
          >
            <font-awesome-icon :icon="['fas', 'rotate']" /> Refresh
          </button>
        </FormActions>
      </section>

      <section class="mb-20">
        <h4 class="section-heading">Bot token</h4>
        <FormField
          label="Bot token"
          html-for="discord-token"
          help="Developer Portal, your application, Bot, Reset Token. It is checked with Discord and stored encrypted; this page never shows it again."
        >
          <input
            id="discord-token"
            v-model="tokenDraft"
            type="password"
            autocomplete="off"
            spellcheck="false"
            :disabled="!st.key_available"
            :placeholder="
              st.token_set
                ? 'A token is saved - paste a new one to replace it'
                : 'Paste the bot token'
            "
          />
        </FormField>
        <FormActions align="start">
          <button
            type="button"
            class="btn-confirm"
            :disabled="!tokenDraft.trim() || discord.savingToken"
            @click="saveToken"
          >
            {{ st.token_set ? 'Replace token' : 'Save token' }}
          </button>
          <button
            v-if="st.token_set"
            type="button"
            class="btn-danger"
            :disabled="discord.savingToken"
            @click="discord.clearToken()"
          >
            Remove token
          </button>
        </FormActions>
      </section>

      <section class="mb-20">
        <h4 class="section-heading">Settings</h4>
        <FormRow>
          <FormField label="Server ID" html-for="discord-guild">
            <input id="discord-guild" v-model="form.guild_id" inputmode="numeric" />
          </FormField>
          <FormField label="Application ID" html-for="discord-app">
            <input id="discord-app" v-model="form.application_id" inputmode="numeric" />
          </FormField>
        </FormRow>
        <FormField
          label="Status text"
          html-for="discord-status"
          help="Shown under the bot's name in the member list. Leave blank for none."
        >
          <input id="discord-status" v-model="form.status_text" maxlength="128" />
        </FormField>
        <FormField
          label="Public site URL"
          html-for="discord-base"
          help="Image links in posts are built from this, including posts the bot updates on its own."
        >
          <input
            id="discord-base"
            v-model="form.public_base_url"
            placeholder="https://apps.senpan.cafe"
          />
        </FormField>
        <FormField
          label="Test mode"
          help="While on, every new post goes to the test channel instead of its real one, and scheduled announcements wait. Turning it off sends them."
        >
          <button
            type="button"
            class="switch"
            role="switch"
            :class="{ 'is-on': form.test_mode }"
            :aria-checked="form.test_mode"
            aria-label="Test mode"
            @click="form.test_mode = !form.test_mode"
          >
            <span class="switch-knob"></span>
          </button>
        </FormField>
        <FormField label="Test channel" html-for="discord-test-channel">
          <ChannelPicker
            id="discord-test-channel"
            v-model="form.test_channel_id"
            allow-none
            placeholder="No test channel"
          />
        </FormField>
        <FormActions align="start">
          <button
            type="button"
            class="btn-confirm"
            :disabled="discord.savingConfig"
            @click="discord.saveConfig(form)"
          >
            Save settings
          </button>
        </FormActions>
      </section>

      <section v-if="discord.importRows.length" class="mb-20">
        <h4 class="section-heading">Webhook import</h4>
        <p class="text-muted text-sm mb-12">
          Where each old webhook posted, carried over to the bot. A system whose webhook posted into
          another server, or no longer exists, needs a channel picked on its own page.
        </p>
        <ul class="stack mb-12">
          <li v-for="row in discord.importRows" :key="row.source">
            <div class="stack-row">
              <strong>{{ row.label }}</strong>
              <span class="badge" :class="importStatus[row.status]?.badge ?? 'badge--muted'">
                {{ importStatus[row.status]?.label ?? row.status }}
              </span>
              <span v-if="row.status === 'imported'">{{ importChannel(row.channel_id) }}</span>
            </div>
            <small class="text-muted text-sm">
              {{
                [row.detail, row.status === 'imported' ? '' : `Set it at ${row.where}.`]
                  .filter(Boolean)
                  .join(' ')
              }}
            </small>
          </li>
        </ul>
        <FormActions align="start">
          <button
            v-if="canRetry"
            type="button"
            class="btn-view btn-sm"
            :disabled="discord.importBusy"
            @click="discord.retryImport()"
          >
            <font-awesome-icon :icon="['fas', 'rotate']" /> Try again
          </button>
          <button
            type="button"
            class="btn-danger btn-sm"
            :disabled="discord.importBusy"
            @click="discord.removeOldWebhooks()"
          >
            {{ anyImported ? 'Remove old webhooks' : 'Clear report' }}
          </button>
        </FormActions>
      </section>

      <section>
        <h4 class="section-heading">Test message</h4>
        <FormField label="Channel" html-for="discord-test-target">
          <ChannelPicker id="discord-test-target" v-model="testChannel" />
        </FormField>
        <FormActions align="start">
          <button
            type="button"
            class="btn-action"
            :disabled="!testChannel || !st.connected || discord.testing"
            @click="discord.sendTest(testChannel)"
          >
            <font-awesome-icon :icon="['fas', 'paper-plane']" /> Send test message
          </button>
        </FormActions>
      </section>
    </template>
  </AdminPanel>
</template>
