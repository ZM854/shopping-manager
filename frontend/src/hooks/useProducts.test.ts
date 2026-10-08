import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useProducts } from './useProducts';
import ProductService from '../services/productService';
vi.mock('../services/productService', () => ({
  default: {
    getProducts: vi.fn(),
    postProduct: vi.fn(),
    updateProduct: vi.fn(),
    deleteProduct: vi.fn(),
    deleteAllProducts: vi.fn(),
  },
}));
const service = vi.mocked(ProductService);
const initial = {
  id: 1,
  name: 'Молоко',
  quantity: 1.5,
  unit: 'л',
  isMarked: false,
};
beforeEach(() => {
  vi.resetAllMocks();
  service.getProducts.mockResolvedValue([initial]);
});
describe('useProducts', () => {
  it('загружает и изменяет локальное состояние после успешных CRUD', async () => {
    const { result } = renderHook(useProducts);
    await waitFor(() => expect(result.current.products).toEqual([initial]));
    const added = { ...initial, id: 2, name: 'Хлеб' };
    service.postProduct.mockResolvedValue(added);
    await act(async () => {
      await result.current.createProduct(added);
    });
    expect(result.current.products).toEqual([initial, added]);
    service.updateProduct.mockResolvedValue({ ...initial, isMarked: true });
    await act(async () => {
      await result.current.updateProduct(1, { ...initial, isMarked: true });
    });
    expect(result.current.products[0].isMarked).toBe(true);
    service.deleteProduct.mockResolvedValue(undefined as never);
    await act(async () => {
      await result.current.deleteProduct(1);
    });
    expect(result.current.products).toEqual([added]);
    service.deleteAllProducts.mockResolvedValue([]);
    await act(async () => {
      await result.current.deleteAllProducts();
    });
    expect(result.current.products).toEqual([]);
  });
  it('показывает ошибку загрузки', async () => {
    service.getProducts.mockRejectedValue(new Error('offline'));
    const { result } = renderHook(useProducts);
    await waitFor(() =>
      expect(result.current.error).toBe('failed to load products'),
    );
    expect(result.current.products).toEqual([]);
  });
  it.each(['create', 'delete', 'clear'] as const)(
    '%s при отказе сохраняет данные и пробрасывает ошибку',
    async (operation) => {
      const { result } = renderHook(useProducts);
      await waitFor(() => expect(result.current.products).toHaveLength(1));
      const error = new Error('offline');
      service.postProduct.mockRejectedValue(error);
      service.deleteProduct.mockRejectedValue(error);
      service.deleteAllProducts.mockRejectedValue(error);
      await act(async () => {
        const promise =
          operation === 'create'
            ? result.current.createProduct(initial)
            : operation === 'delete'
              ? result.current.deleteProduct(1)
              : result.current.deleteAllProducts();
        await expect(promise).rejects.toBe(error);
      });
      expect(result.current.products).toEqual([initial]);
      expect(result.current.error).not.toBeNull();
    },
  );
});
