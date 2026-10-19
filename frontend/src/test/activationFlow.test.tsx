import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ActivationPage from '../pages/ActivationPage/ActivationPage';
import { clearAccessToken, getAccessToken } from '../services/tokenStorage';

vi.mock('../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), error: vi.fn() },
}));
const fetchMock = vi.fn<typeof fetch>();
const token = '12345678-1234-4123-8123-123456789abc';

function Location() {
  return <span data-testid="location">{useLocation().pathname}</span>;
}
function renderActivation(path = `/activation/${token}`) {
  render(
    <MemoryRouter initialEntries={[path]}>
      <Location />
      <Routes>
        <Route path="/activation/:token?" element={<ActivationPage />} />
        <Route path="/login" element={<div>Вход в аккаунт</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
  clearAccessToken();
});

describe('Подтверждение почты', () => {
  it('убирает токен из URL и подтверждает только после нажатия без автоматического входа', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    renderActivation();
    await waitFor(() =>
      expect(screen.getByTestId('location')).toHaveTextContent(
        /^\/activation$/,
      ),
    );
    expect(fetchMock).not.toHaveBeenCalled();
    await userEvent.click(
      screen.getByRole('button', { name: 'Подтвердить почту' }),
    );
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Почта подтверждена',
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/activation');
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      token,
    });
    expect(getAccessToken()).toBeNull();
    await userEvent.click(
      screen.getByRole('link', { name: 'Перейти ко входу' }),
    );
    expect(await screen.findByText('Вход в аккаунт')).toBeVisible();
  });

  it('ожидает подтверждение и не допускает второй запрос', async () => {
    let resolve!: (value: Response) => void;
    fetchMock.mockReturnValue(
      new Promise<Response>((done) => {
        resolve = done;
      }),
    );
    renderActivation();
    await userEvent.click(
      screen.getByRole('button', { name: 'Подтвердить почту' }),
    );
    const pending = screen.getByRole('button', { name: 'Подтверждаем…' });
    expect(pending).toBeDisabled();
    await userEvent.click(pending);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    resolve(new Response(null, { status: 204 }));
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Почта подтверждена',
    );
  });

  it('показывает истёкшую ссылку и позволяет запросить письмо', async () => {
    fetchMock.mockResolvedValue(
      Response.json(
        {
          code: 'invalid_activation',
          message: 'Ссылка истекла. Запросите новое письмо.',
        },
        { status: 400 },
      ),
    );
    renderActivation();
    await userEvent.click(
      screen.getByRole('button', { name: 'Подтвердить почту' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Ссылка истекла',
    );
    expect(
      screen.queryByRole('button', { name: 'Подтвердить почту' }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Отправить письмо повторно' }),
    ).toBeVisible();
  });

  it('позволяет повторить подтверждение после временного сетевого сбоя', async () => {
    fetchMock
      .mockRejectedValueOnce(new Error('Сеть недоступна'))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    renderActivation();
    await userEvent.click(
      screen.getByRole('button', { name: 'Подтвердить почту' }),
    );
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Сеть недоступна',
    );
    await userEvent.click(
      screen.getByRole('button', { name: 'Подтвердить почту' }),
    );
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Почта подтверждена',
    );
  });

  it.each([202, 429, 503])(
    'обрабатывает ответ resend %s без ложного успеха',
    async (status) => {
      const message =
        status === 202
          ? 'Если аккаунт ожидает подтверждения, письмо отправлено.'
          : status === 429
            ? 'Повторите позже'
            : 'Не удалось отправить письмо';
      fetchMock.mockResolvedValue(
        Response.json({ code: 'activation_cooldown', message }, { status }),
      );
      renderActivation('/activation');
      expect(
        screen.queryByRole('button', { name: 'Подтвердить почту' }),
      ).not.toBeInTheDocument();
      const form = within(
        screen.getByRole('form', { name: 'Повторное письмо' }),
      );
      await userEvent.type(
        form.getByLabelText('Почта для подтверждения'),
        'user@example.test',
      );
      await userEvent.click(
        form.getByRole('button', { name: 'Отправить письмо повторно' }),
      );
      expect(
        await form.findByRole(status === 202 ? 'status' : 'alert'),
      ).toHaveTextContent(message);
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(fetchMock.mock.calls[0][0]).toBe('/api/activation/resend');
      if (status !== 202)
        expect(form.queryByRole('status')).not.toBeInTheDocument();
    },
  );
});
