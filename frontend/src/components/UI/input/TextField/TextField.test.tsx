import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import TextField from './TextField';

describe('TextField', () => {
  it('связывает подпись с полем и передаёт пользовательский ввод', async () => {
    const onChange = vi.fn();
    render(<TextField label="Название списка" onChange={onChange} />);

    const input = screen.getByRole('textbox', { name: 'Название списка' });
    await userEvent.setup({ delay: null }).type(input, 'Покупки');

    expect(input).toHaveValue('Покупки');
    expect(onChange).toHaveBeenCalled();
  });

  it('делает ошибку доступной для пользователя и скринридера', () => {
    render(<TextField label="Название" error="Введите название" />);

    expect(screen.getByRole('alert')).toHaveTextContent('Введите название');
    const input = screen.getByRole('textbox', { name: 'Название' });
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription('Введите название');
  });
});
