/**
 * Personal Card Requests store: the public flow where a user builds a custom bingo
 * card, picks a 6-char ID + their character/world, and submits it for staff
 * approval. Client-side validation mirrors the backend (structural card validity +
 * ID format); the ID-taken / duplicate-board checks are server-side and surface as
 * error toasts on submit.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { emptyNumberBoard, validateBoard } from '@/lib/constants'
import { useUiStore } from './ui'

const CARD_ID_RE = /^[A-Za-z0-9]{6}$/

export const useCardRequestsStore = defineStore('cardRequests', () => {
  const ui = useUiStore()

  const characterName = ref('')
  const world = ref('')
  const cardId = ref('')
  const board = ref<number[][]>(emptyNumberBoard())
  const turnstileToken = ref('')
  const submitting = ref(false)
  /** The accepted request (id + 'pending') after a successful submit, else null. */
  const result = ref<{ id: string; status: string } | null>(null)

  /**
   * Returns the first client-side problem with the form (or '' when it's ready to
   * submit). Mirrors the backend's structural checks; does NOT cover ID-taken or
   * duplicate-board, which only the server can determine.
   */
  function validate(): string {
    if (!characterName.value.trim() || !world.value.trim()) {
      return 'Enter your character name and world.'
    }
    if (!CARD_ID_RE.test(cardId.value.trim())) {
      return 'Card ID must be exactly 6 letters or numbers.'
    }
    return validateBoard(board.value)
  }

  async function submit(): Promise<void> {
    const err = validate()
    if (err) {
      ui.notify(err, 'error')
      return
    }
    submitting.value = true
    try {
      const data = await endpoints.cards.request({
        character_name: characterName.value.trim(),
        world: world.value.trim(),
        card_id: cardId.value.trim().toUpperCase(),
        board_data: board.value,
        turnstile_token: turnstileToken.value || undefined,
      })
      result.value = { id: data.id, status: data.status }
      ui.notify(`Card ${data.id} submitted for staff approval!`, 'success')
    } catch (e) {
      // The server rejects a taken ID / duplicate board with an actionable message.
      ui.notify((e as Error).message, 'error')
    } finally {
      // Turnstile tokens are single-use — clear so the widget re-issues one.
      turnstileToken.value = ''
      submitting.value = false
    }
  }

  /** Clears the form back to a blank card (used by "request another"). */
  function reset(): void {
    characterName.value = ''
    world.value = ''
    cardId.value = ''
    board.value = emptyNumberBoard()
    turnstileToken.value = ''
    result.value = null
  }

  return {
    characterName,
    world,
    cardId,
    board,
    turnstileToken,
    submitting,
    result,
    validate,
    submit,
    reset,
  }
})
