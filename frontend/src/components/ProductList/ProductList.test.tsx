import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ProductList from './ProductList';
describe('Список и карточки позиций', () => {
  it('различает пустую коллекцию и ошибку', () => {
    const props = {
      products: [],
      error: null,
      editProduct: vi.fn(),
      updateProduct: vi.fn(),
      deleteProduct: vi.fn(),
    };
    const { rerender } = render(<ProductList {...props} />);
    expect(screen.getByText('Список покупок пока пуст.')).toBeVisible();
    rerender(<ProductList {...props} error="offline" />);
    expect(screen.getByRole('alert')).toHaveTextContent('Не удалось загрузить');
    expect(
      screen.queryByText('Список покупок пока пуст.'),
    ).not.toBeInTheDocument();
  });
  it('передаёт ID нужной позиции при отметке, редактировании и удалении', async () => {
    const product = {
      id: 7,
      name: 'Молоко',
      quantity: 1.5,
      unit: 'л',
      isMarked: false,
    };
    const edit = vi.fn(),
      update = vi.fn(),
      remove = vi.fn();
    const user = userEvent.setup({ delay: null });
    render(
      <ProductList
        products={[product]}
        error={null}
        editProduct={edit}
        updateProduct={update}
        deleteProduct={remove}
      />,
    );
    expect(screen.getByText('1.5 л')).toBeVisible();
    await user.click(screen.getByRole('checkbox'));
    expect(update).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ isMarked: true }),
    );
    await user.click(
      screen.getByRole('button', { name: 'Редактировать Молоко' }),
    );
    expect(edit).toHaveBeenCalledWith(product);
    await user.click(screen.getByRole('button', { name: 'Удалить Молоко' }));
    expect(remove).toHaveBeenCalledWith(7);
  });
});
