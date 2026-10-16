import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useSubmission } from './useSubmission';
import { ApiError } from '../services/api';
import { InputError } from '../shared/shoppingValidation';

describe('Отправка формы', () => {
  it('сохраняет ключ после сетевой ошибки и выдаёт новый для следующего действия', async () => {
    const { result } = renderHook(useSubmission);
    const failed = vi.fn(async () => {
      throw new Error('offline');
    });
    await act(async () => result.current.submit(failed));
    expect(result.current.retry).toBe(true);
    const succeed = vi.fn(async () => {});
    await act(async () => result.current.submit(succeed));
    expect(failed.mock.calls[0]).toEqual(succeed.mock.calls[0]);
    await act(async () => result.current.submit(succeed));
    expect(succeed.mock.calls[0]).not.toEqual(succeed.mock.calls[1]);
    expect(result.current.retry).toBe(false);
  });

  it('серверная и локальная валидация разрешают исправить поля', async () => {
    const { result } = renderHook(useSubmission);
    await act(async () =>
      result.current.submit(async () => {
        throw new ApiError(400, 'Проверьте поля', 'validation_failed', {
          name: 'Имя слишком длинное',
        });
      }),
    );
    expect(result.current.fieldErrors).toEqual({ name: 'Имя слишком длинное' });
    expect(result.current.retry).toBe(false);
    await act(async () =>
      result.current.submit(async () => {
        throw new InputError('Проверьте количество');
      }),
    );
    expect(result.current.error).toBe('Проверьте количество');
    expect(result.current.fieldErrors).toEqual({});
    expect(result.current.retry).toBe(false);
  });

  it('двойной вызов не отправляет второй запрос; поздняя ошибка после unmount обработана', async () => {
    let reject!: (error: Error) => void;
    const promise = new Promise<void>((_, no) => {
      reject = no;
    });
    const action = vi.fn(() => promise);
    const { result, unmount } = renderHook(useSubmission);
    let submitting!: Promise<void>;
    act(() => {
      submitting = result.current.submit(action);
    });
    await act(async () => result.current.submit(action));
    expect(action).toHaveBeenCalledOnce();
    expect(result.current.pending).toBe(true);
    unmount();
    reject(new Error('offline'));
    await submitting;
  });
});
