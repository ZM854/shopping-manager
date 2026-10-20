import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import AuthForm from './AuthForm';
describe('AuthForm', () => {
  it('обрезает имя/email, сохраняет 20 Unicode-кодовых точек и отклоняет 74 байта пароля', async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    render(
      <MemoryRouter>
        <AuthForm mode="registration" onSubmit={submit} />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText('Имя'), {
      target: { value: '  ' + '😀'.repeat(20) + '  ' },
    });
    fireEvent.change(screen.getByLabelText('Email'), {
      target: { value: ' USER@Example.Test ' },
    });
    fireEvent.change(screen.getByLabelText('Пароль'), {
      target: { value: 'я'.repeat(37) },
    });
    await userEvent.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    expect(
      await screen.findByText('Пароль должен занимать не более 72 байт UTF-8'),
    ).toBeVisible();
    expect(submit).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('Пароль'), {
      target: { value: 'я'.repeat(36) },
    });
    await userEvent.click(
      screen.getByRole('button', { name: 'Зарегистрироваться' }),
    );
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith({
        name: '😀'.repeat(20),
        email: 'user@example.test',
        password: 'я'.repeat(36),
      }),
    );
  });
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
