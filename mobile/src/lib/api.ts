// Talking to the API. One place, so headers and failures are not spread across
// every screen.

import * as Crypto from 'expo-crypto';
import * as SecureStore from 'expo-secure-store';
import Constants from 'expo-constants';

export class ApiError extends Error {
  status: number;
  code: string;
  retryAfter: number;

  constructor(status: number, payload: any, retryAfter: string | null) {
    super(payload?.error?.message ?? `request failed with ${status}`);

    this.status = status;
    this.code = payload?.error?.code ?? 'unknown';
    this.retryAfter = Number(retryAfter) || 0;
  }
}

// Where the server is.
//
// Taken from whichever machine is serving the bundle rather than written down,
// because a phone cannot reach localhost and a hard-coded address goes stale the
// next time the router hands out a different one — which it already has once.
// EXPO_PUBLIC_API_URL overrides it for a deployment that is not the dev machine.
function baseUrl(): string {
  const configured = process.env.EXPO_PUBLIC_API_URL;
  if (configured) return configured.replace(/\/$/, '');

  const host = Constants.expoConfig?.hostUri?.split(':')[0];
  if (host) return `http://${host}:8099`;

  // A production build with nothing configured has nowhere to go, and saying so
  // beats failing on every request with a confusing URL.
  throw new Error('No API address: set EXPO_PUBLIC_API_URL');
}

const TOKEN_KEY = 'seathold.token';

let token: string | null = null;

// The token lives in the device keychain rather than in memory alone. On the web
// this is kept in memory on purpose, because anything that reaches the page can
// read web storage; SecureStore is backed by the Keychain and the Keystore, so
// the trade-off is not the same one and signing in on every launch would be a
// cost without a benefit.
export async function loadToken(): Promise<string | null> {
  if (token) return token;

  try {
    token = await SecureStore.getItemAsync(TOKEN_KEY);
  } catch {
    // A device that will not give it back is a device that is signed out.
    token = null;
  }

  return token;
}

export async function setToken(value: string | null): Promise<void> {
  token = value;

  try {
    if (value === null) await SecureStore.deleteItemAsync(TOKEN_KEY);
    else await SecureStore.setItemAsync(TOKEN_KEY, value);
  } catch {
    // Keeping it in memory is still a usable session; losing the write only
    // means signing in again next launch.
  }
}

export function hasToken(): boolean {
  return token !== null;
}

// newIdempotencyKey returns a fresh key for a write that must not be applied
// twice. React Native has no global crypto, so this comes from expo-crypto
// rather than crypto.randomUUID, which is also unavailable on the web outside a
// secure context.
export function newIdempotencyKey(): string {
  return Crypto.randomUUID();
}

type Options = {
  body?: unknown;
  idempotencyKey?: string;
};

export async function api<T>(method: string, path: string, { body, idempotencyKey }: Options = {}): Promise<T> {
  const headers: Record<string, string> = {};

  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey;

  const response = await fetch(`${baseUrl()}${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  // 204 has no body to read, and neither does a failure that sent none.
  const payload = response.status === 204 ? null : await response.json().catch(() => null);

  if (!response.ok) {
    throw new ApiError(response.status, payload, response.headers.get('Retry-After'));
  }

  return payload as T;
}

// The address an image or a QR code is fetched from. Those go through <Image>
// rather than this client, so they need the base on its own.
export function assetUrl(path: string): string {
  return `${baseUrl()}${path}`;
}

export function authHeader(): Record<string, string> {
  return token ? { Authorization: `Bearer ${token}` } : {};
}
