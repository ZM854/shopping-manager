import { describe, expect, it } from 'vitest';
import { normalizeQuantity, normalizeText } from './shoppingValidation';

describe('Валидация формы покупок', () => {
  it('сохраняет decimal без числового преобразования', () => {
    expect(normalizeQuantity('')).toBeNull();
    for (const [value, expected] of [
      ['0.001', '0.001'],
      ['1.000', '1'],
      ['0.100', '0.1'],
      ['999999999.999', '999999999.999'],
      ['999999999', '999999999'],
    ]) {
      expect(normalizeQuantity(value)).toBe(expected);
    }
    for (const value of [
      '0',
      '0.000',
      '-1',
      '+1',
      '1.0001',
      '1e3',
      '1,5',
      '01',
      '.5',
      'NaN',
      'Infinity',
      '1000000000',
      '1\n',
    ]) {
      expect(() => normalizeQuantity(value)).toThrow();
    }
  });
  it('считает Unicode-кодовые точки и разрешает пустую единицу', () => {
    expect(normalizeText(' 🥛 ', 1)).toBe('🥛');
    expect(normalizeText('  ', 32, false)).toBe('');
    for (const value of [' ', 'a\nb', 'a\u0085b', 'ab'])
      expect(() => normalizeText(value, 1)).toThrow();
  });
});
