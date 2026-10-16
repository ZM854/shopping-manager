import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useShoppingResource } from './useShoppingResource';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('Загрузка и мутации списков', () => {
  it('смена load сбрасывает блокировку старой мутации', async () => {
    const loadA = vi.fn(async () => ['А']);
    const loadB = vi.fn(async () => ['Б']);
    const { result, rerender } = renderHook(
      ({ load }) => useShoppingResource(load),
      { initialProps: { load: loadA } },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    const change = deferred<string>();
    let mutation!: Promise<unknown>;
    act(() => {
      mutation = result.current.mutate(
        () => change.promise,
        (data, value) => [...data, value],
      );
    });
    rerender({ load: loadB });
    await waitFor(() => expect(result.current.data).toEqual(['Б']));
    expect(result.current.pending).toBe(false);
    const rejection = expect(mutation).rejects.toThrow('закрыта');
    await act(async () => change.resolve('А-позднее'));
    await rejection;
    await act(async () =>
      result.current.mutate(
        async () => 'Б-новое',
        (data, value) => [...data, value],
      ),
    );
    expect(result.current.data).toEqual(['Б', 'Б-новое']);
  });
  it('отменяет старое чтение и игнорирует ответ даже без поддержки abort', async () => {
    const first = deferred<string>();
    const loadA = vi.fn(() => first.promise);
    const loadB = vi.fn(async () => 'Б');
    const { result, rerender } = renderHook(
      ({ load }) => useShoppingResource(load),
      { initialProps: { load: loadA } },
    );
    rerender({ load: loadB });
    await waitFor(() => expect(result.current.data).toBe('Б'));
    await act(async () => first.resolve('А'));
    expect(result.current.data).toBe('Б');
  });

  it('показывает ошибку чтения и позволяет повтор', async () => {
    const load = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce([]);
    const { result } = renderHook(() => useShoppingResource(load));
    await waitFor(() => expect(result.current.error).toBe('offline'));
    expect(result.current.data).toBeNull();
    await act(async () => result.current.reload());
    expect(result.current.data).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it('не смешивает позднюю мутацию с новым чтением и не допускает дубль', async () => {
    const load = vi.fn(async () => ['старое']);
    const { result } = renderHook(() => useShoppingResource(load));
    await waitFor(() => expect(result.current.loading).toBe(false));
    const change = deferred<string>();
    let mutation!: Promise<unknown>;
    act(() => {
      mutation = result.current.mutate(
        () => change.promise,
        (data, value) => [...data, value],
      );
    });
    await expect(
      result.current.mutate(
        async () => 'дубль',
        (data) => data,
      ),
    ).rejects.toThrow('недоступно');
    await act(async () => result.current.reload());
    const rejection = expect(mutation).rejects.toThrow('закрыта');
    await act(async () => change.resolve('позднее'));
    await rejection;
    expect(result.current.data).toEqual(['старое']);
    expect(result.current.pending).toBe(false);
  });

  it('пробрасывает ошибки мутаций, сохраняет данные и игнорирует завершение после unmount', async () => {
    const load = vi.fn(async () => ['А']);
    const { result, unmount } = renderHook(() => useShoppingResource(load));
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await expect(
        result.current.mutate(
          async () => {
            throw new Error('Ошибка записи');
          },
          (data) => data,
        ),
      ).rejects.toThrow('Ошибка записи');
    });
    expect(result.current.error).toBe('Ошибка записи');
    expect(result.current.data).toEqual(['А']);
    const change = deferred<string>();
    let mutation!: Promise<unknown>;
    act(() => {
      mutation = result.current.mutate(
        () => change.promise,
        (data, value) => [...data, value],
      );
    });
    const rejection = expect(mutation).rejects.toThrow('закрыта');
    unmount();
    change.resolve('Б');
    await rejection;
  });
});
