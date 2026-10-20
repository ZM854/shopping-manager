export const trimAuth = (value: string) =>
  value.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '');
export const normalizeEmail = (value: string) =>
  trimAuth(value).replace(/[A-Z]/g, (char) => char.toLowerCase());

export function nameError(value: string): string | undefined {
  const name = trimAuth(value);
  if (/\p{Cc}/u.test(name)) return 'Управляющие символы в имени запрещены';
  if ([...name].length < 2) return 'Минимум 2 символа';
  if ([...name].length > 20) return 'Максимум 20 символов';
}

export function emailError(value: string): string | undefined {
  const email = normalizeEmail(value);
  if (email.length > 254) return 'Максимум 254 символа';
  if (
    !/^[\x21-\x7e]+$/.test(email) ||
    !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) ||
    email.split('@')[0].length > 64
  )
    return 'Некорректный email';
}

export function passwordError(
  value: string,
  registration: boolean,
): string | undefined {
  if (new TextEncoder().encode(value).length > 72)
    return 'Пароль должен занимать не более 72 байт UTF-8';
  if (registration && [...value].length < 6) return 'Минимум 6 символов';
}
