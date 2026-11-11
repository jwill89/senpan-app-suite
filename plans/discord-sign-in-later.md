# Plan: Discord sign-in later

The parts of shared-tooling's `plans/sign-in-on-huma.md` (2026-11-09) that concern Senpan, moved here on 2026-11-10 when shared-tooling became project-agnostic.

It doesn't argue for keeping sign-in outside huma. What pulled OAuth outside an API framework was that it answers a browser with redirects, and the pattern above does that inside huma, with the session in the operation's context either way. What it argues for is the redirect pattern settled once in the standard before anyone else builds it, a callback path that never moves, since Discord registers it, and an account model where a way to sign in is a row, not a column.

## The account model

- An `identities` table: `id`, `user_id` (cascading), `provider` (`discord`), `subject` (Discord's user id), `name` (shown on the account page), `created_at`, `last_used_at`, and `UNIQUE (provider, subject)`. Linking adds a row, unlinking deletes one, and signing in looks up `(provider, subject)`.
- Linking happens only from a signed-in session (the account page's Link Discord, through the same start and callback with a note in the session that it's a link), never by matching a Discord name to a username: anyone can choose a Discord name.
- The session names the way it signed in, as Home Library's `sessionPasskey` does for passkeys (`auth.go:28`), so unlinking signs out the devices that came in by it (security.md line 36).
- Senpan has no Discord application today: it posts through webhooks, and its bot is to be redone after the overhaul (decided 2026-11-09). Sign-in can share that bot's application once it exists; it needs the application's client secret, which a bot alone doesn't, in the service's environment file, and the callback registered.

## The standard's rules that bite (security.md)

- Line 50: removing the last way to sign in is refused with 409. Every Senpan account has a password today (`password_hash NOT NULL`), so it can't bite until an account can lack one; then the check sits in `delete-passkey` and the unlink.
- Line 101: `next` is checked on the server and kept in the session across Discord's round trip.
- Lines 54 and 55: the `identify` scope only, so there is no email to link by; claims read strictly.
- Line 46: an account made through Discord starts with no rights. In Senpan it starts inactive (`is_active` defaults to 0), so the callback redirects with `?error=not_activated` where `sign-in` answers 403.
- Line 32: signing in moves the session to a new id, in the one sign-in function.
- Line 53: adding a passkey asks for the password when the account has one (Senpan's `begin-passkey` does), so an account made through Discord adds one without.
- Line 117: Turnstile can't ride a navigation. Discord's own consent stands in for it on the start, and the per-address limit still applies.

## What Senpan's phase 2 should avoid

1. A provider's id as a column on `users`. Discord's id belongs in an `identities` row.
2. A session started anywhere but `startSession`, which is already "every way of signing in" (`signin.go:172`). The callback will call it too.
3. A session that records only a passkey. When it records how it signed in, it records a kind and an id, so an identity fits beside a passkey.
4. The SPA's 401 handler and the rate limits matched to a list of today's operations. Matched on the `/auth/` prefix, a Discord start and callback are covered when they come.
5. Adding a passkey with no password check (line 53). Phase 2 has it: `begin-passkey` takes the account's password and answers 422 for a wrong one. Every Senpan account has a password today; once one can lack it (an account made through Discord), the check is skipped for that account.
