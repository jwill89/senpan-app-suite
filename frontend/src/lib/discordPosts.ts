/**
 * Shared helpers for items the bot keeps a Discord post for (tea rooms, affiliates).
 */
import type { DiscordDestination, DiscordPostState, RepostMode } from '@/types/api'

/** One choice in a destination form's "when posting again" picker. */
export interface RepostOption {
  value: RepostMode
  label: string
  /** One sentence saying what happens, shown under the picker. */
  help: string
}

/** The choices for an item kept in Discord (a tea room, an affiliate). */
export const ITEM_REPOST_OPTIONS: RepostOption[] = [
  {
    value: 'edit',
    label: 'Keep one post',
    help: 'Posting again updates the same message, and later changes to the item update it on their own.',
  },
  {
    value: 'replace',
    label: 'Replace it',
    help: 'Posting again puts up a new message, then deletes the old one.',
  },
  {
    value: 'new',
    label: 'Add another',
    help: 'Posting again adds a new message and leaves the earlier ones up.',
  },
]

/** The choices for a reading list, which is several messages and can't be edited in place. */
export const LIST_REPOST_OPTIONS: RepostOption[] = [
  {
    value: 'replace',
    label: 'Replace the previous list',
    help: 'Publishing deletes the previous reading list once the new one is up.',
  },
  {
    value: 'new',
    label: 'Keep earlier lists',
    help: 'Publishing adds the new list and leaves the earlier ones up.',
  },
]

/**
 * What posting an item will do: "Update" edits its live post in place (the destination
 * edits in place, and the item has a post up), "Post" makes a new one.
 */
export function postVerb(
  dest: DiscordDestination | null | undefined,
  post: DiscordPostState | null | undefined,
): 'Post' | 'Update' {
  return dest?.repost_mode === 'edit' && post?.state === 'live' ? 'Update' : 'Post'
}

/** Replace one item's post state in a list, by id. */
export function setPostState(
  list: { id: number; discord_post?: DiscordPostState }[],
  id: number,
  post: DiscordPostState | null | undefined,
): void {
  const item = list.find((x) => x.id === id)
  if (item) item.discord_post = post ?? undefined
}
