import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { recipeService } from './recipeService';
import { sessionCoordinator } from './session';
import { ApiError } from './api';
import type { SaveRecipeRequest } from '../models/recipe';

const fetchMock = vi.fn<typeof fetch>();
const body: SaveRecipeRequest = {
  title: 'Пирог',
  cookingTimeMinutes: 30,
  categoryCode: 'baking',
  ingredients: [{ name: 'Мука', quantity: '999999999.999', unit: '' }],
  steps: [{ text: 'Испечь' }],
};
beforeEach(() => {
  sessionCoordinator.dispose();
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  sessionCoordinator.dispose();
  vi.unstubAllGlobals();
});

describe('HTTP клиент рецептов', () => {
  it.each([
    {
      run: () => recipeService.getCategories(),
      path: '/recipe-categories',
      method: 'GET',
    },
    { run: () => recipeService.getRecipes(), path: '/recipes', method: 'GET' },
    {
      run: () => recipeService.getRecipe('9007199254740993'),
      path: '/recipes/9007199254740993',
      method: 'GET',
    },
    {
      run: () => recipeService.createRecipe(body, 'recipe-action-0001'),
      path: '/recipes',
      method: 'POST',
    },
    {
      run: () => recipeService.updateRecipe('9007199254740993', body),
      path: '/recipes/9007199254740993',
      method: 'PUT',
    },
    {
      run: () => recipeService.deleteRecipe('9007199254740993'),
      path: '/recipes/9007199254740993',
      method: 'DELETE',
    },
  ])('$method $path', async ({ run, path, method }) => {
    fetchMock.mockResolvedValue(
      method === 'DELETE'
        ? new Response(null, { status: 204 })
        : Response.json([]),
    );
    await run();
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe(`/api${path}`);
    expect(options?.method ?? 'GET').toBe(method);
    if (method === 'POST' || method === 'PUT')
      expect(options?.body).toBe(JSON.stringify(body));
    expect(new Headers(options?.headers).get('Idempotency-Key')).toBe(
      method === 'POST' ? 'recipe-action-0001' : null,
    );
  });
  it('передаёт AbortSignal и кодирует ID как одну часть пути', async () => {
    fetchMock.mockResolvedValue(Response.json([]));
    const controller = new AbortController();
    await recipeService.getRecipe('a/b', controller.signal);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/recipes/a%2Fb');
    expect(fetchMock.mock.calls[0][1]?.signal).toBe(controller.signal);
  });
  it('повторяет POST после refresh теми же телом и ключом', async () => {
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        Response.json({
          user: {
            id: '1',
            name: 'Тест',
            email: 'test@example.test',
            isEmailVerified: true,
          },
          accessToken: 'new-access',
        }),
      )
      .mockResolvedValueOnce(
        Response.json({ id: '9007199254740993', ...body }),
      );
    const recipe = await recipeService.createRecipe(body, 'recipe-action-0001');
    expect(recipe.id).toBe('9007199254740993');
    expect(fetchMock).toHaveBeenCalledTimes(3);
    for (const index of [0, 2]) {
      expect(fetchMock.mock.calls[index][1]?.body).toBe(JSON.stringify(body));
      expect(
        new Headers(fetchMock.mock.calls[index][1]?.headers).get(
          'Idempotency-Key',
        ),
      ).toBe('recipe-action-0001');
    }
    expect(
      new Headers(fetchMock.mock.calls[2][1]?.headers).get('Authorization'),
    ).toBe('Bearer new-access');
  });
  it('не подавляет ошибки API и сети', async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json(
        { code: 'resource_deleted', message: 'Удалён' },
        { status: 409 },
      ),
    );
    await expect(
      recipeService.createRecipe(body, 'recipe-action-0001'),
    ).rejects.toBeInstanceOf(ApiError);
    fetchMock.mockRejectedValueOnce(new TypeError('offline'));
    await expect(recipeService.getRecipes()).rejects.toThrow('offline');
  });
});
