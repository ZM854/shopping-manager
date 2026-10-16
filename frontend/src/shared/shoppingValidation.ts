export class InputError extends Error {}

export function normalizeQuantity(value: string): string | null {
  if (value === '') return null;
  if (!/^(0|[1-9][0-9]{0,8})(\.[0-9]{1,3})?$/.test(value) || /\s/.test(value)) {
    throw new InputError(
      'Количество: от 0.001 до 999999999.999, до 3 знаков после точки',
    );
  }
  const normalized = value.includes('.')
    ? value.replace(/0+$/, '').replace(/\.$/, '')
    : value;
  if (normalized === '0')
    throw new InputError('Количество должно быть больше нуля');
  return normalized;
}

export function normalizeText(
  value: string,
  max: number,
  required = true,
): string {
  const text = value.trim();
  if ((required && !text) || [...text].length > max || /\p{Cc}/u.test(text)) {
    throw new InputError(
      `Проверьте текст: максимум ${max} символов, без управляющих символов`,
    );
  }
  return text;
}
