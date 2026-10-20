import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import { requestJSON } from './http';
import ProductService from './productService';
import { authService } from './authService';
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from './tokenStorage';
vi.mock('./api', () => ({ apiFetch: vi.fn() }));
vi.mock('./http', () => ({ requestJSON: vi.fn() }));
beforeEach(() => {
  vi.resetAllMocks();
  clearAccessToken();
});
const product = { name: 'Молоко', quantity: 1.5, unit: 'л' };
const update = { ...product, isMarked: true };
const login = { email: 'user@example.test', password: 'test-password' };
describe('Сервисы текущего API', () => {
  it.each([
    {
      run: () => ProductService.getProducts(),
      path: '/products',
      options: undefined,
    },
    {
      run: () => ProductService.getProductById(7),
      path: '/products/7',
      options: undefined,
    },
    {
      run: () => ProductService.postProduct(product),
      path: '/products',
      options: { method: 'POST', body: JSON.stringify(product) },
    },
    {
      run: () => ProductService.updateProduct(7, update),
      path: '/products/7',
      options: { method: 'PUT', body: JSON.stringify(update) },
    },
    {
      run: () => ProductService.deleteProduct(7),
      path: '/products/7',
      options: { method: 'DELETE' },
    },
    {
      run: () => ProductService.deleteAllProducts(),
      path: '/products',
      options: { method: 'DELETE' },
    },
    {
      run: () => authService.login(login),
      path: '/login',
      options: {
        method: 'POST',
        body: JSON.stringify(login),
      },
    },
    {
      run: () => authService.register({ ...login, name: 'Имя' }),
      path: '/registration',
      options: {
        method: 'POST',
        body: JSON.stringify({ ...login, name: 'Имя' }),
      },
    },
    {
      run: () => authService.refresh(),
      path: '/refresh',
      options: { method: 'POST' },
    },
    {
      run: () => authService.activate('test-token'),
      path: '/activation',
      options: {
        method: 'POST',
        body: JSON.stringify({ token: 'test-token' }),
      },
    },
    {
      run: () => authService.resendActivation(login.email),
      path: '/activation/resend',
      options: {
        method: 'POST',
        body: JSON.stringify({ email: login.email }),
      },
    },
    {
      run: () => authService.logout(),
      path: '/logout',
      options: { method: 'POST' },
    },
  ])(
    '$path $options.method сохраняет контракт и ошибки',
    async ({ run, path, options }) => {
      const request = vi.mocked(
        path.startsWith('/products') ? apiFetch : requestJSON,
      );
      request.mockResolvedValue({ result: 'test' });
      await expect(run()).resolves.toEqual({ result: 'test' });
      if (options) expect(request).toHaveBeenCalledWith(path, options);
      else expect(request).toHaveBeenCalledWith(path);
      request.mockRejectedValue(new Error('failed'));
      await expect(run()).rejects.toThrow('failed');
    },
  );
  it('access хранится в памяти и удаляется при очистке', () => {
    expect(getAccessToken()).toBeNull();
    setAccessToken('test');
    expect(getAccessToken()).toBe('test');
    setAccessToken(null);
    expect(getAccessToken()).toBeNull();
    setAccessToken('other');
    clearAccessToken();
    expect(getAccessToken()).toBeNull();
  });
});
