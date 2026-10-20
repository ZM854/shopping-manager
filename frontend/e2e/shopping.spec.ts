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

async function activationPathFor(request: APIRequestContext, email: string) {
  let messageId = '';
  await expect
    .poll(async () => {
      const response = await request.get(`${mailOrigin}/api/v1/messages`);
      const inbox: { messages: { ID: string; To: { Address: string }[] }[] } =
        await response.json();
      messageId =
        inbox.messages.find((message) =>
          message.To.some((to) => to.Address === email),
        )?.ID ?? '';
      return messageId;
    })
    .not.toBe('');
  const response = await request.get(
    `${mailOrigin}/api/v1/message/${messageId}`,
  );
  const content: { Text: string; HTML: string } = await response.json();
  const path = `${content.Text}\n${content.HTML}`.match(
    /\/activation\/[A-Za-z0-9_-]+/,
  )?.[0];
  if (!path) throw new Error('В тестовом письме нет ссылки подтверждения');
  return path;
}

test('повторное письмо заменяет ссылку; подтверждение требует кнопки и одноразово', async ({
  page,
  request,
}) => {
  const email = `resend-${randomUUID()}@example.test`;
  const registration = await request.post(`${appOrigin}/api/registration`, {
    data: { name: 'Проверка', email, password: `test-${randomUUID()}` },
  });
  expect(registration.status()).toBe(201);
  const originalPath = await activationPathFor(request, email);
  await page.goto('/login');
  const resendForm = page.getByRole('form', { name: 'Повторное письмо' });
  await resendForm.getByLabel('Почта для подтверждения').fill(email);
  await expect
    .poll(
      async () => {
        const response = page.waitForResponse((response) =>
          response.url().endsWith('/api/activation/resend'),
        );
        await resendForm
          .getByRole('button', { name: 'Отправить письмо повторно' })
          .click();
        return (await response).status();
      },
      { timeout: 10000 },
    )
    .toBe(202);
  const newPath = await activationPathFor(request, email);
  expect(newPath).not.toBe(originalPath);
  for (const [path, status] of [
    [originalPath, 400],
    [newPath, 204],
    [newPath, 400],
  ] as const) {
    await page.goto(path);
    await expect(page).toHaveURL(/\/activation$/);
    expect(
      (await page.context().cookies()).some(
        (cookie) => cookie.name === 'refreshToken',
      ),
    ).toBe(false);
    const response = page.waitForResponse((response) =>
      response.url().endsWith('/api/activation'),
    );
    await page
      .getByRole('button', { name: 'Подтвердить почту', exact: true })
      .click();
    expect((await response).status()).toBe(status);
    if (status === 204)
      await expect(page.getByRole('status')).toContainText(
        'Почта подтверждена',
      );
    else
      await expect(page.getByRole('alert')).toContainText(
        'Ссылка недействительна',
      );
  }
  expect(
    (await request.get(`${appOrigin}/api/activate/removed-route`)).status(),
  ).toBe(404);
});

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
  const registered = await registration;
  expect(registered.status()).toBe(201);
  expect(await registered.json()).toEqual({ activationRequired: true });
  expect(
    (await page.context().cookies()).some(
      (cookie) => cookie.name === 'refreshToken',
    ),
  ).toBe(false);
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Пароль').fill(password);
  const inactiveLogin = page.waitForResponse((response) =>
    response.url().endsWith('/api/login'),
  );
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  expect((await inactiveLogin).status()).toBe(403);
  await expect(page.getByRole('alert')).toHaveText(
    'Подтвердите email перед входом',
  );
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
    /\/activation\/[A-Za-z0-9_-]+/,
  )?.[0];
  expect(activationPath).toBeDefined();
  await page.goto(`${appOrigin}${activationPath}`);
  await expect(page).toHaveURL(/\/activation$/);
  const activation = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/activation') &&
      response.request().method() === 'POST',
  );
  await page
    .getByRole('button', { name: 'Подтвердить почту', exact: true })
    .click();
  expect((await activation).status()).toBe(204);
  await expect(page.getByRole('status')).toContainText('Почта подтверждена');
  await page.getByRole('link', { name: 'Перейти ко входу' }).click();
  await page.getByLabel('Email', { exact: true }).fill(email);

  await page.getByLabel('Пароль').fill('incorrect-test-password');
  const rejectedLogin = page.waitForResponse((response) =>
    response.url().endsWith('/api/login'),
  );
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  expect((await rejectedLogin).status()).toBe(401);
  await expect(page.getByRole('alert')).toHaveText('Неверный email или пароль');
  await expect(page).toHaveURL(/\/login$/);

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
  expect(response.headers()['cache-control']).toBe('no-store');
  const session: { accessToken: string; user: { id: string } } =
    await response.json();
  expect(typeof session.user.id).toBe('string');
  expect(session.user.id).toMatch(/^[1-9][0-9]*$/);
  expect(Object.keys(session).sort()).toEqual(['accessToken', 'user']);
  await expect(page).toHaveURL(/\/lists$/);
  await expect(page.getByText('У вас пока нет списков покупок.')).toBeVisible();
  return { accessToken: session.accessToken, email, password };
}

test('два браузера одного пользователя сохраняют независимые сессии после выхода', async ({
  page,
  request,
  browser,
}) => {
  const { email, password } = await registerAndLogin(page, request);
  await createList(page, 'Общий аккаунт');
  const otherContext = await browser.newContext({ baseURL: appOrigin });
  try {
    const otherPage = await otherContext.newPage();
    await otherPage.goto('/login');
    await otherPage.getByLabel('Email', { exact: true }).fill(email);
    await otherPage.getByLabel('Пароль').fill(password);
    await otherPage.getByRole('button', { name: 'Войти', exact: true }).click();
    await expect(otherPage).toHaveURL(/\/lists$/);
    const firstCookie = (await page.context().cookies()).find(
      (cookie) => cookie.name === 'refreshToken',
    );
    const otherCookie = (await otherContext.cookies()).find(
      (cookie) => cookie.name === 'refreshToken',
    );
    expect(
      Boolean(
        firstCookie && otherCookie && firstCookie.value !== otherCookie.value,
      ),
    ).toBe(true);
    expect(otherCookie?.httpOnly).toBe(true);
    expect(otherCookie?.sameSite).toBe('Lax');

    for (const current of [page, otherPage]) {
      await current.reload();
      await expect(
        current.getByRole('link', { name: 'Общий аккаунт', exact: true }),
      ).toBeVisible();
    }
    await page.goto('/profile');
    const logout = page.waitForResponse((response) =>
      response.url().endsWith('/api/logout'),
    );
    await page.getByRole('button', { name: 'Выйти из аккаунта' }).click();
    expect((await logout).status()).toBe(200);
    expect(
      (await page.context().cookies()).some(
        (cookie) => cookie.name === 'refreshToken',
      ),
    ).toBe(false);
    await page.goto('/lists');
    await expect(page).toHaveURL(/\/login$/);
    await otherPage.reload();
    await expect(
      otherPage.getByRole('link', { name: 'Общий аккаунт', exact: true }),
    ).toBeVisible();
  } finally {
    await otherContext.close();
  }
});

test('вкладки с общей cookie согласуют refresh и выход во время задержанного refresh', async ({
  page,
  request,
}) => {
  await registerAndLogin(page, request);
  await createList(page, 'Общие вкладки');
  const sibling = await page.context().newPage();
  try {
    await sibling.goto('/lists');
    await expect(
      sibling.getByRole('link', { name: 'Общие вкладки', exact: true }),
    ).toBeVisible();
    const refreshStatuses: number[] = [];
    const observeRefresh = (response: import('@playwright/test').Response) => {
      if (response.url().endsWith('/api/refresh'))
        refreshStatuses.push(response.status());
    };
    page.on('response', observeRefresh);
    sibling.on('response', observeRefresh);
    await Promise.all([page.reload(), sibling.reload()]);
    for (const current of [page, sibling]) {
      await expect(
        current.getByRole('link', { name: 'Общие вкладки', exact: true }),
      ).toBeVisible();
    }
    expect(refreshStatuses).toEqual([200, 200]);
    await sibling.goto('/profile');
    await expect(
      sibling.getByRole('button', { name: 'Выйти из аккаунта' }),
    ).toBeVisible();

    let release!: () => void;
    let started!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const refreshing = new Promise<void>((resolve) => {
      started = resolve;
    });
    await page.route('**/api/refresh', async (route) => {
      const response = await route.fetch();
      started();
      await held;
      await route.fulfill({ response });
    });
    let logoutRequests = 0;
    sibling.on('request', (request) => {
      if (request.url().endsWith('/api/logout')) logoutRequests++;
    });
    const reloading = page.reload();
    try {
      await refreshing;
      await sibling.getByRole('button', { name: 'Выйти из аккаунта' }).click();
      await expect(
        sibling.getByRole('button', { name: 'Выходим…' }),
      ).toBeDisabled();
      expect(logoutRequests).toBe(0);
    } finally {
      release();
    }
    await reloading;
    await expect(sibling).toHaveURL(/\/login$/);
    await expect(page).toHaveURL(/\/login$/);
    expect(logoutRequests).toBe(1);
    expect(
      (await page.context().cookies()).some(
        (cookie) => cookie.name === 'refreshToken',
      ),
    ).toBe(false);
    // Persistent browser state contains only public change markers.
    const persistent = await page.evaluate(() => ({
      local: { ...localStorage },
      session: { ...sessionStorage },
    }));
    expect(Object.keys(persistent.local)).toEqual([
      'shopping-manager.session-revision',
    ]);
    expect(persistent.session).toEqual({});
    expect(
      JSON.parse(persistent.local['shopping-manager.session-revision']).kind,
    ).toBe('logout');
    await Promise.all([page.reload(), sibling.reload()]);
    await expect(page).toHaveURL(/\/login$/);
    await expect(sibling).toHaveURL(/\/login$/);
  } finally {
    await sibling.close();
  }
});

test('смена аккаунта в одной вкладке сбрасывает списки и открытую форму другой', async ({
  page,
  request,
  browser,
}) => {
  await registerAndLogin(page, request);
  await createList(page, 'Первый аккаунт');
  const sibling = await page.context().newPage();
  const otherContext = await browser.newContext({ baseURL: appOrigin });
  try {
    const otherPage = await otherContext.newPage();
    const second = await registerAndLogin(otherPage, request);
    await createList(otherPage, 'Второй аккаунт');
    await sibling.goto('/lists');
    await expect(
      sibling.getByRole('link', { name: 'Первый аккаунт', exact: true }),
    ).toBeVisible();
    await sibling
      .getByRole('button', { name: 'Создать список', exact: true })
      .click();
    await sibling
      .getByRole('dialog')
      .getByLabel('Название списка')
      .fill('Личный черновик');
    await page.goto('/login');
    await page.getByLabel('Email', { exact: true }).fill(second.email);
    await page.getByLabel('Пароль').fill(second.password);
    await page.getByRole('button', { name: 'Войти', exact: true }).click();
    for (const current of [page, sibling]) {
      await expect(current).toHaveURL(/\/lists$/);
      await expect(
        current.getByRole('link', { name: 'Второй аккаунт', exact: true }),
      ).toBeVisible();
      await expect(
        current.getByRole('link', { name: 'Первый аккаунт', exact: true }),
      ).toHaveCount(0);
    }
    await expect(sibling.getByRole('dialog')).toHaveCount(0);
    await sibling
      .getByRole('button', { name: 'Создать список', exact: true })
      .click();
    await expect(
      sibling.getByRole('dialog').getByLabel('Название списка'),
    ).toHaveValue('');
  } finally {
    await sibling.close();
    await otherContext.close();
  }
});

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
  const { accessToken: token } = await registerAndLogin(page, request);
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
    const { accessToken: otherToken } = await registerAndLogin(
      otherPage,
      request,
    );
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
  const { accessToken: token } = await registerAndLogin(page, request);
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
