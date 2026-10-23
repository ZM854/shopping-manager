import { StrictMode } from 'react';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../context/authContext/AuthProvider';
import RequireAuth from '../router/RequireAuth';
import Layout from '../layout/Layout';
import RecipesPage from '../pages/RecipesPage/RecipesPage';
import RecipePage from '../pages/RecipePage/RecipePage';
import RecipeEditorPage from '../pages/RecipeEditorPage/RecipeEditorPage';
import { recipeService } from '../services/recipeService';
import { sessionCoordinator } from '../services/session';
import { ApiError } from '../services/api';
import type { Recipe, SaveRecipeRequest } from '../models/recipe';

vi.mock('../services/recipeService', () => ({
  recipeService: {
    getCategories: vi.fn(),
    getRecipes: vi.fn(),
    getRecipe: vi.fn(),
    createRecipe: vi.fn(),
    updateRecipe: vi.fn(),
    deleteRecipe: vi.fn(),
  },
}));
const api = vi.mocked(recipeService);
const fetchMock = vi.fn<typeof fetch>();
const id = '9007199254740993';
const otherId = '9007199254740994';
const categories = [
  { code: 'baking', name: 'Выпечка' },
  { code: 'soups', name: 'Супы' },
];
const auth = {
  user: {
    id: '1',
    name: 'Тест',
    email: 'test@example.test',
    isEmailVerified: true,
  },
  accessToken: 'test-access',
};
let recipes: Recipe[];
function recipe(recipeId = id, title = 'Первый рецепт'): Recipe {
  return {
    id: recipeId,
    title,
    cookingTimeMinutes: 30,
    categoryCode: 'baking',
    createdAt: '2026-10-23T10:00:00Z',
    updatedAt: '2026-10-23T10:00:00Z',
    ingredients: [
      { position: 1, name: 'Мука', quantity: '999999999.999', unit: 'г' },
    ],
    steps: [{ position: 1, text: 'Смешать\n\tИспечь' }],
  };
}
function saved(body: SaveRecipeRequest, recipeId = '9007199254740995'): Recipe {
  return {
    ...recipe(recipeId),
    ...body,
    ingredients: body.ingredients.map((ingredient, index) => ({
      ...ingredient,
      position: index + 1,
    })),
    steps: body.steps.map((step, index) => ({ ...step, position: index + 1 })),
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  sessionCoordinator.dispose();
  localStorage.clear();
  vi.resetAllMocks();
  fetchMock.mockImplementation(async () => Response.json(auth));
  vi.stubGlobal('fetch', fetchMock);
  recipes = [recipe(), recipe(otherId, 'Второй рецепт')];
  api.getCategories.mockResolvedValue(categories);
  api.getRecipes.mockImplementation(async () => [...recipes]);
  api.getRecipe.mockImplementation(async (recipeId) => {
    const found = recipes.find((item) => item.id === recipeId);
    if (!found) throw new ApiError(404, 'Рецепт не найден', 'not_found');
    return found;
  });
  api.createRecipe.mockImplementation(async (body) => {
    const result = saved(body);
    recipes.push(result);
    return result;
  });
  api.updateRecipe.mockImplementation(async (recipeId, body) => {
    const result = saved(body, recipeId);
    recipes = recipes.map((item) => (item.id === recipeId ? result : item));
    return result;
  });
  api.deleteRecipe.mockImplementation(async (recipeId) => {
    recipes = recipes.filter((item) => item.id !== recipeId);
  });
});
afterEach(() => {
  sessionCoordinator.dispose();
  vi.unstubAllGlobals();
});

function show(path = '/recipes') {
  return render(
    <StrictMode>
      <AuthProvider>
        <MemoryRouter initialEntries={[path]}>
          <Link to={`/recipes/${otherId}`}>Другой рецепт</Link>
          <Link to={`/recipes/${otherId}/edit`}>Другой редактор</Link>
          <Routes>
            <Route path="/login" element={<p>Страница входа</p>} />
            <Route element={<RequireAuth />}>
              <Route element={<Layout />}>
                <Route path="/recipes" element={<RecipesPage />} />
                <Route path="/recipes/new" element={<RecipeEditorPage />} />
                <Route path="/recipes/:recipeId" element={<RecipePage />} />
                <Route
                  path="/recipes/:recipeId/edit"
                  element={<RecipeEditorPage />}
                />
              </Route>
            </Route>
          </Routes>
        </MemoryRouter>
      </AuthProvider>
    </StrictMode>,
  );
}
async function fill(
  user: ReturnType<typeof userEvent.setup>,
  title = 'Новый пирог',
) {
  await user.type(await screen.findByLabelText('Название рецепта'), title);
  await user.selectOptions(screen.getByLabelText('Категория блюда'), 'baking');
  fireEvent.change(screen.getByLabelText('Название ингредиента 1'), {
    target: { value: 'Яйца' },
  });
  fireEvent.change(screen.getByLabelText('Количество ингредиента 1'), {
    target: { value: '2.500' },
  });
  fireEvent.change(screen.getByLabelText('Описание шага 1'), {
    target: { value: 'Первый шаг' },
  });
}

describe('Демонстрационный клиент рецептов', () => {
  it('показывает коллекцию, метаданные и полный текст с точным количеством', async () => {
    show();
    const user = userEvent.setup();
    expect(
      await screen.findByRole('link', { name: 'Первый рецепт' }),
    ).toHaveAttribute('href', `/recipes/${id}`);
    expect(screen.getByRole('link', { name: 'Рецепты' })).toHaveAttribute(
      'href',
      '/recipes',
    );
    await user.click(screen.getByRole('link', { name: 'Первый рецепт' }));
    expect(
      await screen.findByRole('heading', { level: 1, name: 'Первый рецепт' }),
    ).toBeVisible();
    expect(screen.getByText('Мука — 999999999.999 — г')).toBeVisible();
    expect(screen.getByText('Смешать Испечь')).toHaveTextContent(
      'Смешать Испечь',
    );
    expect(screen.getByText('Смешать Испечь').textContent).toBe(
      'Смешать\n\tИспечь',
    );
  });
  it('показывает пустую коллекцию и позволяет повторить отказ загрузки', async () => {
    api.getRecipes.mockRejectedValue(new Error('offline'));
    show();
    expect(await screen.findByRole('alert')).toHaveTextContent('offline');
    api.getRecipes.mockResolvedValue([]);
    await userEvent.click(
      screen.getByRole('button', { name: 'Повторить загрузку' }),
    );
    expect(await screen.findByText('У вас пока нет рецептов.')).toBeVisible();
  });
  it('создаёт рецепт, добавляет/удаляет строки и меняет порядок шагов с клавиатуры', async () => {
    show('/recipes/new');
    const user = userEvent.setup();
    await fill(user);
    await user.click(
      screen.getByRole('button', { name: 'Добавить ингредиент' }),
    );
    fireEvent.change(screen.getByLabelText('Название ингредиента 2'), {
      target: { value: 'Соль' },
    });
    fireEvent.change(screen.getByLabelText('Единица ингредиента 2'), {
      target: { value: 'по вкусу' },
    });
    await user.click(
      screen.getByRole('button', { name: 'Добавить ингредиент' }),
    );
    await user.click(
      screen.getByRole('button', { name: 'Удалить ингредиент 3' }),
    );
    await user.click(screen.getByRole('button', { name: 'Добавить шаг' }));
    fireEvent.change(screen.getByLabelText('Описание шага 2'), {
      target: { value: 'Второй шаг' },
    });
    screen.getByRole('button', { name: 'Шаг 2: выше' }).focus();
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('Описание шага 1')).toHaveValue('Второй шаг');
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    await screen.findByRole('heading', { level: 1, name: 'Новый пирог' });
    const [body, key] = api.createRecipe.mock.calls[0];
    expect(body).toEqual({
      title: 'Новый пирог',
      cookingTimeMinutes: 30,
      categoryCode: 'baking',
      ingredients: [
        { name: 'Яйца', quantity: '2.5', unit: '' },
        { name: 'Соль', quantity: null, unit: 'по вкусу' },
      ],
      steps: [{ text: 'Второй шаг' }, { text: 'Первый шаг' }],
    });
    expect(key).toMatch(/^[A-Za-z0-9_-]{16,128}$/);
  });
  it('заменяет рецепт через PUT без ID/позиций в теле', async () => {
    show(`/recipes/${id}/edit`);
    const user = userEvent.setup();
    const title = await screen.findByLabelText('Название рецепта');
    expect(title).toHaveValue('Первый рецепт');
    await user.clear(title);
    await user.type(title, 'Правка');
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    await screen.findByRole('heading', { level: 1, name: 'Правка' });
    expect(api.createRecipe).not.toHaveBeenCalled();
    expect(api.updateRecipe.mock.calls[0][0]).toBe(id);
    expect(Object.keys(api.updateRecipe.mock.calls[0][1])).toEqual([
      'title',
      'cookingTimeMinutes',
      'categoryCode',
      'ingredients',
      'steps',
    ]);
    expect(api.updateRecipe.mock.calls[0][1].steps).toEqual([
      { text: 'Смешать\n\tИспечь' },
    ]);
  });
  it('валидирует до POST и отображает серверные ошибки вложенных полей', async () => {
    show('/recipes/new');
    const user = userEvent.setup();
    await screen.findByLabelText('Название рецепта');
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    expect(await screen.findAllByRole('alert')).not.toHaveLength(0);
    expect(api.createRecipe).not.toHaveBeenCalled();
    await fill(user);
    api.createRecipe.mockRejectedValueOnce(
      new ApiError(400, 'Проверьте поля', 'validation_failed', {
        'steps.0.text': 'Ошибка описания',
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    expect(await screen.findByText('Ошибка описания')).toBeVisible();
    expect(screen.getByLabelText('Описание шага 1')).toHaveAttribute(
      'aria-invalid',
      'true',
    );
    expect(screen.getByLabelText('Название рецепта')).toBeEnabled();
  });
  it('блокирует двойную отправку и повторяет неизменное тело с прежним ключом после потери ответа', async () => {
    const pending = deferred<Recipe>();
    api.createRecipe.mockReturnValueOnce(pending.promise);
    show('/recipes/new');
    const user = userEvent.setup();
    await fill(user);
    await user.dblClick(
      screen.getByRole('button', { name: 'Сохранить рецепт' }),
    );
    expect(api.createRecipe).toHaveBeenCalledTimes(1);
    expect(screen.getByLabelText('Название рецепта')).toBeDisabled();
    await act(async () => pending.reject(new TypeError('offline')));
    expect(
      await screen.findByRole('button', { name: 'Повторить сохранение' }),
    ).toBeVisible();
    expect(screen.getByRole('button', { name: 'Добавить шаг' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Название рецепта'), {
      target: { value: 'Скрытая правка' },
    });
    await user.click(
      screen.getByRole('button', { name: 'Повторить сохранение' }),
    );
    await screen.findByRole('heading', { level: 1, name: 'Новый пирог' });
    expect(api.createRecipe.mock.calls[1]).toEqual(
      api.createRecipe.mock.calls[0],
    );
  });
  it.each(['resource_deleted', 'idempotency_conflict'])(
    'требует отдельного нового действия при %s',
    async (code) => {
      api.createRecipe.mockRejectedValueOnce(
        new ApiError(409, 'Конфликт запроса', code),
      );
      show('/recipes/new');
      const user = userEvent.setup();
      await fill(user);
      await user.click(
        screen.getByRole('button', { name: 'Сохранить рецепт' }),
      );
      expect(
        await screen.findByRole('button', { name: 'Начать новое сохранение' }),
      ).toBeVisible();
      expect(
        screen.getByRole('button', { name: 'Сохранить рецепт' }),
      ).toBeDisabled();
      await user.click(
        screen.getByRole('button', { name: 'Начать новое сохранение' }),
      );
      expect(screen.getByLabelText('Название рецепта')).toHaveValue(
        'Новый пирог',
      );
      await user.click(
        screen.getByRole('button', { name: 'Сохранить рецепт' }),
      );
      await screen.findByRole('heading', { level: 1, name: 'Новый пирог' });
      expect(api.createRecipe.mock.calls[1][1]).not.toBe(
        api.createRecipe.mock.calls[0][1],
      );
    },
  );
  it('отмена до отправки не создаёт рецепт', async () => {
    show('/recipes/new');
    await screen.findByLabelText('Название рецепта');
    await userEvent.click(screen.getByRole('button', { name: 'Отмена' }));
    await screen.findByRole('heading', { level: 1, name: 'Мои рецепты' });
    expect(api.createRecipe).not.toHaveBeenCalled();
  });

  it('после 503 повторяет PUT с прежним телом и сохраняет черновик', async () => {
    api.updateRecipe.mockRejectedValueOnce(
      new ApiError(503, 'Временно недоступно', 'internal_error'),
    );
    show(`/recipes/${id}/edit`);
    const user = userEvent.setup();
    const title = await screen.findByLabelText('Название рецепта');
    await user.clear(title);
    await user.type(title, 'Новая версия');
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    expect(
      await screen.findByRole('button', { name: 'Повторить сохранение' }),
    ).toBeVisible();
    expect(title).toBeDisabled();
    await user.click(
      screen.getByRole('button', { name: 'Повторить сохранение' }),
    );
    await screen.findByRole('heading', { level: 1, name: 'Новая версия' });
    expect(api.updateRecipe.mock.calls[1]).toEqual(
      api.updateRecipe.mock.calls[0],
    );
  });

  it('не закрывает диалог удаления по Escape и не дублирует ожидающий DELETE', async () => {
    const pending = deferred<void>();
    api.deleteRecipe.mockReturnValueOnce(pending.promise);
    show(`/recipes/${id}`);
    const user = userEvent.setup();
    await screen.findByRole('heading', { level: 1, name: 'Первый рецепт' });
    await user.click(screen.getByRole('button', { name: 'Удалить рецепт' }));
    const dialog = screen.getByRole('dialog');
    await user.dblClick(
      within(dialog).getByRole('button', { name: 'Удалить' }),
    );
    await user.keyboard('{Escape}');
    expect(dialog).toBeVisible();
    expect(
      within(dialog).getByRole('button', { name: 'Отменить' }),
    ).toBeDisabled();
    expect(api.deleteRecipe).toHaveBeenCalledTimes(1);
    await act(async () => pending.resolve());
    await screen.findByRole('heading', { level: 1, name: 'Мои рецепты' });
  });
  it('после неоднозначного DELETE проверяет отсутствие, не выдавая первую ошибку за успех', async () => {
    show(`/recipes/${id}`);
    const user = userEvent.setup();
    await screen.findByRole('heading', { level: 1, name: 'Первый рецепт' });
    api.deleteRecipe.mockImplementationOnce(async () => {
      recipes = recipes.filter((item) => item.id !== id);
      throw new TypeError('Ответ потерян');
    });
    await user.click(screen.getByRole('button', { name: 'Удалить рецепт' }));
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Удалить' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'Ответ потерян',
    );
    await user.click(within(dialog).getByRole('button', { name: 'Удалить' }));
    await screen.findByRole('heading', { level: 1, name: 'Мои рецепты' });
    expect(api.deleteRecipe).toHaveBeenCalledTimes(1);
  });
  it('отбрасывает загрузку и результат сохранения после смены страницы', async () => {
    const loading = deferred<Recipe>();
    api.getRecipe.mockImplementation((recipeId) =>
      recipeId === id
        ? loading.promise
        : Promise.resolve(recipe(otherId, 'Второй рецепт')),
    );
    show(`/recipes/${id}/edit`);
    const user = userEvent.setup();
    await user.click(screen.getByRole('link', { name: 'Другой редактор' }));
    expect(await screen.findByLabelText('Название рецепта')).toHaveValue(
      'Второй рецепт',
    );
    await act(async () => loading.resolve(recipe()));
    expect(screen.getByLabelText('Название рецепта')).toHaveValue(
      'Второй рецепт',
    );
    const saving = deferred<Recipe>();
    api.updateRecipe.mockReturnValueOnce(saving.promise);
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    await user.click(screen.getByRole('link', { name: 'Другой рецепт' }));
    await screen.findByRole('heading', { level: 1, name: 'Второй рецепт' });
    await act(async () => saving.resolve(recipe(id, 'Поздний ответ')));
    expect(
      screen.getByRole('heading', { level: 1, name: 'Второй рецепт' }),
    ).toBeVisible();
  });
  it('смена аккаунта сбрасывает форму и позднее сохранение не меняет новый маршрут', async () => {
    const pending = deferred<Recipe>();
    api.createRecipe.mockReturnValueOnce(pending.promise);
    show('/recipes/new');
    const user = userEvent.setup();
    await fill(user);
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    fetchMock.mockResolvedValueOnce(
      Response.json({ ...auth, user: { ...auth.user, id: '2' } }),
    );
    await act(async () =>
      sessionCoordinator.login({
        email: 'second@example.test',
        password: 'password',
      }),
    );
    expect(await screen.findByLabelText('Название рецепта')).toHaveValue('');
    await act(async () => pending.resolve(recipe(id, 'Поздний ответ')));
    expect(screen.getByLabelText('Название рецепта')).toHaveValue('');
    expect(
      screen.queryByRole('heading', { level: 1, name: 'Поздний ответ' }),
    ).not.toBeInTheDocument();
  });
  it('временный отказ refresh сохраняет форму и прежнюю попытку того же пользователя', async () => {
    show('/recipes/new');
    const user = userEvent.setup();
    await fill(user);
    fetchMock.mockResolvedValueOnce(
      Response.json(
        { code: 'origin_forbidden', message: 'Временный отказ refresh' },
        { status: 403 },
      ),
    );
    api.createRecipe.mockImplementationOnce(async () => {
      await sessionCoordinator.refresh();
      return recipe();
    });
    await user.click(screen.getByRole('button', { name: 'Сохранить рецепт' }));
    const retrySession = await screen.findByRole('button', {
      name: 'Повторить проверку сессии',
    });
    expect(screen.getByLabelText('Название рецепта')).not.toBeVisible();
    await user.click(retrySession);
    await waitFor(() =>
      expect(screen.getByLabelText('Название рецепта')).toBeVisible(),
    );
    expect(screen.getByLabelText('Название рецепта')).toHaveValue(
      'Новый пирог',
    );
    await user.click(
      screen.getByRole('button', { name: 'Повторить сохранение' }),
    );
    await screen.findByRole('heading', { level: 1, name: 'Новый пирог' });
    expect(api.createRecipe.mock.calls[1]).toEqual(
      api.createRecipe.mock.calls[0],
    );
  });
});
