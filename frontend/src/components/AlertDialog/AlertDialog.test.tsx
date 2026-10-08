import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import AlertDialog from './AlertDialog';
it('подтверждение и отмена вызывают разные обработчики', async () => {
  const confirm = vi.fn(),
    discard = vi.fn();
  const user = userEvent.setup({ delay: null });
  render(
    <AlertDialog
      title="Очистить список?"
      message="Удалить позиции"
      onConfirm={confirm}
      onDiscard={discard}
      confirmText="Удалить"
      isDanger
    />,
  );
  await user.click(screen.getByRole('button', { name: 'Отменить' }));
  expect(discard).toHaveBeenCalledOnce();
  expect(confirm).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Удалить' }));
  expect(confirm).toHaveBeenCalledOnce();
});
