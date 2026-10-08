import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ProductForm from './ProductForm';
describe('ProductForm текущего API', () => {
  it('создаёт позицию и отклоняет название из пробелов', async () => {
    const save = vi.fn();
    const user = userEvent.setup({ delay: null });
    render(<ProductForm product={null} onSave={save} />);
    await user.type(screen.getByLabelText('Название'), '   ');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(save).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText('Название'));
    await user.type(screen.getByLabelText('Название'), 'Молоко');
    await user.type(screen.getByLabelText('Количество'), '1.5');
    await user.type(screen.getByLabelText('Единица измерения'), 'л');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(save).toHaveBeenCalledWith({
      name: 'Молоко',
      quantity: 1.5,
      unit: 'л',
      isMarked: false,
    });
  });
  it('заполняет форму редактирования и передаёт снятие отметки', async () => {
    const save = vi.fn();
    const user = userEvent.setup({ delay: null });
    const product = {
      id: 1,
      name: 'Молоко',
      quantity: 1.5,
      unit: 'л',
      isMarked: true,
    };
    render(<ProductForm product={product} onSave={save} />);
    expect(screen.getByLabelText('Название')).toHaveValue('Молоко');
    expect(screen.getByLabelText('Количество')).toHaveValue(1.5);
    await user.click(screen.getByLabelText('Отметить как купленный'));
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Молоко',
        isMarked: false,
        quantity: 1.5,
      }),
    );
  });
});
