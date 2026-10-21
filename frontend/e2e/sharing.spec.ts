import { randomBytes } from 'node:crypto';
import { expect, test, type Page, type Route } from '@playwright/test';
import type { ShoppingItem, ShoppingList } from '../src/models/shoppingList';
import {
  appOrigin,
  createItem,
  createList,
  openList,
  registerAndLogin,
} from './helpers';

const importPath = '/api/shopping-list-shares/import';
const importRoute = `**${importPath}`;

async function share(page: Page, preserveMarks = false) {
  await page.bringToFront();
  await page.getByRole('button', { name: 'Поделиться', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Поделиться списком' });
  const checkbox = dialog.getByRole('checkbox');
  await expect(checkbox).not.toBeChecked();
  if (preserveMarks) await checkbox.locator('..').click();
  const response = page.waitForResponse((response) =>
    /\/api\/shopping-lists\/\d+\/shares$/.test(response.url()),
  );
  await dialog
    .getByRole('button', { name: 'Создать ссылку', exact: true })
    .click();
  expect((await response).status()).toBe(201);
  const url = await dialog.getByLabel('Ссылка на копию списка').inputValue();
  expect(new URL(url).pathname).toBe('/share');
  // Никогда не включать секрет в сообщения об ошибках теста.
  expect(/^#token=[A-Za-z0-9_-]{43}$/.test(new URL(url).hash)).toBe(true);
  await dialog.getByRole('button', { name: 'Закрыть', exact: true }).click();
  return url;
}

async function confirmImport(page: Page) {
  const response = page.waitForResponse((response) =>
    response.url().endsWith(importPath),
  );
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Добавить', exact: true })
    .click();
  const saved = await response;
  expect(saved.status()).toBe(201);
  expect(saved.headers()['cache-control']).toBe('no-store');
  const list: ShoppingList = await saved.json();
  await expect(page).toHaveURL(`${appOrigin}/lists/${list.id}`);
  return list;
}

async function lists(page: Page, accessToken: string): Promise<ShoppingList[]> {
  const response = await page.request.get('/api/shopping-lists', {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  expect(response.status()).toBe(200);
  return response.json();
}

async function items(
  page: Page,
  token: string,
  listId: string,
): Promise<ShoppingItem[]> {
  const response = await page.request.get(
    `/api/shopping-lists/${listId}/items`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  expect(response.status()).toBe(200);
  return response.json();
}

async function deleteList(page: Page, list: ShoppingList) {
  await page.goto('/lists');
  await page
    .getByRole('button', { name: `Удалить список ${list.name}`, exact: true })
    .click();
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Удалить', exact: true })
    .click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
}

test('снимки обоих режимов переживают правки и удаление оригинала; копии личные и независимые', async ({
  page,
  browser,
  request,
}) => {
  const author = await registerAndLogin(page, request);
  const original = await createList(page, 'Снимок на неделю');
  await openList(page, original);
  const milk = await createItem(page, original.id, 'Молоко', '0.001', 'л');
  await createItem(page, original.id, 'Запас', '999999999.999', 'шт');
  await createItem(page, original.id, 'Без количества');
  const marked = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/items/${milk.id}`) &&
      response.request().method() === 'PATCH',
  );
  await page
    .getByRole('checkbox', { name: 'Куплено: Молоко', exact: true })
    .locator('..')
    .click();
  expect((await marked).status()).toBe(200);
  const resetURL = await share(page);
  const preservedURL = await share(page, true);
  await expect(
    page.getByRole('checkbox', { name: 'Куплено: Молоко', exact: true }),
  ).toBeChecked();

  await page
    .getByRole('button', { name: 'Редактировать Молоко', exact: true })
    .click();
  await page
    .getByRole('dialog')
    .getByLabel('Название', { exact: true })
    .fill('После ссылки');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Сохранить', exact: true })
    .click();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await deleteList(page, original);

  const context = await browser.newContext({ baseURL: appOrigin });
  try {
    const recipient = await context.newPage();
    const account = await registerAndLogin(recipient, request);
    await recipient.goto('/profile');
    await recipient.getByRole('button', { name: 'Выйти из аккаунта' }).click();
    await expect(recipient).toHaveURL(/\/login$/);
    let imports = 0;
    recipient.on('request', (request) => {
      if (request.url().endsWith(importPath)) imports++;
    });
    await recipient.goto(resetURL);
    await recipient.getByLabel('Email', { exact: true }).fill(account.email);
    await recipient.getByLabel('Пароль').fill(account.password);
    await recipient.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(recipient.getByRole('dialog')).toBeVisible();
    expect(recipient.url() === resetURL).toBe(true);
    expect(imports).toBe(0);
    const reset = await confirmImport(recipient);
    expect(reset.name).toBe(original.name);
    expect(reset.id).not.toBe(original.id);
    await recipient.reload();
    const firstItems = await items(recipient, account.accessToken, reset.id);
    expect(
      firstItems.map(({ name, quantity, unit, isMarked }) => ({
        name,
        quantity,
        unit,
        isMarked,
      })),
    ).toEqual([
      { name: 'Молоко', quantity: '0.001', unit: 'л', isMarked: false },
      { name: 'Запас', quantity: '999999999.999', unit: 'шт', isMarked: false },
      { name: 'Без количества', quantity: null, unit: '', isMarked: false },
    ]);
    expect(firstItems[0].id).not.toBe(milk.id);
    await expect(
      recipient.getByText('999999999.999 шт', { exact: true }),
    ).toBeVisible();
    await recipient
      .getByRole('button', { name: 'Удалить Молоко', exact: true })
      .click();
    await expect(
      recipient.getByRole('heading', { name: 'Молоко', exact: true }),
    ).toHaveCount(0);
    await createItem(recipient, reset.id, 'Только в копии', '2.675', 'кг');
    await recipient
      .getByRole('button', {
        name: 'Редактировать Только в копии',
        exact: true,
      })
      .click();
    await recipient
      .getByRole('dialog')
      .getByLabel('Название', { exact: true })
      .fill('Изменено в копии');
    await recipient
      .getByRole('dialog')
      .getByRole('button', { name: 'Сохранить', exact: true })
      .click();
    await expect(recipient.getByRole('dialog')).not.toBeVisible();
    await recipient.reload();
    await expect(
      recipient.getByRole('heading', { name: 'Изменено в копии', exact: true }),
    ).toBeVisible();

    await recipient.goto(preservedURL);
    const preserved = await confirmImport(recipient);
    const secondItems = await items(
      recipient,
      account.accessToken,
      preserved.id,
    );
    expect(secondItems.map((item) => item.name)).toEqual([
      'Молоко',
      'Запас',
      'Без количества',
    ]);
    expect(secondItems.map((item) => item.isMarked)).toEqual([
      true,
      false,
      false,
    ]);
    expect(secondItems[0].id).not.toBe(firstItems[0].id);
    await expect(
      recipient.getByRole('checkbox', { name: 'Куплено: Молоко', exact: true }),
    ).toBeChecked();
    for (const method of ['get', 'patch', 'delete'] as const) {
      const denied = await page.request[method](
        `/api/shopping-lists/${preserved.id}`,
        {
          headers: { Authorization: `Bearer ${author.accessToken}` },
          ...(method === 'patch' ? { data: { name: 'Чужая правка' } } : {}),
        },
      );
      expect(denied.status()).toBe(404);
    }
    await recipient.goto(resetURL);
    const repeated = await confirmImport(recipient);
    expect(repeated.id).not.toBe(reset.id);
    expect(
      (await items(recipient, account.accessToken, repeated.id)).map(
        (item) => item.name,
      ),
    ).toEqual(['Молоко', 'Запас', 'Без количества']);
  } finally {
    await context.close();
  }
});

test('регистрация в новой вкладке, отмена и reload не импортируют; новые подтверждения и удаление разрешают копии', async ({
  page,
  browser,
  request,
}) => {
  await registerAndLogin(page, request);
  const original = await createList(page, 'Пустой снимок');
  await openList(page, original);
  const url = await share(page);
  const context = await browser.newContext({ baseURL: appOrigin });
  try {
    const recipient = await context.newPage();
    let imports = 0;
    context.on('request', (request) => {
      if (request.url().endsWith(importPath)) imports++;
    });
    await recipient.goto(url);
    const popup = recipient.waitForEvent('popup');
    await recipient
      .getByRole('link', { name: 'Зарегистрироваться в новой вкладке' })
      .click();
    const registration = await popup;
    await expect(registration).toHaveURL(/\/registration$/);
    const account = await registerAndLogin(registration, request);
    // Пользователь возвращается из открытой вкладки регистрации к ссылке.
    await recipient.bringToFront();
    await expect(recipient.getByRole('dialog')).toBeVisible();
    expect(recipient.url() === url).toBe(true);
    expect(imports).toBe(0);
    await recipient.reload();
    await expect(recipient.getByRole('dialog')).toBeVisible();
    expect(imports).toBe(0);
    await recipient
      .getByRole('dialog')
      .getByRole('button', { name: 'Отмена', exact: true })
      .click();
    await expect(recipient).toHaveURL(/\/lists$/);
    expect(await lists(recipient, account.accessToken)).toEqual([]);
    expect(imports).toBe(0);
    await registration.close();

    await recipient.goto(url);
    const first = await confirmImport(recipient);
    await expect(
      recipient.getByText('Список покупок пока пуст.'),
    ).toBeVisible();
    await deleteList(recipient, first);
    await recipient.goto(url);
    const afterDelete = await confirmImport(recipient);
    expect(afterDelete.id).not.toBe(first.id);
    // Оба подтверждения принадлежат одному пользователю, но разным действиям.
    const sibling = await context.newPage();
    await Promise.all([recipient.goto(url), sibling.goto(url)]);
    for (const current of [recipient, sibling])
      await expect(current.getByRole('dialog')).toBeVisible();
    expect((await lists(recipient, account.accessToken)).length).toBe(1);
    const [left, right] = await Promise.all([
      confirmImport(recipient),
      confirmImport(sibling),
    ]);
    expect(new Set([afterDelete.id, left.id, right.id]).size).toBe(3);
    expect((await lists(recipient, account.accessToken)).length).toBe(3);
  } finally {
    await context.close();
  }
});

type Attempt = {
  key: string | undefined;
  body: string | null;
  authorization: string | undefined;
};
function attempt(route: Route): Attempt {
  return {
    key: route.request().headers()['idempotency-key'],
    body: route.request().postData(),
    authorization: route.request().headers().authorization,
  };
}

test('потеря реальных ответов, 500 и 401 с refresh сохраняют действие и не создают дублей', async ({
  page,
  request,
}) => {
  const account = await registerAndLogin(page, request);
  const original = await createList(page, 'Проверка обмена');
  await openList(page, original);
  const endpoint = `**/api/shopping-lists/${original.id}/shares`;
  const creates: Attempt[] = [];
  const shareIds: string[] = [];
  await page.route(endpoint, async (route) => {
    creates.push(attempt(route));
    if (creates.length === 1) {
      await route.fulfill({
        status: 500,
        json: { code: 'internal_error', message: 'Тестовый временный отказ' },
      });
    } else {
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      shareIds.push((await response.json()).id);
      if (creates.length === 2) await route.abort('failed');
      else await route.fulfill({ response });
    }
  });
  await page.getByRole('button', { name: 'Поделиться', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog
    .getByRole('button', { name: 'Создать ссылку', exact: true })
    .click();
  await expect(dialog.getByRole('alert')).toHaveText(
    'Тестовый временный отказ',
  );
  await expect(dialog.getByLabel('Ссылка на копию списка')).toHaveCount(0);
  await expect(dialog.getByRole('checkbox')).toBeDisabled();
  await dialog.getByRole('button', { name: 'Повторить', exact: true }).click();
  await expect(dialog.getByRole('alert')).toBeVisible();
  await expect(
    dialog.getByRole('button', { name: 'Повторить', exact: true }),
  ).toBeEnabled();
  await dialog.getByRole('button', { name: 'Повторить', exact: true }).click();
  const url = await dialog.getByLabel('Ссылка на копию списка').inputValue();
  expect(creates.length).toBe(3);
  expect(
    creates.every(
      (entry) => entry.key === creates[0].key && entry.body === creates[0].body,
    ),
  ).toBe(true);
  expect(shareIds[1]).toBe(shareIds[0]);
  await page.unroute(endpoint);
  await dialog.getByRole('button', { name: 'Закрыть', exact: true }).click();

  const imports: Attempt[] = [];
  const ids: string[] = [];
  let refreshes = 0;
  page.on('response', (response) => {
    if (response.url().endsWith('/api/refresh') && response.status() === 200)
      refreshes++;
  });
  await page.route(importRoute, async (route) => {
    imports.push(attempt(route));
    if (imports.length === 1) {
      await route.fulfill({
        status: 401,
        json: { code: 'unauthorized', message: 'Тестовый срок access' },
      });
    } else if (imports.length === 3) {
      await route.fulfill({
        status: 500,
        json: { code: 'internal_error', message: 'Тестовый временный отказ' },
      });
    } else {
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      ids.push((await response.json()).id);
      if (imports.length === 2) await route.abort('failed');
      else await route.fulfill({ response });
    }
  });
  await page.goto(url);
  await expect(page.getByRole('dialog')).toBeVisible();
  const refreshesBefore = refreshes;
  const refreshing = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/refresh') && response.status() === 200,
  );
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Добавить', exact: true })
    .click();
  await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();
  expect(refreshes).toBe(refreshesBefore + 1);
  const refreshed = await (await refreshing).json();
  expect(imports[1].authorization === `Bearer ${refreshed.accessToken}`).toBe(
    true,
  );
  expect((await lists(page, account.accessToken)).length).toBe(2);
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Повторить', exact: true })
    .click();
  await expect(page.getByRole('dialog').getByRole('alert')).toHaveText(
    'Тестовый временный отказ',
  );
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Повторить', exact: true })
    .click();
  await expect(page).toHaveURL(`${appOrigin}/lists/${ids[0]}`);
  expect(ids[1]).toBe(ids[0]);
  expect(imports.length).toBe(4);
  expect(
    imports.every(
      (entry) => entry.key === imports[0].key && entry.body === imports[0].body,
    ),
  ).toBe(true);
  await page.reload();
  expect((await lists(page, account.accessToken)).length).toBe(2);
});

test('смена аккаунта в соседней вкладке отменяет прежнее согласие и поздний переход', async ({
  page,
  browser,
  request,
}) => {
  const first = await registerAndLogin(page, request);
  const original = await createList(page, 'Смена получателя');
  await openList(page, original);
  const url = await share(page);
  const otherContext = await browser.newContext({ baseURL: appOrigin });
  const sibling = await page.context().newPage();
  let release!: () => void;
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  let saved!: () => void;
  const persisted = new Promise<void>((resolve) => {
    saved = resolve;
  });
  try {
    const second = await registerAndLogin(
      await otherContext.newPage(),
      request,
    );
    let imports = 0;
    await page.route(importRoute, async (route) => {
      imports++;
      if (imports === 1) {
        const response = await route.fetch();
        expect(response.status()).toBe(201);
        saved();
        await held;
        await route.fulfill({ response });
      } else await route.continue();
    });
    await page.goto(url);
    await page
      .getByRole('dialog')
      .getByRole('button', { name: 'Добавить', exact: true })
      .click();
    await persisted;
    await sibling.goto('/login');
    await sibling.getByLabel('Email', { exact: true }).fill(second.email);
    await sibling.getByLabel('Пароль').fill(second.password);
    await sibling.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(sibling).toHaveURL(/\/lists$/);
    await expect(
      page
        .getByRole('dialog')
        .getByRole('button', { name: 'Добавить', exact: true }),
    ).toBeEnabled();
    expect(imports).toBe(1);
    expect((await lists(page, second.accessToken)).length).toBe(0);
    const lateResponse = page.waitForResponse((response) =>
      response.url().endsWith(importPath),
    );
    release();
    // Старое действие уже отвергнуто координатором до чтения JSON.
    // После доставки ответа проверяем интерфейс, а не завершение его чтения.
    await lateResponse;
    await expect(
      page
        .getByRole('dialog')
        .getByRole('button', { name: 'Добавить', exact: true }),
    ).toBeEnabled();
    expect(new URL(page.url()).pathname).toBe('/share');
    expect((await lists(page, first.accessToken)).length).toBe(2);
    await confirmImport(page);
    expect((await lists(page, second.accessToken)).length).toBe(1);
    expect(imports).toBe(2);
  } finally {
    release();
    await sibling.close();
    await otherContext.close();
  }
});

test('повреждённая и неизвестная ссылки не создают списков', async ({
  page,
  request,
}) => {
  const account = await registerAndLogin(page, request);
  let imports = 0;
  page.on('request', (request) => {
    if (request.url().endsWith(importPath)) imports++;
  });
  await page.goto('/share#token=damaged');
  await expect(page.getByRole('alert')).toContainText('Некорректная ссылка');
  expect(imports).toBe(0);
  const token = randomBytes(32).toString('base64url');
  await page.goto(`/share#token=${token}`);
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(imports).toBe(0);
  const response = page.waitForResponse((response) =>
    response.url().endsWith(importPath),
  );
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Добавить', exact: true })
    .click();
  expect((await response).status()).toBe(404);
  await expect(page.getByRole('alert')).toHaveText(
    'Ссылка недоступна. Проверьте её у отправителя.',
  );
  expect(await lists(page, account.accessToken)).toEqual([]);
});
