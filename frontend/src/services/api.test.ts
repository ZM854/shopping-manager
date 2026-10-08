import { beforeEach, describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { apiFetch } from './api';
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from './tokenStorage';
vi.mock('../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), warn: vi.fn(), error: vi.fn() },
}));
const fetchMock = vi.fn<typeof fetch>();
beforeEach(() => {
  clearAccessToken();
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

describe('API-клиент', () => {
  it('передаёт bearer, cookies, тело и пользовательские заголовки', async () => {
    setAccessToken('test-access');
    fetchMock.mockResolvedValue(Response.json({ id: 7 }));
    await expect(
      apiFetch('/products', {
        method: 'POST',
        body: '{"name":"Молоко"}',
        headers: { 'X-Test': 'yes' },
      }),
    ).resolves.toEqual({ id: 7 });
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/products');
    expect(options?.credentials).toBe('include');
    expect(options?.body).toBe('{"name":"Молоко"}');
    const headers = new Headers(options?.headers);
    expect(headers.get('Authorization')).toBe('Bearer test-access');
    expect(headers.get('Content-Type')).toBe('application/json');
    expect(headers.get('X-Test')).toBe('yes');
  });
  it('обрабатывает 204 без попытки прочесть JSON', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(
      apiFetch('/products', { method: 'DELETE' }),
    ).resolves.toBeUndefined();
  });
  it('пробрасывает ошибки сети, HTTP и JSON', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline'));
    await expect(apiFetch('/products')).rejects.toThrow('offline');
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 500 }));
    await expect(apiFetch('/products')).rejects.toThrow('Request failed');
    fetchMock.mockResolvedValueOnce(
      new Response('broken json', { status: 200 }),
    );
    await expect(apiFetch('/products')).rejects.toThrow();
  });
  it('объединяет refresh параллельных 401 и повторяет запросы с новым bearer', async () => {
    setAccessToken('old');
    let resolveRefresh!: (response: Response) => void;
    const refresh = new Promise<Response>((resolve) => {
      resolveRefresh = resolve;
    });
    let refreshCalls = 0;
    let originalCalls = 0;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/refresh')) {
        refreshCalls++;
        return refresh;
      }
      originalCalls++;
      if (new Headers(options?.headers).get('Authorization') === 'Bearer old')
        return new Response(null, { status: 401 });
      return Response.json({ url: String(url) });
    });
    const results = Promise.all([
      apiFetch('/products'),
      apiFetch('/products/7'),
    ]);
    await waitFor(() => {
      expect(refreshCalls).toBe(1);
      expect(originalCalls).toBe(2);
    });
    resolveRefresh(Response.json({ accessToken: 'new', user: { id: 1 } }));
    await expect(results).resolves.toEqual([
      { url: '/api/products' },
      { url: '/api/products/7' },
    ]);
    expect(refreshCalls).toBe(1);
    expect(originalCalls).toBe(4);
    expect(getAccessToken()).toBe('new');
  });
  it('очищает access при отказе refresh и допускает новую попытку позже', async () => {
    setAccessToken('old');
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(new Response(null, { status: 403 }));
    await expect(apiFetch('/products')).rejects.toThrow('Request failed');
    expect(getAccessToken()).toBeNull();
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(Response.json({ accessToken: 'new' }))
      .mockResolvedValueOnce(Response.json([]));
    await expect(apiFetch('/products')).resolves.toEqual([]);
    expect(getAccessToken()).toBe('new');
  });
  it('не зацикливается при повторном 401 и учитывает skipRefresh', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(Response.json({ accessToken: 'new' }))
      .mockResolvedValueOnce(new Response(null, { status: 401 }));
    await expect(apiFetch('/products')).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(3);
    fetchMock.mockClear();
    fetchMock.mockResolvedValue(new Response(null, { status: 401 }));
    await expect(apiFetch('/refresh', { skipRefresh: true })).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
