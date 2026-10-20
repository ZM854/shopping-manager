import { StrictMode, useState, type PropsWithChildren } from 'react';
import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from './AuthProvider';
import { useAuth } from '../../hooks/useAuth';
import { apiFetch } from '../../services/api';
import { sessionCoordinator, StaleSessionError } from '../../services/session';
import { getAccessToken } from '../../services/tokenStorage';
import RequireAuth from '../../router/RequireAuth';
import ProfilePage from '../../pages/ProfilePage/ProfilePage';

const fetchMock = vi.fn<typeof fetch>();
const response = {
  user: {
    id: 1,
    name: 'Имя',
    email: 'user@example.test',
    isEmailVerified: true,
  },
  accessToken: 'test-access',
};
const credentials = { email: 'user@example.test', password: 'test-password' };
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function StrictProvider({ children }: PropsWithChildren) {
  return (
    <StrictMode>
      <AuthProvider>{children}</AuthProvider>
    </StrictMode>
  );
}
beforeEach(() => {
  sessionCoordinator.dispose();
  localStorage.clear();
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => sessionCoordinator.dispose());

describe('AuthProvider с настоящим HTTP-клиентом', () => {
  it('в StrictMode и параллельных API-запросах выполняет один refresh и обновляет user/access вместе', async () => {
    const pending = deferred<Response>();
    fetchMock.mockImplementation(async (url, options) => {
      if (String(url).endsWith('/refresh')) return pending.promise;
      return new Headers(options?.headers).has('Authorization')
        ? Response.json([])
        : new Response(null, { status: 401 });
    });
    const { result } = renderHook(useAuth, { wrapper: StrictProvider });
    const resource = apiFetch('/shopping-lists');
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await act(async () => {
      pending.resolve(Response.json(response));
      await resource;
    });
    expect(result.current.user).toEqual(response.user);
    expect(result.current.status).toBe('authenticated');
    expect(getAccessToken()).toBe(response.accessToken);
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/refresh')),
    ).toHaveLength(1);
  });

  it('временная ошибка сохраняет маршрут и даёт повтор, а 401 очищает и user, и access', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('offline'));
    render(
      <AuthProvider>
        <MemoryRouter initialEntries={['/private']}>
          <Routes>
            <Route path="/login" element={<div>Страница входа</div>} />
            <Route element={<RequireAuth />}>
              <Route path="/private" element={<div>Приватная страница</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </AuthProvider>,
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Не удалось проверить сессию',
    );
    expect(screen.queryByText('Страница входа')).not.toBeInTheDocument();
    fetchMock.mockResolvedValueOnce(Response.json(response));
    await userEvent.click(
      screen.getByRole('button', { name: 'Повторить проверку сессии' }),
    );
    expect(await screen.findByText('Приватная страница')).toBeVisible();
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 401 }));
    await act(async () => {
      await expect(sessionCoordinator.refresh()).rejects.toMatchObject({
        status: 401,
      });
    });
    expect(await screen.findByText('Страница входа')).toBeVisible();
    expect(sessionCoordinator.getSnapshot().user).toBeNull();
    expect(getAccessToken()).toBeNull();
  });

  it('после 401 API обновляет пользователя провайдера; сетевой сбой refresh не объявляет сессию отозванной', async () => {
    fetchMock.mockResolvedValueOnce(Response.json(response));
    const { result } = renderHook(useAuth, { wrapper: AuthProvider });
    await waitFor(() => expect(result.current.status).toBe('authenticated'));
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockResolvedValueOnce(
        Response.json({
          ...response,
          user: { ...response.user, name: 'Новое имя' },
          accessToken: 'new',
        }),
      )
      .mockResolvedValueOnce(Response.json([]));
    await act(async () => {
      await apiFetch('/shopping-lists');
    });
    expect(result.current.user?.name).toBe('Новое имя');
    expect(getAccessToken()).toBe('new');
    fetchMock
      .mockResolvedValueOnce(new Response(null, { status: 401 }))
      .mockRejectedValueOnce(new TypeError('offline'));
    await act(async () => {
      await expect(apiFetch('/shopping-lists')).rejects.toThrow('offline');
    });
    expect(result.current.status).toBe('error');
    expect(result.current.user?.name).toBe('Новое имя');
    expect(getAccessToken()).toBe('new');
  });

  it('logout ждёт refresh, игнорирует его поздний ответ и блокирует повтор; отказ сервера виден и допускает повтор', async () => {
    fetchMock.mockResolvedValueOnce(Response.json(response));
    render(
      <AuthProvider>
        <ProfilePage />
      </AuthProvider>,
    );
    await screen.findByRole('heading', { name: 'Имя' });
    const pendingRefresh = deferred<Response>();
    const pendingLogout = deferred<Response>();
    fetchMock.mockImplementation(async (url) =>
      String(url).endsWith('/refresh')
        ? pendingRefresh.promise
        : pendingLogout.promise,
    );
    let refreshing!: Promise<void>;
    act(() => {
      refreshing = sessionCoordinator.refresh();
    });
    // Attach the rejection handler before releasing the response.
    const rejectedRefresh =
      expect(refreshing).rejects.toBeInstanceOf(StaleSessionError);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await userEvent.click(
      screen.getByRole('button', { name: 'Выйти из аккаунта' }),
    );
    expect(screen.getByRole('button', { name: 'Выходим…' })).toBeDisabled();
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/logout')),
    ).toHaveLength(0);
    await act(async () => {
      pendingRefresh.resolve(
        Response.json({ ...response, accessToken: 'late' }),
      );
      await rejectedRefresh;
    });
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    await act(async () => {
      pendingLogout.resolve(new Response(null, { status: 500 }));
    });
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Выход не подтверждён сервером',
    );
    expect(getAccessToken()).toBe('test-access');
    expect(screen.getByRole('heading', { name: 'Имя' })).toBeVisible();
    fetchMock.mockResolvedValueOnce(Response.json({ message: 'ok' }));
    await userEvent.click(
      screen.getByRole('button', { name: 'Выйти из аккаунта' }),
    );
    await waitFor(() =>
      expect(sessionCoordinator.getSnapshot().status).toBe('guest'),
    );
    expect(getAccessToken()).toBeNull();
  });

  it('при новом входе отбрасывает старый API-ответ и сбрасывает приватную форму на том же маршруте', async () => {
    function PrivateForm() {
      const [draft, setDraft] = useState('');
      const { user } = useAuth();
      return (
        <>
          <p>{user?.name}</p>
          <input
            aria-label="Черновик"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
          />
        </>
      );
    }
    fetchMock.mockResolvedValueOnce(Response.json(response));
    render(
      <AuthProvider>
        <MemoryRouter initialEntries={['/private']}>
          <Routes>
            <Route element={<RequireAuth />}>
              <Route path="/private" element={<PrivateForm />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </AuthProvider>,
    );
    await screen.findByText('Имя');
    await userEvent.type(
      screen.getByLabelText('Черновик'),
      'Личные данные первого аккаунта',
    );
    const pending = deferred<Response>();
    fetchMock.mockReturnValueOnce(pending.promise);
    const oldRequest = apiFetch('/shopping-lists');
    const rejected =
      expect(oldRequest).rejects.toBeInstanceOf(StaleSessionError);
    fetchMock.mockResolvedValueOnce(
      Response.json({
        ...response,
        user: { ...response.user, id: 2, name: 'Другой аккаунт' },
        accessToken: 'other',
      }),
    );
    await act(async () => {
      await sessionCoordinator.login(credentials);
    });
    expect(await screen.findByText('Другой аккаунт')).toBeVisible();
    expect(screen.getByLabelText('Черновик')).toHaveValue('');
    pending.resolve(new Response(null, { status: 401 }));
    await rejected;
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/refresh')),
    ).toHaveLength(1);
    expect(getAccessToken()).toBe('other');
  });

  it('useAuth требует провайдера', () => {
    expect(() => renderHook(useAuth)).toThrow('useAuth');
  });
});
