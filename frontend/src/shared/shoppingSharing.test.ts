import { describe, expect, it, vi } from 'vitest';
import {
  createShareToken,
  isShareToken,
  readShareFragment,
  shareURL,
} from './shoppingSharing';
import { InputError } from './shoppingValidation';

describe('Секрет ссылки', () => {
  it('получает 32 криптографических байта и кодирует канонический base64url', () => {
    const random = vi
      .spyOn(crypto, 'getRandomValues')
      .mockImplementation((array) => {
        if (array instanceof Uint8Array) array.fill(255);
        return array;
      });
    const token = createShareToken();
    expect(random).toHaveBeenCalledOnce();
    expect(random.mock.calls[0][0]).toBeInstanceOf(Uint8Array);
    expect(random.mock.calls[0][0]?.byteLength).toBe(32);
    expect(token).toBe('_'.repeat(42) + '8');
    expect(isShareToken(token)).toBe(true);
    const url = new URL(shareURL(token));
    expect(url.origin).toBe(window.location.origin);
    expect(url.pathname).toBe('/share');
    expect(url.search).toBe('');
    expect(readShareFragment(url.hash)).toBe(token);
  });

  it('отказывает без безопасного генератора, не используя Math.random', () => {
    const insecure = vi.spyOn(Math, 'random');
    vi.stubGlobal('crypto', undefined);
    expect(createShareToken).toThrow(InputError);
    expect(insecure).not.toHaveBeenCalled();
  });

  it.each([
    '',
    'A'.repeat(42),
    'A'.repeat(44),
    'A'.repeat(42) + 'B',
    '+' + 'A'.repeat(42),
    '/' + 'A'.repeat(42),
    'A'.repeat(42) + '\n',
    'Я' + 'A'.repeat(42),
  ])('отвергает некорректный token без раскрытия значения', (token) => {
    expect(isShareToken(token)).toBe(false);
    expect(readShareFragment('#token=' + token)).toBeNull();
  });

  it.each([
    '',
    '?token=',
    '#other=',
    '#token=%41',
    '#token=',
    '#token=TOKEN&token=TOKEN',
    '#token=TOKEN&extra=1',
  ])(
    'не принимает секрет из иных мест и неоднозначные fragments',
    (fragment) => {
      expect(
        readShareFragment(fragment.replaceAll('TOKEN', 'A'.repeat(43))),
      ).toBeNull();
    },
  );
});
