/**
 * Signing in with Discord.
 *
 * The sign-in is a page the browser goes to, not a call: the start
 * (`/api/v1/auth/discord/start`) sends it to Discord, and Discord sends it back
 * to the server, which signs it in and sends it on. When a sign-in doesn't
 * finish, the server sends the browser to the sign-in page with `?error=` and
 * one of the reasons below, since a navigation can't carry a problem body.
 */

/** The start's address; `next` is a page on this site to land on once signed in. */
export function discordStartUrl(next?: string): string {
  const start = '/api/v1/auth/discord/start'
  return next ? `${start}?next=${encodeURIComponent(next)}` : start
}

/** What the sign-in page says for each reason a Discord sign-in didn't finish. */
const DISCORD_ERRORS: Record<string, string> = {
  discord_denied: 'Discord sign-in was cancelled.',
  discord_failed: "Signing in with Discord didn't work. Try again.",
  discord_busy: 'Too many tries from here. Try again in a few minutes.',
  discord_unavailable: "Discord sign-in isn't set up on this server.",
  discord_not_linked:
    'No account is linked to that Discord account. Ask an admin for a link to sign up or to link your account.',
  discord_inactive:
    "Your Discord account is linked, but an admin hasn't activated this account yet.",
  discord_link_expired: 'That link has expired or was already used. Ask an admin for a new one.',
  discord_taken:
    'That Discord account is already linked to another account. Sign in with Discord instead, or ask an admin.',
}

/** The message for a sign-in page's `?error=` value, or '' when it isn't one of ours. */
export function discordSignInError(code: unknown): string {
  return typeof code === 'string' && Object.hasOwn(DISCORD_ERRORS, code)
    ? (DISCORD_ERRORS[code] ?? '')
    : ''
}
