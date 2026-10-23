import { randomUUID } from 'node:crypto';
import { expect, test, type Page } from '@playwright/test';
import type { Recipe, SaveRecipeRequest } from '../src/models/recipe';
import { appOrigin, registerAndLogin } from './helpers';

async function fillRecipe(page: Page, title: string, text: string) {
  await page.goto('/recipes/new');
  await page.getByLabel('Название рецепта', { exact: true }).fill(title);
  await page.getByLabel('Время приготовления').selectOption('45');
  await page.getByLabel('Категория блюда').selectOption('soups');
  await page.getByLabel('Название ингредиента 1').fill('Соль');
  await page.getByLabel('Описание шага 1').fill(text);
}

function saving(page: Page, method: 'POST' | 'PUT') {
  return page.waitForResponse(
    (response) =>
      /\/api\/recipes(?:\/[1-9][0-9]*)?$/.test(response.url()) &&
      response.request().method() === method,
  );
}

test('личный CRUD: большой шаг, точные количества, порядок и два владельца после reload', async ({
  page,
  request,
  browser,
}) => {
  const owner = await registerAndLogin(page, request);
  await page.getByRole('link', { name: 'Рецепты', exact: true }).click();
  await expect(page.getByText('У вас пока нет рецептов.')).toBeVisible();
  const ending = '\n\tГотовить до готовности.\nНе обрезать текст.';
  const longText = `${'🥣'.repeat(20000 - [...ending].length)}${ending}`;
  expect([...longText].length).toBe(20000);
  await fillRecipe(page, 'Большой суп', longText);
  await page.getByLabel('Единица ингредиента 1').fill('по вкусу');
  await page.getByRole('button', { name: 'Добавить ингредиент' }).click();
  await page.getByLabel('Название ингредиента 2').fill('Вода');
  await page.getByLabel('Количество ингредиента 2').fill('999999999.999');
  await page.getByRole('button', { name: 'Добавить ингредиент' }).click();
  await page.getByLabel('Название ингредиента 3').fill('Соль');
  const created = saving(page, 'POST');
  await page
    .getByRole('button', { name: 'Сохранить рецепт', exact: true })
    .click();
  const response = await created;
  expect(response.status()).toBe(201);
  expect(response.headers()['cache-control']).toBe('no-store');
  const original: Recipe = await response.json();
  expect(original.id).toMatch(/^[1-9][0-9]*$/);
  expect(original.steps).toEqual([{ position: 1, text: longText }]);
  expect(original.ingredients).toEqual([
    { position: 1, name: 'Соль', quantity: null, unit: 'по вкусу' },
    { position: 2, name: 'Вода', quantity: '999999999.999', unit: '' },
    { position: 3, name: 'Соль', quantity: null, unit: '' },
  ]);
  await expect(page).toHaveURL(`/recipes/${original.id}`);
  await page.reload();
  await expect(page.locator('ol > li > p')).toHaveText([longText], {
    useInnerText: false,
  });
  // textContent проверяет сохранность переносов и табуляции без нормализации Playwright.
  expect(await page.locator('ol > li > p').textContent()).toBe(longText);
  await expect(
    page.getByText('Вода — 999999999.999', { exact: true }),
  ).toBeVisible();

  const otherContext = await browser.newContext({ baseURL: appOrigin });
  try {
    const otherPage = await otherContext.newPage();
    const other = await registerAndLogin(otherPage, request);
    const headers = {
      Authorization: `Bearer ${other.accessToken}`,
      Origin: appOrigin,
    };
    for (const method of ['GET', 'PUT', 'DELETE'] as const) {
      const denied = await otherPage.request.fetch(
        `/api/recipes/${original.id}`,
        {
          method,
          headers,
          ...(method === 'PUT'
            ? {
                data: {
                  title: 'Чужая правка',
                  cookingTimeMinutes: 5,
                  categoryCode: 'other',
                  ingredients: [{ name: 'Вода' }],
                  steps: [{ text: 'Шаг' }],
                },
              }
            : {}),
        },
      );
      expect(denied.status()).toBe(404);
      expect((await denied.json()).code).toBe('not_found');
      expect(denied.headers()['cache-control']).toBe('no-store');
    }
    await otherPage.goto(`/recipes/${original.id}`);
    await expect(otherPage.getByRole('alert')).toBeVisible();
    await otherPage.goto('/recipes');
    await expect(otherPage.getByText('У вас пока нет рецептов.')).toBeVisible();
  } finally {
    await otherContext.close();
  }

  await page.getByRole('link', { name: 'Редактировать рецепт' }).click();
  await expect(page.getByLabel('Описание шага 1')).toHaveValue(longText);
  await page.getByLabel('Описание шага 1').fill('Первый\n\tОтступ сохранён.');
  await page.getByRole('button', { name: 'Добавить шаг', exact: true }).click();
  await page.getByLabel('Описание шага 2').fill('Второй');
  await page.getByRole('button', { name: 'Шаг 2: выше', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByLabel('Описание шага 1')).toHaveValue('Второй');
  await page
    .getByRole('button', { name: 'Удалить ингредиент 3', exact: true })
    .click();
  await page
    .getByLabel('Название рецепта', { exact: true })
    .fill('Обновлённый суп');
  const updated = saving(page, 'PUT');
  await page
    .getByRole('button', { name: 'Сохранить рецепт', exact: true })
    .click();
  const saved = await updated;
  expect(saved.status()).toBe(200);
  const edited: Recipe = await saved.json();
  expect(edited.id).toBe(original.id);
  expect(edited.steps).toEqual([
    { position: 1, text: 'Второй' },
    { position: 2, text: 'Первый\n\tОтступ сохранён.' },
  ]);
  expect(edited.ingredients).toEqual(original.ingredients.slice(0, 2));
  await expect(page).toHaveURL(`/recipes/${original.id}`);
  await page.reload();
  await expect(
    page.getByRole('heading', { name: 'Обновлённый суп', exact: true }),
  ).toBeVisible();
  expect(
    await page
      .locator('ol > li')
      .evaluateAll((items) => items.map((item) => item.getAttribute('value'))),
  ).toEqual(['1', '2']);
  expect(await page.locator('ol > li > p').allTextContents()).toEqual(
    edited.steps.map((step) => step.text),
  );

  await page
    .getByRole('button', { name: 'Удалить рецепт', exact: true })
    .click();
  const deleted = page.waitForResponse(
    (response) =>
      response.request().method() === 'DELETE' &&
      response.url().endsWith(`/api/recipes/${original.id}`),
  );
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  expect((await deleted).status()).toBe(204);
  await expect(page).toHaveURL('/recipes');
  await page.reload();
  await expect(page.getByText('У вас пока нет рецептов.')).toBeVisible();
  const missing = await page.request.get(`/api/recipes/${original.id}`, {
    headers: { Authorization: `Bearer ${owner.accessToken}` },
  });
  expect(missing.status()).toBe(404);
});

test('503 и потерянный успешный POST сохраняют форму, тело и ключ без второго рецепта', async ({
  page,
  request,
}) => {
  const { accessToken } = await registerAndLogin(page, request);
  await fillRecipe(page, 'Повторяемый рецепт', 'Смешать и подать.');
  const attempts: { key: string | undefined; body: string | null }[] = [];
  let original: Recipe | undefined;
  await page.route('**/api/recipes', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    attempts.push({
      key: route.request().headers()['idempotency-key'],
      body: route.request().postData(),
    });
    if (attempts.length === 1) {
      return route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({
          code: 'internal_error',
          message: 'Временный отказ сервера',
        }),
      });
    }
    const response = await route.fetch();
    expect(response.status()).toBe(201);
    if (attempts.length === 2) {
      original = await response.json();
      return route.abort('failed');
    }
    expect(await response.json()).toEqual(original);
    return route.fulfill({ response });
  });
  await page
    .getByRole('button', { name: 'Сохранить рецепт', exact: true })
    .click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(
    page.getByLabel('Название рецепта', { exact: true }),
  ).toHaveValue('Повторяемый рецепт');
  await expect(
    page.getByLabel('Название рецепта', { exact: true }),
  ).toBeDisabled();
  await page
    .getByRole('button', { name: 'Повторить сохранение', exact: true })
    .click();
  await expect(
    page.getByRole('button', { name: 'Повторить сохранение', exact: true }),
  ).toBeEnabled();
  expect(original).toBeDefined();
  await expect(page.getByLabel('Описание шага 1')).toHaveValue(
    'Смешать и подать.',
  );
  await expect(page.getByLabel('Описание шага 1')).toBeDisabled();
  await page
    .getByRole('button', { name: 'Повторить сохранение', exact: true })
    .click();
  await expect(page).toHaveURL(`/recipes/${original!.id}`);
  expect(attempts).toHaveLength(3);
  expect(attempts[0].key).toMatch(/^[A-Za-z0-9_-]{16,128}$/);
  expect(attempts).toEqual([attempts[0], attempts[0], attempts[0]]);
  await page.reload();
  const collection = await page.request.get('/api/recipes', {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  expect(collection.status()).toBe(200);
  const recipes: Recipe[] = await collection.json();
  expect(recipes.map(({ id }) => id)).toEqual([original!.id]);
});

test('лимиты формы и 413 собранного nginx не создают частичный рецепт', async ({
  page,
  request,
}) => {
  const { accessToken } = await registerAndLogin(page, request);
  await fillRecipe(page, 'Лимит', 'Я'.repeat(20001));
  let posts = 0;
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/recipes'))
      posts++;
  });
  await page
    .getByRole('button', { name: 'Сохранить рецепт', exact: true })
    .click();
  await expect(
    page.getByRole('group', { name: 'Шаг 1', exact: true }).getByRole('alert'),
  ).toHaveText('Описание шага: от 1 до 20000 символов');
  await expect(page.getByLabel('Описание шага 1')).toHaveValue(
    'Я'.repeat(20001),
  );
  await expect(page.getByLabel('Описание шага 1')).toBeEnabled();
  expect(posts).toBe(0);
  const body: SaveRecipeRequest = {
    title: 'Лимит',
    cookingTimeMinutes: 5,
    categoryCode: 'other',
    ingredients: [{ name: 'Вода', quantity: null, unit: '' }],
    steps: [{ text: 'x'.repeat(1048576) }],
  };
  const credentials: Record<string, string>[] = [
    {},
    { Authorization: `Bearer ${accessToken}` },
  ];
  for (const headers of credentials) {
    const oversized = await page.request.post('/api/recipes', {
      headers: {
        ...headers,
        Origin: appOrigin,
        'Idempotency-Key': randomUUID(),
      },
      data: body,
    });
    expect(oversized.status()).toBe(413);
    expect(oversized.headers()['cache-control']).toBe('no-store');
    expect((await oversized.json()).code).toBe('body_too_large');
  }
  const collection = await page.request.get('/api/recipes', {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  expect(collection.status()).toBe(200);
  expect(await collection.json()).toEqual([]);
  await page.getByLabel('Описание шага 1').fill('Сохранённый текст формы.');
  await page.route('**/api/recipes', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    await route.continue({ postData: JSON.stringify(body) });
  });
  const rejected = saving(page, 'POST');
  await page
    .getByRole('button', { name: 'Сохранить рецепт', exact: true })
    .click();
  expect((await rejected).status()).toBe(413);
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page).toHaveURL('/recipes/new');
  await expect(
    page.getByLabel('Название рецепта', { exact: true }),
  ).toHaveValue('Лимит');
  await expect(page.getByLabel('Описание шага 1')).toHaveValue(
    'Сохранённый текст формы.',
  );
  await expect(page.getByLabel('Описание шага 1')).toBeEnabled();
  const unchanged = await page.request.get('/api/recipes', {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  expect(unchanged.status()).toBe(200);
  expect(await unchanged.json()).toEqual([]);
});
