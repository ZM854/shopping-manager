import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  AuthContext,
  type AuthContextValue,
} from '../context/authContext/authContext';
import LoginPage from '../pages/LoginPage/LoginPage';
import RegistrationPage from '../pages/RegistrationPage/RegistrationPage';
import ProfilePage from '../pages/ProfilePage/ProfilePage';
import ShoppingListPage from '../pages/ShoppingListPage/ShoppingListPage';
import RequireAuth from '../router/RequireAuth';
import Layout from '../layout/Layout';
import ProductService from '../services/productService';
vi.mock('../services/productService', () => ({
  default: {
    getProducts: vi.fn(),
    postProduct: vi.fn(),
    updateProduct: vi.fn(),
    deleteProduct: vi.fn(),
    deleteAllProducts: vi.fn(),
  },
}));
const products = vi.mocked(ProductService);
const initial = {
  id: 1,
  name: 'Молоко',
  quantity: 1.5,
  unit: 'л',
  isMarked: false,
};
beforeEach(() => {
  vi.resetAllMocks();
  products.getProducts.mockResolvedValue([initial]);
});
function authValue(
  overrides: Partial<AuthContextValue> = {},
): AuthContextValue {
  return {
    user: null,
    isAuthenticated: false,
    isLoading: false,
    login: vi.fn().mockResolvedValue(undefined),
    register: vi.fn().mockResolvedValue(undefined),
    refresh: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}
function LocationState() {
  const location = useLocation();
  return <div data-testid="location">{JSON.stringify(location.state)}</div>;
}
async function fillCredentials() {
  const user = userEvent.setup({ delay: null });
  await user.type(screen.getByLabelText('Email'), 'user@example.test');
  await user.type(screen.getByLabelText('Пароль'), 'test-password');
  return user;
}
describe('Страницы авторизации и навигация', () => {
  it('после входа возвращает пользователя на запрошенную страницу', async () => {
    const auth = authValue();
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter
          initialEntries={[
            {
              pathname: '/login',
              state: {
                from: { pathname: '/profile' },
                registered: true,
                email: 'user@example.test',
              },
            },
          ]}
        >
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="/profile" element={<div>Возврат в профиль</div>} />
          </Routes>
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    expect(screen.getByText(/Мы отправили письмо/)).toHaveTextContent(
      'user@example.test',
    );
    const user = await fillCredentials();
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByText('Возврат в профиль')).toBeVisible();
    expect(auth.login).toHaveBeenCalledWith({
      email: 'user@example.test',
      password: 'test-password',
    });
  });
  it('при отклонённом входе показывает ошибку и остаётся на форме', async () => {
    const auth = authValue({
      login: vi.fn().mockRejectedValue(new Error('Неверный пароль')),
    });
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter>
          <LoginPage />
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    const user = await fillCredentials();
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Неверный пароль',
    );
  });
  it('регистрация передаёт email на страницу входа', async () => {
    const auth = authValue();
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter initialEntries={['/registration']}>
          <Routes>
            <Route path="/registration" element={<RegistrationPage />} />
            <Route path="/login" element={<LocationState />} />
          </Routes>
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    const user = await fillCredentials();
    await user.type(screen.getByLabelText('Имя'), 'Имя');
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(await screen.findByTestId('location')).toHaveTextContent(
      '"registered":true',
    );
    expect(screen.getByTestId('location')).toHaveTextContent(
      'user@example.test',
    );
  });
  it('ошибка регистрации отображается в форме', async () => {
    const auth = authValue({
      register: vi.fn().mockRejectedValue(new Error('Email занят')),
    });
    render(
      <AuthContext.Provider value={auth}>
        <MemoryRouter>
          <RegistrationPage />
        </MemoryRouter>
      </AuthContext.Provider>,
    );
    const user = await fillCredentials();
    await user.type(screen.getByLabelText('Имя'), 'Имя');
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent('Email занят');
  });
  it.each([
    { loading: true, authenticated: false },
    { loading: false, authenticated: false },
    { loading: false, authenticated: true },
  ])(
    'защищённый маршрут: $loading/$authenticated',
    async ({ loading, authenticated }) => {
      render(
        <AuthContext.Provider
          value={authValue({
            isLoading: loading,
            isAuthenticated: authenticated,
          })}
        >
          <MemoryRouter initialEntries={['/private']}>
            <Routes>
              <Route element={<RequireAuth />}>
                <Route path="/private" element={<div>Закрытая страница</div>} />
              </Route>
              <Route path="/login" element={<LocationState />} />
            </Routes>
          </MemoryRouter>
        </AuthContext.Provider>,
      );
      if (loading) {
        expect(screen.queryByText('Закрытая страница')).not.toBeInTheDocument();
        expect(screen.queryByTestId('location')).not.toBeInTheDocument();
      } else if (authenticated)
        expect(screen.getByText('Закрытая страница')).toBeVisible();
      else
        expect(await screen.findByTestId('location')).toHaveTextContent(
          '/private',
        );
    },
  );
  it('профиль показывает пользователя и вызывает выход', async () => {
    const auth = authValue({
      user: {
        id: 1,
        name: 'Имя',
        email: 'user@example.test',
        isEmailVerified: true,
      },
    });
    const { rerender } = render(
      <AuthContext.Provider value={auth}>
        <ProfilePage />
      </AuthContext.Provider>,
    );
    expect(screen.getByRole('heading', { name: 'Имя' })).toBeVisible();
    await userEvent
      .setup({ delay: null })
      .click(screen.getByRole('button', { name: 'Выйти из аккаунта' }));
    expect(auth.logout).toHaveBeenCalledOnce();
    rerender(
      <AuthContext.Provider value={authValue()}>
        <ProfilePage />
      </AuthContext.Provider>,
    );
    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
  });
});
describe('Страница покупок с существующими компонентами и hooks', () => {
  it('создаёт, редактирует и очищает позиции; отмена не вызывает запрос', async () => {
    const user = userEvent.setup({ delay: null });
    products.postProduct.mockResolvedValue({ ...initial, id: 2, name: 'Хлеб' });
    products.updateProduct.mockResolvedValue({ ...initial, name: 'Кефир' });
    products.deleteAllProducts.mockResolvedValue([]);
    render(
      <MemoryRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<ShoppingListPage />} />
            <Route path="/profile" element={<div>Профиль</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );
    expect(
      await screen.findByRole('heading', { name: 'Молоко' }),
    ).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    let dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByLabelText('Название'), 'Хлеб');
    await user.type(within(dialog).getByLabelText('Количество'), '1');
    await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByRole('heading', { name: 'Хлеб' })).toBeVisible();
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
    await user.click(
      screen.getByRole('button', { name: 'Редактировать Молоко' }),
    );
    dialog = screen.getByRole('dialog');
    await user.clear(within(dialog).getByLabelText('Название'));
    await user.type(within(dialog).getByLabelText('Название'), 'Кефир');
    await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByRole('heading', { name: 'Кефир' })).toBeVisible();
    await user.click(
      screen.getByRole('button', { name: 'Очистить список покупок' }),
    );
    await user.click(screen.getByRole('button', { name: 'Отменить' }));
    expect(products.deleteAllProducts).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole('button', { name: 'Очистить список покупок' }),
    );
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    expect(await screen.findByText('Список покупок пока пуст.')).toBeVisible();
    await user.click(screen.getByRole('link', { name: 'Профиль' }));
    expect(
      await screen.findByText('Профиль', { selector: 'div' }),
    ).toBeVisible();
    expect(
      screen.queryByRole('button', { name: 'Добавить товар' }),
    ).not.toBeInTheDocument();
  });
});
