/**
 * Passkey (WebAuthn) client.
 *
 * Registration and login each have a server "begin" step (returns the credential
 * options and stashes a one-time challenge in the session) and a "finish" step
 * (verifies the browser's response). The browser's credential API sits between
 * them. We use the standard JSON helpers (`parseCreationOptionsFromJSON` /
 * `parseRequestOptionsFromJSON` / `credential.toJSON()`) so there's no hand-rolled
 * base64url ⇄ ArrayBuffer conversion — at the cost of requiring a reasonably
 * modern browser (feature-detected via `passkeysSupported`).
 */
import { apiDelete, apiGet, apiPost } from './api'
import type { LoginResponse, PasskeysResponse } from '@/types/api'

/** Whether this browser supports the passkey ceremonies we use. */
export function passkeysSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.PublicKeyCredential === 'function' &&
    typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === 'function'
  )
}

/** Registers a new passkey for the logged-in account under `name`. */
export async function registerPasskey(name: string): Promise<PasskeysResponse> {
  const options = await apiPost<{ publicKey: PublicKeyCredentialCreationOptionsJSON }>(
    'account/passkeys/register/begin',
    {},
  )
  const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(options.publicKey)
  const credential = (await navigator.credentials.create({
    publicKey,
  })) as PublicKeyCredential | null
  if (!credential) throw new Error('Passkey creation was cancelled.')
  return await apiPost<PasskeysResponse>(
    `account/passkeys/register/finish?name=${encodeURIComponent(name)}`,
    credential.toJSON(),
  )
}

/**
 * Logs in via a discoverable passkey (usernameless). On success the server
 * establishes the session and returns the account, as a password login does.
 */
export async function loginWithPasskey(): Promise<LoginResponse> {
  const options = await apiPost<{ publicKey: PublicKeyCredentialRequestOptionsJSON }>(
    'auth/passkey/begin',
    {},
    // The user isn't authenticated yet — a failed assertion legitimately 401s and
    // must not trigger the global "session expired" redirect.
    { skipAuthRedirect: true },
  )
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(options.publicKey)
  const assertion = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null
  if (!assertion) throw new Error('Passkey login was cancelled.')
  return await apiPost<LoginResponse>('auth/passkey/finish', assertion.toJSON(), {
    skipAuthRedirect: true,
  })
}

/** Lists the logged-in account's passkeys (metadata only). */
export function listPasskeys(): Promise<PasskeysResponse> {
  return apiGet<PasskeysResponse>('account/passkeys')
}

/** Deletes one of the logged-in account's passkeys by id. */
export function deletePasskey(id: number): Promise<PasskeysResponse> {
  return apiDelete<PasskeysResponse>(`account/passkeys/${id}`)
}
