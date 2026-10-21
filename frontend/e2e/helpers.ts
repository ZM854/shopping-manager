import { randomUUID } from 'node:crypto';
import { expect, type APIRequestContext, type Page } from '@playwright/test';
import type { ShoppingItem, ShoppingList } from '../src/models/shoppingList';

export const appOrigin = 'http://127.0.0.1:55173';
export const mailOrigin = 'http://127.0.0.1:58025';

export async function activationPathFor(
  request: APIRequestContext,
  email: string,
) {
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

export async function registerAndLogin(page: Page, request: APIRequestContext) {
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

export async function createList(page: Page, name: string) {
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

export async function openList(page: Page, list: ShoppingList) {
  await page.goto(`/lists/${list.id}`);
  await expect(
    page.getByRole('heading', { name: list.name, exact: true }),
  ).toBeVisible();
}

export async function createItem(
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
