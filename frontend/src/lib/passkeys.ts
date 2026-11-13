/**
 * Passkey (WebAuthn) client.
 *
 * Adding a passkey and signing in with one each take two steps on the server: a
 * "begin" that answers the browser's options (and keeps a one-time challenge in the
 * session, for ten minutes at most) and a finish that verifies what the browser
 * made or offered. The browser's credential API sits between them. The standard JSON
 * helpers (`parseCreationOptionsFromJSON` / `parseRequestOptionsFromJSON` /
 * `credential.toJSON()`) do the base64url <-> ArrayBuffer work, at the cost of
 * needing a reasonably modern browser (feature-detected by `passkeysSupported`).
 */
import { api, unwrap, type Account, type Passkey } from '@/api/client'

/** Whether this browser supports the passkey ceremonies we use. */
export function passkeysSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.PublicKeyCredential === 'function' &&
    typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function' &&
    typeof PublicKeyCredential.parseRequestOptionsFromJSON === 'function'
  )
}

/**
 * Adds a passkey to the signed-in account under `name`, and answers it. Adding a
 * way to sign in asks for the account's password; an account with none (made
 * with Discord) passes none.
 */
export async function addPasskey(name: string, password?: string): Promise<Passkey> {
  const options = await unwrap(
    api.POST('/me/passkeys/begin', { body: password ? { password } : {} }),
  )
  const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(
    options.publicKey as PublicKeyCredentialCreationOptionsJSON,
  )
  const credential = (await navigator.credentials.create({
    publicKey,
  })) as PublicKeyCredential | null
  if (!credential) {
    throw new Error('Passkey creation was cancelled.')
  }
  return await unwrap(api.POST('/me/passkeys', { body: { name, credential: credential.toJSON() } }))
}

/**
 * Signs in with a discoverable passkey (no username): the browser offers one of its
 * own. Answers the signed-in account, as a password sign-in does.
 */
export async function signInWithPasskey(): Promise<Account> {
  const options = await unwrap(api.POST('/auth/passkey/begin'))
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(
    options.publicKey as PublicKeyCredentialRequestOptionsJSON,
  )
  const assertion = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null
  if (!assertion) {
    throw new Error('Passkey sign-in was cancelled.')
  }
  const me = await unwrap(
    api.POST('/auth/passkey/finish', { body: { credential: assertion.toJSON() } }),
  )
  if (!me.account) {
    throw new Error('That passkey did not sign you in.')
  }
  return me.account
}

/** The signed-in account's passkeys (metadata only). */
export async function listPasskeys(): Promise<Passkey[]> {
  return (await unwrap(api.GET('/me/passkeys'))).passkeys
}

/** Removes one of the signed-in account's passkeys. */
export async function deletePasskey(id: number): Promise<void> {
  await unwrap(api.DELETE('/me/passkeys/{id}', { params: { path: { id } } }))
}
