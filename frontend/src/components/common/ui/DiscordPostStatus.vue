<script setup lang="ts">
/**
 * Where an item stands in Discord, for a list row: "Live in #channel - View", or "Deleted
 * in Discord" when someone removed the post there. Renders nothing for an item that was
 * never posted (or was taken down).
 */
import { computed, onMounted } from 'vue'
import { useDiscordStore } from '@/stores/discord'
import type { DiscordPostState } from '@/types/api'

const props = defineProps<{ post?: DiscordPostState | null }>()

const discord = useDiscordStore()
// The channel's name comes from the server's channel list.
onMounted(() => {
  if (props.post && !discord.directoryAvailable && !discord.loadingDirectory) {
    void discord.loadDirectory()
  }
})
const channel = computed(() =>
  props.post ? discord.channelLabel(props.post.channel_id) || 'Discord' : '',
)
</script>

<template>
  <span v-if="post?.state === 'live'" class="text-sm">
    <span class="badge badge--success">Live</span>
    in {{ channel }} -
    <a :href="post.url" target="_blank" rel="noopener">View</a>
  </span>
  <span
    v-else-if="post?.state === 'gone'"
    class="badge badge--warning"
    title="Someone deleted this post in Discord. Post again to put it back."
  >
    Deleted in Discord
  </span>
</template>
