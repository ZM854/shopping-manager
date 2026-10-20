import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from '../context/authContext/AuthProvider';
import LoginPage from '../pages/LoginPage/LoginPage';
import RegistrationPage from '../pages/RegistrationPage/RegistrationPage';
import RequireAuth from '../router/RequireAuth';
import { sessionCoordinator } from '../services/session';
import { clearAccessToken, getAccessToken } from '../services/tokenStorage';

vi.mock('../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), error: vi.fn() },
}));

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => {
  sessionCoordinator.dispose();
  localStorage.clear();
  clearAccessToken();
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => sessionCoordinator.dispose());

function renderAuth(path: string) {
  render(
    <AuthProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/registration" element={<RegistrationPage />} />
          <Route element={<RequireAuth />}>
            <Route path="/" element={<div>Личные данные</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </AuthProvider>,
  );
}

async function fillAuth(registration = false) {
  const user = userEvent.setup({ delay: null });
  if (registration) await user.type(screen.getByLabelText('Имя'), 'Проверка');
  await user.type(screen.getByLabelText('Email'), 'user@example.test');
  await user.type(screen.getByLabelText('Пароль'), 'password123');
  return user;
}

describe('AuthProvider, HTTP и формы', () => {
  it.each([
    {
      path: '/login',
      status: 401,
      code: 'invalid_credentials',
      message: 'Неверный email или пароль',
    },
    {
      path: '/login',
      status: 403,
      code: 'email_not_verified',
      message: 'Подтвердите email перед входом',
    },
    {
      path: '/registration',
      status: 409,
      code: 'user_already_exists',
      message: 'Пользователь с таким email уже существует',
    },
    {
      path: '/registration',
      status: 500,
      code: 'internal_error',
      message: 'Не удалось зарегистрироваться',
    },
  ])(
    'показывает $code без ложного успеха',
    async ({ path, status, code, message }) => {
      fetchMock.mockImplementation(async (url) =>
        String(url).endsWith('/refresh')
          ? Response.json(
              { code: 'unauthorized', message: 'Нет сессии' },
              { status: 401 },
            )
          : Response.json({ code, message }, { status }),
      );
      renderAuth(path);
      const registration = path === '/registration';
      const user = await fillAuth(registration);
      await user.click(
        screen.getByRole('button', {
          name: registration ? 'Зарегистрироваться' : 'Войти',
        }),
      );
      expect(await screen.findByRole('alert')).toHaveTextContent(message);
      expect(screen.queryByText('Личные данные')).not.toBeInTheDocument();
      expect(screen.queryByText(/Мы отправили письмо/)).not.toBeInTheDocument();
      expect(getAccessToken()).toBeNull();
      expect(
        fetchMock.mock.calls.filter(([url]) =>
          String(url).endsWith('/refresh'),
        ),
      ).toHaveLength(1);
      expect(
        fetchMock.mock.calls.filter(([url]) => String(url).endsWith(path)),
      ).toHaveLength(1);
    },
  );

  it('после регистрации предлагает подтвердить почту без входа', async () => {
    fetchMock.mockImplementation(async (url) =>
      String(url).endsWith('/refresh')
        ? Response.json(
            { code: 'unauthorized', message: 'Нет сессии' },
            { status: 401 },
          )
        : Response.json({ activationRequired: true }, { status: 201 }),
    );
    renderAuth('/registration');
    const user = await fillAuth(true);
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(await screen.findByText(/Мы отправили письмо/)).toHaveTextContent(
      'user@example.test',
    );
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled();
    expect(getAccessToken()).toBeNull();
  });

  it('после отказа SMTP оставляет форму и предлагает повторить письмо', async () => {
    fetchMock.mockImplementation(async (url) =>
      String(url).endsWith('/refresh')
        ? Response.json(
            { code: 'unauthorized', message: 'Нет сессии' },
            { status: 401 },
          )
        : Response.json(
            {
              code: 'activation_delivery_failed',
              message: 'Аккаунт создан, но письмо не отправлено.',
            },
            { status: 503 },
          ),
    );
    renderAuth('/registration');
    const user = await fillAuth(true);
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Аккаунт создан, но письмо не отправлено',
    );
    expect(screen.getByLabelText('Почта для подтверждения')).toHaveValue(
      'user@example.test',
    );
    expect(
      screen.getByRole('button', { name: 'Отправить письмо повторно' }),
    ).toBeVisible();
    expect(screen.queryByText(/Мы отправили письмо/)).not.toBeInTheDocument();
  });

  it('ожидает ответ входа и блокирует повторную отправку', async () => {
    let resolveLogin!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      resolveLogin = resolve;
    });
    fetchMock.mockImplementation(async (url) =>
      String(url).endsWith('/refresh')
        ? Response.json(
            { code: 'unauthorized', message: 'Нет сессии' },
            { status: 401 },
          )
        : pending,
    );
    renderAuth('/login');
    const user = await fillAuth();
    const button = screen.getByRole('button', { name: 'Войти' });
    await user.click(button);
    await waitFor(() => expect(button).toBeDisabled());
    await user.click(button);
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url).endsWith('/login')),
    ).toHaveLength(1);
    resolveLogin(
      Response.json({
        user: {
          id: 1,
          name: 'Проверка',
          email: 'user@example.test',
          isEmailVerified: true,
        },
        accessToken: 'test-only-access',
      }),
    );
    expect(await screen.findByText('Личные данные')).toBeVisible();
    expect(getAccessToken()).toBe('test-only-access');
  });
});
