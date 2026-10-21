import { StrictMode } from 'react';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../context/authContext/AuthProvider';
import SharedShoppingListPage from '../pages/SharedShoppingListPage/SharedShoppingListPage';
import ShoppingListPage from '../pages/ShoppingListPage/ShoppingListPage';
import ShoppingListsPage from '../pages/ShoppingListsPage/ShoppingListsPage';
import Layout from '../layout/Layout';
import RequireAuth from '../router/RequireAuth';
import { sessionCoordinator } from '../services/session';
import { logger } from '../shared/logger';
import { isShareToken } from '../shared/shoppingSharing';

vi.mock('../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), error: vi.fn() },
}));

const token = 'A'.repeat(43);
const sourceId = '9007199254740993';
const copyId = '9007199254740994';
const metadata = (id: string) => ({
  id,
  name: 'Покупки',
  createdAt: '2026-10-21T10:00:00Z',
  updatedAt: '2026-10-21T10:00:00Z',
});
const session = (id = '1') => ({
  user: {
    id,
    name: 'Пользователь ' + id,
    email: `user${id}@example.test`,
    isEmailVerified: true,
  },
  accessToken: `access-secret-${id}`,
});
const fetchMock = vi.fn<typeof fetch>();
let refreshUser: string | null;
let accessVersion: number;
let sharedRequests: ReturnType<typeof deferred<Response>> | null;
let routers: ReturnType<typeof createMemoryRouter>[];

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function failure(status: number, code: string) {
  return Response.json({ code, message: 'Запрос не выполнен' }, { status });
}

async function respond(
  url: string | URL | Request,
  options?: RequestInit,
): Promise<Response> {
  const path = String(url);
  if (path.endsWith('/refresh')) {
    return refreshUser
      ? Response.json({
          ...session(refreshUser),
          accessToken: `access-secret-${refreshUser}-${++accessVersion}`,
        })
      : failure(401, 'unauthorized');
  }
  if (path.endsWith('/login')) {
    const { email } = JSON.parse(String(options?.body));
    refreshUser = email.startsWith('user2') ? '2' : '1';
    return Response.json(session(refreshUser));
  }
  if (path.endsWith('/logout')) {
    refreshUser = null;
    return new Response(null, { status: 204 });
  }
  if (path.endsWith('/shares')) {
    if (sharedRequests) return sharedRequests.promise;
    return Response.json(
      {
        id: '9223372036854775807',
        createdAt: '2026-10-21T10:00:00Z',
        preserveMarks: JSON.parse(String(options?.body)).preserveMarks,
      },
      { status: 201 },
    );
  }
  if (path.endsWith('/shopping-list-shares/import')) {
    if (sharedRequests) return sharedRequests.promise;
    return Response.json(metadata(copyId), { status: 201 });
  }
  if (path.endsWith('/items'))
    return Response.json([
      {
        id: '9223372036854775807',
        listId: sourceId,
        name: 'Молоко',
        quantity: '2.675',
        unit: 'л',
        isMarked: true,
      },
    ]);
  if (path.endsWith('/shopping-lists'))
    return Response.json([metadata(sourceId), metadata(copyId)]);
  if (path.includes('/shopping-lists/'))
    return Response.json(metadata(path.split('/').at(-1)!));
  throw new Error('Unexpected request to test transport');
}

beforeEach(() => {
  sessionCoordinator.dispose();
  localStorage.clear();
  sessionStorage.clear();
  vi.clearAllMocks();
  refreshUser = '1';
  accessVersion = 0;
  sharedRequests = null;
  routers = [];
  fetchMock.mockReset().mockImplementation(respond);
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  for (const router of routers) router.dispose();
  sessionCoordinator.dispose();
});

function show(path = `/share#token=${token}`) {
  const router = createMemoryRouter(
    [
      { path: '/share', element: <SharedShoppingListPage /> },
      {
        element: <RequireAuth />,
        children: [
          {
            element: <Layout />,
            children: [
              { path: '/lists/:listId', element: <ShoppingListPage /> },
              { path: '/lists', element: <ShoppingListsPage /> },
            ],
          },
        ],
      },
    ],
    { initialEntries: [path] },
  );
  routers.push(router);
  const view = render(
    <StrictMode>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </StrictMode>,
  );
  return { router, ...view };
}

function calls(suffix: string) {
  return fetchMock.mock.calls.filter(([url]) => String(url).endsWith(suffix));
}

function otherTab(kind: 'login' | 'logout') {
  const key = 'shopping-manager.session-revision';
  const newValue = JSON.stringify({ id: crypto.randomUUID(), kind });
  localStorage.setItem(key, newValue);
  window.dispatchEvent(new StorageEvent('storage', { key, newValue }));
}

async function openShare(user: ReturnType<typeof userEvent.setup>) {
  const view = show(`/lists/${sourceId}`);
  await user.click(await screen.findByRole('button', { name: 'Поделиться' }));
  return view;
}

describe('Создание ссылки', () => {
  it('по умолчанию сбрасывает отметки только в снимке; ждёт ответ и блокирует закрытие/двойной клик', async () => {
    const user = userEvent.setup({ delay: null });
    sharedRequests = deferred<Response>();
    await openShare(user);
    expect(
      screen.getByLabelText('Сохранить отметки покупки'),
    ).not.toBeChecked();
    expect(calls('/shares')).toHaveLength(0);
    await user.dblClick(screen.getByRole('button', { name: 'Создать ссылку' }));
    expect(calls('/shares')).toHaveLength(1);
    const body = JSON.parse(String(calls('/shares')[0][1]?.body));
    expect(body.preserveMarks).toBe(false);
    expect(isShareToken(body.token)).toBe(true);
    expect(Object.keys(body).sort()).toEqual(['preserveMarks', 'token']);
    expect(
      screen.queryByLabelText('Ссылка на копию списка'),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Закрыть' })).toBeDisabled();
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeVisible();
    await act(async () =>
      sharedRequests!.resolve(
        Response.json(
          { id: '1', createdAt: '2026-10-21T10:00:00Z', preserveMarks: false },
          { status: 201 },
        ),
      ),
    );
    expect(await screen.findByLabelText('Ссылка на копию списка')).toHaveValue(
      `${window.location.origin}/share#token=${body.token}`,
    );
    expect(calls('/items/mark-all')).toHaveLength(0);
    expect(
      screen.getByRole('checkbox', { name: 'Куплено: Молоко' }),
    ).toBeChecked();
  });

  it('сохраняет тело/ключ при сети и 5xx, а для новой ссылки меняет их', async () => {
    const user = userEvent.setup({ delay: null });
    let failed = 0;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/shares')) {
        if (failed++ === 0) throw new TypeError('Failed to fetch');
        if (failed === 2) return failure(503, 'internal_error');
      }
      return respond(url, options);
    });
    await openShare(user);
    await user.click(screen.getByLabelText('Сохранить отметки покупки'));
    await user.click(screen.getByRole('button', { name: 'Создать ссылку' }));
    await screen.findByRole('button', { name: 'Повторить' });
    expect(screen.getByLabelText('Сохранить отметки покупки')).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Повторить' }));
    await screen.findByRole('alert');
    await user.click(screen.getByRole('button', { name: 'Повторить' }));
    await screen.findByLabelText('Ссылка на копию списка');
    const original = calls('/shares')[0][1];
    expect(calls('/shares').map(([, options]) => options?.body)).toEqual([
      original?.body,
      original?.body,
      original?.body,
    ]);
    expect(
      calls('/shares').map(([, options]) =>
        new Headers(options?.headers).get('Idempotency-Key'),
      ),
    ).toEqual(
      Array(3).fill(new Headers(original?.headers).get('Idempotency-Key')),
    );
    await user.click(
      screen.getByRole('button', { name: 'Создать новую ссылку' }),
    );
    expect(
      screen.getByLabelText('Сохранить отметки покупки'),
    ).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Создать ссылку' }));
    await screen.findByLabelText('Ссылка на копию списка');
    const next = calls('/shares')[3][1];
    expect(next?.body).not.toBe(original?.body);
    expect(new Headers(next?.headers).get('Idempotency-Key')).not.toBe(
      new Headers(original?.headers).get('Idempotency-Key'),
    );
  });

  it('при отказе clipboard оставляет готовую ссылку для ручного копирования', async () => {
    const user = userEvent.setup({ delay: null });
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(
      new Error('denied'),
    );
    await openShare(user);
    await user.click(screen.getByRole('button', { name: 'Создать ссылку' }));
    const field = await screen.findByLabelText('Ссылка на копию списка');
    await user.click(screen.getByRole('button', { name: 'Копировать ссылку' }));
    expect(await screen.findByText(/скопируйте её вручную/)).toBeVisible();
    expect(field).toHaveAttribute('readonly');
    expect(calls('/shares')).toHaveLength(1);
  });

  it('скрывает поздний результат после перехода к другому списку', async () => {
    const user = userEvent.setup({ delay: null });
    sharedRequests = deferred<Response>();
    const { router } = await openShare(user);
    await user.click(screen.getByRole('button', { name: 'Создать ссылку' }));
    await act(async () => router.navigate(`/lists/${copyId}`));
    await act(async () =>
      sharedRequests!.resolve(
        Response.json(
          { id: '1', createdAt: '2026-10-21T10:00:00Z', preserveMarks: false },
          { status: 201 },
        ),
      ),
    );
    expect(router.state.location.pathname).toBe(`/lists/${copyId}`);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText('Ссылка на копию списка'),
    ).not.toBeInTheDocument();
  });
});

describe('Получение копии', () => {
  it('после сети сохраняет попытку при временной ошибке refresh', async () => {
    let post = 0;
    let refreshFails = false;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/shopping-list-shares/import') && post++ === 0)
        return failure(503, 'internal_error');
      if (String(url).endsWith('/refresh') && refreshFails)
        return failure(503, 'internal_error');
      return respond(url, options);
    });
    const user = userEvent.setup({ delay: null });
    show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Повторить' });
    refreshFails = true;
    await act(async () => {
      await sessionCoordinator.refresh().catch(() => {});
    });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    refreshFails = false;
    await user.click(
      screen.getByRole('button', { name: 'Повторить проверку сессии' }),
    );
    await user.click(await screen.findByRole('button', { name: 'Повторить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    const requests = calls('/shopping-list-shares/import');
    expect(requests).toHaveLength(2);
    expect(requests[0][1]?.body).toBe(requests[1][1]?.body);
    expect(new Headers(requests[0][1]?.headers).get('Idempotency-Key')).toBe(
      new Headers(requests[1][1]?.headers).get('Idempotency-Key'),
    );
  });

  it.each(['login', 'logout'] as const)(
    'уведомление %s другой вкладки отменяет старое согласие и позднюю навигацию',
    async (kind) => {
      const user = userEvent.setup({ delay: null });
      sharedRequests = deferred<Response>();
      const { router } = show();
      await user.click(await screen.findByRole('button', { name: 'Добавить' }));
      refreshUser = kind === 'login' ? '2' : null;
      await act(async () => otherTab(kind));
      await act(async () =>
        sharedRequests!.resolve(
          Response.json(metadata(copyId), { status: 201 }),
        ),
      );
      expect(router.state.location.pathname).toBe('/share');
      expect(calls('/shopping-list-shares/import')).toHaveLength(1);
      if (kind === 'login') {
        await screen.findByRole('dialog');
        sharedRequests = null;
        await user.click(screen.getByRole('button', { name: 'Добавить' }));
        await screen.findByRole('button', { name: 'Поделиться' });
        const requests = calls('/shopping-list-shares/import');
        expect(
          new Headers(requests[0][1]?.headers).get('Idempotency-Key'),
        ).not.toBe(new Headers(requests[1][1]?.headers).get('Idempotency-Key'));
        expect(
          new Headers(requests[1][1]?.headers).get('Authorization'),
        ).toContain('access-secret-2');
      } else {
        await screen.findByRole('heading', { name: 'Вход' });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      }
    },
  );

  it('показывает подтверждение в StrictMode и отменяет без POST', async () => {
    const user = userEvent.setup({ delay: null });
    const { router } = show();
    await screen.findByRole('dialog');
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: 'Отмена' }));
    await waitFor(() => expect(router.state.location.pathname).toBe('/lists'));
    expect(router.state.location.hash).toBe('');
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
  });

  it('двойной клик отправляет один POST; успех заменяет URL и загружает копию', async () => {
    const user = userEvent.setup({ delay: null });
    sharedRequests = deferred<Response>();
    const { router } = show();
    await user.dblClick(
      await screen.findByRole('button', { name: 'Добавить' }),
    );
    expect(calls('/shopping-list-shares/import')).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Отмена' })).toBeDisabled();
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeVisible();
    await act(async () =>
      sharedRequests!.resolve(Response.json(metadata(copyId), { status: 201 })),
    );
    await waitFor(() =>
      expect(router.state.location.pathname).toBe(`/lists/${copyId}`),
    );
    expect(router.state.historyAction).toBe('REPLACE');
    expect(router.state.location.hash).toBe('');
    await screen.findByRole('button', { name: 'Поделиться' });
    expect(calls(`/shopping-lists/${copyId}`).length).toBeGreaterThan(0);
  });

  it('после входа гостя сохраняет fragment и требует подтверждение', async () => {
    refreshUser = null;
    const user = userEvent.setup({ delay: null });
    const { router } = show();
    await screen.findByRole('heading', { name: 'Вход' });
    const registration = screen.getByRole('link', {
      name: /Зарегистрироваться в новой вкладке/,
    });
    expect(registration).toHaveAttribute('target', '_blank');
    expect(registration).toHaveAttribute('href', '/registration');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('Email'), 'user1@example.test');
    await user.type(screen.getByLabelText('Пароль'), 'password123');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    await screen.findByRole('dialog');
    expect(router.state.location.hash).toBe(`#token=${token}`);
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: 'Добавить' }));
    await waitFor(() =>
      expect(router.state.location.pathname).toBe(`/lists/${copyId}`),
    );
  });

  it('ошибка входа видна, подтверждение email можно отправить повторно', async () => {
    refreshUser = null;
    fetchMock.mockImplementation(async (url, options) =>
      String(url).endsWith('/login')
        ? failure(403, 'email_not_verified')
        : respond(url, options),
    );
    const user = userEvent.setup({ delay: null });
    show();
    await screen.findByRole('heading', { name: 'Вход' });
    await user.type(screen.getByLabelText('Email'), 'user1@example.test');
    await user.type(screen.getByLabelText('Пароль'), 'password123');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Запрос не выполнен',
    );
    expect(screen.getByRole('button', { name: /Отправить/ })).toBeVisible();
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
  });

  it('checking/error не импортируют и не объявляют ссылку недействительной; повтор сессии возвращает подтверждение', async () => {
    const refresh = deferred<Response>();
    fetchMock.mockImplementationOnce(() => refresh.promise);
    const user = userEvent.setup({ delay: null });
    show();
    expect(screen.getByText('Проверяем сессию…')).toBeVisible();
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
    await act(async () => refresh.resolve(failure(503, 'internal_error')));
    expect(
      await screen.findByRole('button', { name: 'Повторить проверку сессии' }),
    ).toBeVisible();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByText(/Ссылка недоступна/)).not.toBeInTheDocument();
    await user.click(
      screen.getByRole('button', { name: 'Повторить проверку сессии' }),
    );
    await screen.findByRole('dialog');
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
  });

  it.each([
    '',
    '#token=bad',
    `?token=${token}`,
    `#token=${token}&token=${token}`,
  ])('отвергает некорректную ссылку %s без API импорта', async (fragment) => {
    show('/share' + fragment);
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Некорректная ссылка',
    );
    expect(
      screen.queryByRole('button', { name: 'Добавить' }),
    ).not.toBeInTheDocument();
    expect(calls('/shopping-list-shares/import')).toHaveLength(0);
  });

  it('404 показывает недоступную ссылку без успешной навигации', async () => {
    fetchMock.mockImplementation(async (url, options) =>
      String(url).endsWith('/shopping-list-shares/import')
        ? failure(404, 'not_found')
        : respond(url, options),
    );
    const user = userEvent.setup({ delay: null });
    const { router } = show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Ссылка недоступна',
    );
    expect(router.state.location.pathname).toBe('/share');
    expect(
      screen.queryByRole('button', { name: 'Добавить' }),
    ).not.toBeInTheDocument();
  });

  it('после сети повторяет действие, а resource_deleted предлагает новое подтверждение с новым ключом', async () => {
    let attempt = 0;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/shopping-list-shares/import')) {
        if (attempt++ === 0) throw new TypeError('Failed to fetch');
        if (attempt === 2) return failure(409, 'resource_deleted');
      }
      return respond(url, options);
    });
    const user = userEvent.setup({ delay: null });
    show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    await user.click(await screen.findByRole('button', { name: 'Повторить' }));
    await user.click(
      await screen.findByRole('button', { name: 'Добавить новую копию' }),
    );
    expect(calls('/shopping-list-shares/import')).toHaveLength(2);
    await user.click(screen.getByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    const requests = calls('/shopping-list-shares/import').map(
      ([, options]) => ({
        body: options?.body,
        key: new Headers(options?.headers).get('Idempotency-Key'),
      }),
    );
    expect(requests[0]).toEqual(requests[1]);
    expect(requests[2].body).toBe(requests[0].body);
    expect(requests[2].key).not.toBe(requests[0].key);
  });

  it('refresh access повторяет POST с теми же телом и ключом', async () => {
    let first = true;
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/shopping-list-shares/import') && first) {
        first = false;
        return failure(401, 'unauthorized');
      }
      return respond(url, options);
    });
    const user = userEvent.setup({ delay: null });
    show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    const requests = calls('/shopping-list-shares/import');
    expect(requests).toHaveLength(2);
    expect(requests[0][1]?.body).toBe(requests[1][1]?.body);
    expect(new Headers(requests[0][1]?.headers).get('Idempotency-Key')).toBe(
      new Headers(requests[1][1]?.headers).get('Idempotency-Key'),
    );
    expect(new Headers(requests[0][1]?.headers).get('Authorization')).not.toBe(
      new Headers(requests[1][1]?.headers).get('Authorization'),
    );
  });

  it('повторное открытие и remount не импортируют автоматически; подтверждения получают новые ключи', async () => {
    const user = userEvent.setup({ delay: null });
    const first = show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    await act(async () => first.router.navigate(`/share#token=${token}`));
    await screen.findByRole('dialog');
    expect(calls('/shopping-list-shares/import')).toHaveLength(1);
    first.unmount();
    show();
    await screen.findByRole('dialog');
    expect(calls('/shopping-list-shares/import')).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    const requests = calls('/shopping-list-shares/import');
    expect(
      new Headers(requests[0][1]?.headers).get('Idempotency-Key'),
    ).not.toBe(new Headers(requests[1][1]?.headers).get('Idempotency-Key'));
  });

  it.each(['logout', 'login', 'navigation'])(
    'поздний успех после %s не открывает копию; новый пользователь подтверждает заново',
    async (change) => {
      const user = userEvent.setup({ delay: null });
      sharedRequests = deferred<Response>();
      const { router } = show();
      await user.click(await screen.findByRole('button', { name: 'Добавить' }));
      await act(async () => {
        if (change === 'logout') await sessionCoordinator.logout();
        else if (change === 'login')
          await sessionCoordinator.login({
            email: 'user2@example.test',
            password: 'password123',
          });
        else await router.navigate('/lists');
      });
      await act(async () =>
        sharedRequests!.resolve(
          Response.json(metadata(copyId), { status: 201 }),
        ),
      );
      expect(router.state.location.pathname).toBe(
        change === 'navigation' ? '/lists' : '/share',
      );
      expect(calls('/shopping-list-shares/import')).toHaveLength(1);
      if (change === 'login') {
        await screen.findByRole('dialog');
        sharedRequests = null;
        await user.click(screen.getByRole('button', { name: 'Добавить' }));
        await screen.findByRole('button', { name: 'Поделиться' });
        const requests = calls('/shopping-list-shares/import');
        expect(
          new Headers(requests[0][1]?.headers).get('Idempotency-Key'),
        ).not.toBe(new Headers(requests[1][1]?.headers).get('Idempotency-Key'));
      }
    },
  );

  it('не сохраняет URL/секрет/снимок в storage или журнале', async () => {
    const user = userEvent.setup({ delay: null });
    show();
    await user.click(await screen.findByRole('button', { name: 'Добавить' }));
    await screen.findByRole('button', { name: 'Поделиться' });
    const persisted = JSON.stringify([
      Object.values(localStorage),
      Object.values(sessionStorage),
      vi.mocked(logger.info).mock.calls,
      vi.mocked(logger.error).mock.calls,
    ]);
    expect(persisted).not.toContain(token);
    expect(persisted).not.toContain('access-secret');
    expect(persisted).not.toContain('Молоко');
    expect(
      within(screen.getByRole('main')).queryByRole('dialog'),
    ).not.toBeInTheDocument();
  });
});
