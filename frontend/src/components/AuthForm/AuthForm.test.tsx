import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import AuthForm from './AuthForm';
describe('AuthForm', () => {
  it('проверяет обязательные поля и отправляет данные входа', async () => {
    const submit = vi.fn();
    const user = userEvent.setup({ delay: null });
    render(
      <MemoryRouter>
        <AuthForm mode="login" onSubmit={submit} />
      </MemoryRouter>,
    );
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByText('Введите email')).toBeVisible();
    expect(submit).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText('Email'), 'user@example.test');
    await user.type(screen.getByLabelText('Пароль'), 'test-password');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({
        email: 'user@example.test',
        password: 'test-password',
      }),
    );
    expect(
      screen.getByRole('link', { name: 'Зарегистрироваться' }),
    ).toHaveAttribute('href', '/registration');
  });
  it('проверяет длины имени и пароля при регистрации', async () => {
    const submit = vi.fn();
    const user = userEvent.setup({ delay: null });
    render(
      <MemoryRouter>
        <AuthForm mode="registration" onSubmit={submit} />
      </MemoryRouter>,
    );
    await user.type(screen.getByLabelText('Имя'), 'А');
    await user.type(screen.getByLabelText('Email'), 'user@example.test');
    await user.type(screen.getByLabelText('Пароль'), 'short');
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(await screen.findByText('Минимум 2 символа')).toBeVisible();
    expect(await screen.findByText('Минимум 6 символов')).toBeVisible();
    expect(submit).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText('Имя'));
    await user.type(screen.getByLabelText('Имя'), 'Имя');
    await user.clear(screen.getByLabelText('Пароль'));
    await user.type(screen.getByLabelText('Пароль'), 'test-password');
    await user.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({
        name: 'Имя',
        email: 'user@example.test',
        password: 'test-password',
      }),
    );
  });
  it('показывает серверную ошибку и блокирует кнопку по loading', () => {
    render(
      <MemoryRouter>
        <AuthForm
          mode="login"
          loading
          error="Ошибка входа"
          onSubmit={vi.fn()}
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Ошибка входа');
    expect(screen.getByRole('button', { name: 'Войти' })).toBeDisabled();
  });
});
