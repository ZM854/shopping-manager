import { InputError } from './shoppingValidation';

const tokenPattern = /^[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/;

export function isShareToken(value: string): boolean {
  return value.length === 43 && tokenPattern.test(value);
}

export function readShareFragment(hash: string): string | null {
  const token = hash.startsWith('#token=') ? hash.slice(7) : '';
  return isShareToken(token) ? token : null;
}

export function createShareToken(): string {
  if (typeof globalThis.crypto?.getRandomValues !== 'function') {
    throw new InputError('Браузер не поддерживает безопасное создание ссылки.');
  }
  const bytes = crypto.getRandomValues(new Uint8Array(32));
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

export function shareURL(token: string): string {
  return new URL(`/share#token=${token}`, window.location.origin).href;
}
