import { describe, expect, it } from 'vitest';
import {
  emailError,
  nameError,
  normalizeEmail,
  passwordError,
  trimAuth,
} from './authValidation';

describe('Валидация auth', () => {
  it('нормализует Unicode-пробелы имени/email и проверяет длину по кодовым точкам', () => {
    expect(trimAuth('\u0085 Имя \u00a0')).toBe('Имя');
    expect(normalizeEmail('\u0085 USER@Example.Test \u00a0')).toBe(
      'user@example.test',
    );
    expect(nameError('😀'.repeat(20))).toBeUndefined();
    for (const name of [' ', 'А', '😀'.repeat(21), 'Им\nя'])
      expect(nameError(name)).toBeDefined();
  });
  it('проверяет адрес и ограничение 254 символов', () => {
    expect(emailError(' USER@Example.Test ')).toBeUndefined();
    for (const email of [
      'bad',
      'user@localhost',
      'пользователь@example.test',
      'a'.repeat(65) + '@example.test',
      'user@' + 'a'.repeat(245) + '.test',
    ])
      expect(emailError(email)).toBeDefined();
  });
  it('считает пароль в UTF-8 байтах и не навязывает новый минимум при входе', () => {
    expect(passwordError('short', false)).toBeUndefined();
    expect(passwordError('short', true)).toBe('Минимум 6 символов');
    expect(passwordError('я'.repeat(36), true)).toBeUndefined();
    expect(passwordError('я'.repeat(37), false)).toBeDefined();
    expect(passwordError('😀'.repeat(18), true)).toBeUndefined();
    expect(passwordError('😀'.repeat(19), true)).toBeDefined();
  });
});
