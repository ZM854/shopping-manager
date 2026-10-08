import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import App from './App';
import { router } from './router/router';
vi.mock('./services/authService', () => ({
  authService: {
    refresh: vi
      .fn()
      .mockResolvedValue({
        user: {
          id: 1,
          name: 'Имя',
          email: 'user@example.test',
          isEmailVerified: true,
        },
        accessToken: 'test-access',
      }),
    logout: vi.fn().mockResolvedValue(undefined),
  },
}));
vi.mock('./services/productService', () => ({
  default: { getProducts: vi.fn().mockResolvedValue([]) },
}));
vi.mock('./shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), error: vi.fn() },
}));
it('App использует настоящие маршруты, провайдер и переход к входу после logout', async () => {
  const user = userEvent.setup({ delay: null });
  const { unmount } = render(<App />);
  try {
    expect(await screen.findByText('Список покупок пока пуст.')).toBeVisible();
    await user.click(screen.getByRole('link', { name: 'Профиль' }));
    expect(await screen.findByRole('heading', { name: 'Имя' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Выйти из аккаунта' }));
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeVisible();
    expect(router.state.location.pathname).toBe('/login');
  } finally {
    unmount();
    router.dispose();
  }
});
