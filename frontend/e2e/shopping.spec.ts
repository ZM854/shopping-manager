import { randomUUID } from 'node:crypto';
import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from '@playwright/test';
import type { ShoppingItem, ShoppingList } from '../src/models/shoppingList';

const appOrigin = 'http://127.0.0.1:55173';
const mailOrigin = 'http://127.0.0.1:58025';

async function registerAndLogin(page: Page, request: APIRequestContext) {
  const suffix = randomUUID();
  const email = `e2e-${suffix}@example.test`;
  const password = `e2e-only-${randomUUID()}`;
  await page.goto('/registration');
  await page.getByLabel('Имя', { exact: true }).fill('Проверка');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Пароль').fill(password);
  const registration = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/registration') &&
      response.request().method() === 'POST',
  );
  await page
    .getByRole('button', { name: 'Зарегистрироваться', exact: true })
    .click();
  expect((await registration).status()).toBe(201);
  await expect(page).toHaveURL(/\/login$/);

  // Mailpit принадлежит только E2E Compose, письма не отправляются наружу.
  let messageId = '';
  await expect
    .poll(async () => {
      const response = await request.get(`${mailOrigin}/api/v1/messages`);
      expect(response.status()).toBe(200);
      const inbox: { messages: { ID: string; To: { Address: string }[] }[] } =
        await response.json();
      messageId =
        inbox.messages.find((message) =>
          message.To.some((to) => to.Address === email),
        )?.ID ?? '';
      return messageId !== '';
    })
    .toBe(true);
  const message = await request.get(
    `${mailOrigin}/api/v1/message/${messageId}`,
  );
  const content: { Text: string; HTML: string } = await message.json();
  const activationPath = `${content.Text}\n${content.HTML}`.match(
    /\/api\/activate\/[A-Za-z0-9_-]+/,
  )?.[0];
  expect(activationPath).toBeDefined();
  const activated = await request.get(`${appOrigin}${activationPath}`, {
    maxRedirects: 0,
  });
  expect(activated.status()).toBe(302);

  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Пароль').fill(password);
  const login = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/login') &&
      response.request().method() === 'POST',
  );
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  const response = await login;
  expect(response.status()).toBe(200);
  const session: { accessToken: string } = await response.json();
  await expect(page).toHaveURL(/\/lists$/);
  await expect(page.getByText('У вас пока нет списков покупок.')).toBeVisible();
  return session.accessToken;
}

async function createList(page: Page, name: string) {
  await page
    .getByRole('button', { name: 'Создать список', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Название списка').fill(name);
  const created = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/shopping-lists') &&
      response.request().method() === 'POST',
  );
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  const response = await created;
  expect(response.status()).toBe(201);
  const list: ShoppingList = await response.json();
  expect(typeof list.id).toBe('string');
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('link', { name, exact: true })).toBeVisible();
  return list;
}

async function openList(page: Page, list: ShoppingList) {
  await page.goto(`/lists/${list.id}`);
  await expect(
    page.getByRole('heading', { name: list.name, exact: true }),
  ).toBeVisible();
}

async function createItem(
  page: Page,
  listId: string,
  name: string,
  quantity = '',
  unit = '',
) {
  await page
    .getByRole('button', { name: 'Добавить товар', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Название', { exact: true }).fill(name);
  await dialog.getByLabel('Количество', { exact: true }).fill(quantity);
  await dialog.getByLabel('Единица измерения').fill(unit);
  const created = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/api/shopping-lists/${listId}/items`) &&
      response.request().method() === 'POST',
  );
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  const response = await created;
  expect(response.status()).toBe(201);
  const item: ShoppingItem = await response.json();
  expect(typeof item.id).toBe('string');
  expect(item.quantity === null || typeof item.quantity === 'string').toBe(
    true,
  );
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  return item;
}

test('два пользователя: несколько списков, CRUD, отметки, очистка, изоляция и перезагрузка', async ({
  page,
  browser,
  request,
}) => {
  const token = await registerAndLogin(page, request);
  const home = await createList(page, 'Дом');
  const trip = await createList(page, 'Поездка');
  await openList(page, home);
  const milk = await createItem(page, home.id, 'Молоко', '0.125', 'л');
  const maximum = await createItem(
    page,
    home.id,
    'Запас',
    '999999999.999',
    'шт',
  );
  expect(milk.quantity).toBe('0.125');
  expect(maximum.quantity).toBe('999999999.999');

  const marked = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/items/${milk.id}`) &&
      response.request().method() === 'PATCH',
  );
  await page
    .getByRole('checkbox', { name: 'Куплено: Молоко', exact: true })
    .locator('..')
    .click();
  const markResponse = await marked;
  expect(markResponse.status()).toBe(200);
  expect(markResponse.request().postDataJSON()).toEqual({ isMarked: true });
  await page
    .getByRole('button', { name: 'Редактировать Молоко', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Название', { exact: true }).fill('Кефир');
  await dialog.getByLabel('Количество', { exact: true }).fill('');
  const changed = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/items/${milk.id}`) &&
      response.request().method() === 'PATCH',
  );
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  const changedResponse = await changed;
  expect(changedResponse.request().postDataJSON()).toEqual({
    name: 'Кефир',
    quantity: null,
  });
  await expect(dialog).not.toBeVisible();
  await page.reload();
  await expect(
    page.getByRole('checkbox', { name: 'Куплено: Кефир', exact: true }),
  ).toBeChecked();
  await expect(
    page.getByText('999999999.999 шт', { exact: true }),
  ).toBeVisible();

  await page
    .getByRole('button', { name: 'Удалить Запас', exact: true })
    .click();
  await expect(
    page.getByRole('heading', { name: 'Запас', exact: true }),
  ).not.toBeVisible();
  await page
    .getByRole('button', { name: 'Снять все отметки', exact: true })
    .click();
  await expect(
    page.getByRole('checkbox', { name: 'Куплено: Кефир', exact: true }),
  ).not.toBeChecked();
  await page
    .getByRole('button', { name: 'Вычеркнуть всё', exact: true })
    .click();
  await expect(
    page.getByRole('checkbox', { name: 'Куплено: Кефир', exact: true }),
  ).toBeChecked();
  await openList(page, trip);
  const bread = await createItem(page, trip.id, 'Хлеб');
  expect(bread.quantity).toBeNull();
  await openList(page, home);
  for (let repeat = 0; repeat < 2; repeat++) {
    await page
      .getByRole('button', { name: 'Очистить список', exact: true })
      .click();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Удалить', exact: true })
      .click();
    await expect(page.getByRole('dialog')).not.toBeVisible();
    await expect(page.getByText('Список покупок пока пуст.')).toBeVisible();
  }

  const otherContext = await browser.newContext({ baseURL: appOrigin });
  try {
    const otherPage = await otherContext.newPage();
    const otherToken = await registerAndLogin(otherPage, request);
    const own = await createList(otherPage, 'Личный список');
    await openList(otherPage, own);
    await createItem(otherPage, own.id, 'Чай', '1');
    const headers = { Authorization: `Bearer ${otherToken}` };
    for (const path of [
      `/api/shopping-lists/${home.id}`,
      `/api/shopping-lists/${trip.id}/items`,
      `/api/shopping-lists/${trip.id}/items/${bread.id}`,
    ]) {
      const denied = await otherPage.request.get(path, { headers });
      expect(denied.status()).toBe(404);
    }
    const deniedPatch = await otherPage.request.patch(
      `/api/shopping-lists/${trip.id}/items/${bread.id}`,
      {
        headers,
        data: { name: 'Подмена' },
      },
    );
    expect(deniedPatch.status()).toBe(404);
    const deniedClear = await otherPage.request.delete(
      `/api/shopping-lists/${trip.id}/items`,
      { headers },
    );
    expect(deniedClear.status()).toBe(404);
    const wrongList = await page.request.get(
      `/api/shopping-lists/${home.id}/items/${bread.id}`,
      {
        headers: { Authorization: `Bearer ${token}` },
      },
    );
    expect(wrongList.status()).toBe(404);
    await otherPage.reload();
    await expect(
      otherPage.getByRole('heading', { name: 'Чай', exact: true }),
    ).toBeVisible();
  } finally {
    await otherContext.close();
  }
  await openList(page, trip);
  await expect(
    page.getByRole('heading', { name: 'Хлеб', exact: true }),
  ).toBeVisible();
  await page.getByRole('link', { name: 'Все списки', exact: true }).click();
  await page
    .getByRole('button', { name: 'Переименовать Дом', exact: true })
    .click();
  await page
    .getByRole('dialog')
    .getByLabel('Название списка')
    .fill('На неделю');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Сохранить', exact: true })
    .click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await page
    .getByRole('button', { name: 'Удалить список На неделю', exact: true })
    .click();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Поездка', exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(
    page.getByRole('link', { name: 'На неделю', exact: true }),
  ).not.toBeVisible();
  await expect(
    page.getByRole('link', { name: 'Поездка', exact: true }),
  ).toBeVisible();
});

test('потерянный ответ POST повторяется с прежним ключом; ошибка очистки не изображает успех', async ({
  page,
  request,
}) => {
  const token = await registerAndLogin(page, request);
  const list = await createList(page, 'Проверка повторов');
  await openList(page, list);
  const endpoint = `**/api/shopping-lists/${list.id}/items`;
  const attempts: { key: string | undefined; body: string | null }[] = [];
  await page.route(endpoint, async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue();
      return;
    }
    attempts.push({
      key: route.request().headers()['idempotency-key'],
      body: route.request().postData(),
    });
    if (attempts.length === 1) {
      // Реальный сервер сохраняет покупку; браузер теряет только ответ.
      const saved = await route.fetch();
      expect(saved.status()).toBe(201);
      await route.abort('failed');
    } else {
      await route.continue();
    }
  });
  await page
    .getByRole('button', { name: 'Добавить товар', exact: true })
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Название', { exact: true }).fill('Сыр');
  await dialog.getByLabel('Количество', { exact: true }).fill('1.000');
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(dialog.getByLabel('Название', { exact: true })).toBeDisabled();
  const persisted = await page.request.get(
    `/api/shopping-lists/${list.id}/items`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  expect((await persisted.json()).length).toBe(1);
  await dialog.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(attempts).toHaveLength(2);
  expect(attempts[0].key).toMatch(/^[A-Za-z0-9_-]{16,128}$/);
  expect(attempts[1]).toEqual(attempts[0]);
  await page.unroute(endpoint);
  await page.reload();
  await expect(
    page.getByRole('heading', { name: 'Сыр', exact: true }),
  ).toHaveCount(1);

  await page.route(endpoint, async (route) => {
    if (route.request().method() === 'DELETE') {
      await route.fulfill({
        status: 500,
        json: { code: 'internal_error', message: 'Тестовая ошибка очистки' },
      });
    } else {
      await route.continue();
    }
  });
  await page
    .getByRole('button', { name: 'Очистить список', exact: true })
    .click();
  const confirmation = page.getByRole('dialog');
  await confirmation
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  await expect(confirmation.getByRole('alert')).toHaveText(
    'Тестовая ошибка очистки',
  );
  await expect(
    page.getByRole('heading', { name: 'Сыр', exact: true }),
  ).toBeVisible();
  await page.unroute(endpoint);
  await confirmation
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  await expect(confirmation).not.toBeVisible();
  await expect(page.getByText('Список покупок пока пуст.')).toBeVisible();
});
