import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { shoppingService as service } from './shoppingService';
vi.mock('./api', () => ({ apiFetch: vi.fn() }));
const request = vi.mocked(apiFetch);
const list = '9007199254740993';
const item = '9223372036854775807';
const base = `/shopping-lists/${list}`;
const key = 'repeat-action-0001';
const create = { name: 'Молоко', quantity: '999999999.999', unit: 'л' };
beforeEach(() => request.mockReset());
describe('API нескольких списков', () => {
  it.each([
    {
      run: () => service.getLists(),
      path: '/shopping-lists',
      options: { signal: undefined },
    },
    {
      run: () => service.getList(list),
      path: base,
      options: { signal: undefined },
    },
    {
      run: () => service.createList({ name: 'Покупки' }, key),
      path: '/shopping-lists',
      options: {
        method: 'POST',
        headers: { 'Idempotency-Key': key },
        body: '{"name":"Покупки"}',
      },
    },
    {
      run: () => service.updateList(list, { name: 'Новое' }),
      path: base,
      options: { method: 'PATCH', body: '{"name":"Новое"}' },
    },
    {
      run: () => service.deleteList(list),
      path: base,
      options: { method: 'DELETE' },
    },
    {
      run: () => service.getItems(list),
      path: `${base}/items`,
      options: { signal: undefined },
    },
    {
      run: () => service.getItem(list, item),
      path: `${base}/items/${item}`,
      options: { signal: undefined },
    },
    {
      run: () => service.createItem(list, create, key),
      path: `${base}/items`,
      options: {
        method: 'POST',
        headers: { 'Idempotency-Key': key },
        body: JSON.stringify(create),
      },
    },
    {
      run: () => service.updateItem(list, item, { isMarked: false }),
      path: `${base}/items/${item}`,
      options: { method: 'PATCH', body: '{"isMarked":false}' },
    },
    {
      run: () => service.updateItem(list, item, { quantity: null }),
      path: `${base}/items/${item}`,
      options: { method: 'PATCH', body: '{"quantity":null}' },
    },
    {
      run: () => service.deleteItem(list, item),
      path: `${base}/items/${item}`,
      options: { method: 'DELETE' },
    },
    {
      run: () => service.clearItems(list),
      path: `${base}/items`,
      options: { method: 'DELETE' },
    },
    {
      run: () => service.markAll(list, false),
      path: `${base}/items/mark-all`,
      options: { method: 'PATCH', body: '{"isMarked":false}' },
    },
  ])(
    '$path сохраняет строковые ID, decimal, тело и ошибки',
    async ({ run, path, options }) => {
      request.mockResolvedValueOnce({ id: item, quantity: create.quantity });
      await expect(run()).resolves.toEqual({
        id: item,
        quantity: create.quantity,
      });
      expect(request).toHaveBeenLastCalledWith(path, options);
      const error = new Error('offline');
      request.mockRejectedValueOnce(error);
      await expect(run()).rejects.toBe(error);
    },
  );
  it('передаёт signal чтения и сохраняет ключ при повторе POST', async () => {
    const { signal } = new AbortController();
    await service.getLists(signal);
    await service.getList(list, signal);
    await service.getItems(list, signal);
    await service.getItem(list, item, signal);
    for (const [, options] of request.mock.calls)
      expect(options?.signal).toBe(signal);
    await service.createItem(list, create, key);
    await service.createItem(list, create, key);
    expect(request.mock.calls.at(-1)).toEqual(request.mock.calls.at(-2));
  });
});
