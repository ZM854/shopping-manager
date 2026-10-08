import { beforeEach, describe, expect, it, vi } from 'vitest';
import { apiFetch } from './api';
import ProductService from './productService';
import { authService } from './authService';
import {
  clearAccessToken,
  getAccessToken,
  setAccessToken,
} from './tokenStorage';
vi.mock('./api', () => ({ apiFetch: vi.fn() }));
const request = vi.mocked(apiFetch);
beforeEach(() => {
  request.mockReset();
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
      options: { method: 'POST', body: JSON.stringify(login) },
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
      options: { method: 'POST', skipRefresh: true },
    },
    {
      run: () => authService.logout(),
      path: '/logout',
      options: { method: 'POST' },
    },
  ])(
    '$path $options.method сохраняет контракт и ошибки',
    async ({ run, path, options }) => {
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
