import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import Modal from './Modal';
describe('Modal', () => {
  it('управляет фокусом, Escape и кликами внутри/снаружи', async () => {
    const close = vi.fn();
    const user = userEvent.setup({ delay: null });
    const trigger = document.createElement('button');
    document.body.append(trigger);
    trigger.focus();
    try {
      const { rerender, unmount } = render(
        <Modal isOpen onClose={close} ariaLabel="Товар">
          <button>Внутри</button>
        </Modal>,
      );
      const dialog = screen.getByRole('dialog', { name: 'Товар' });
      expect(dialog).toHaveFocus();
      expect(dialog).toHaveAttribute('aria-modal', 'true');
      await user.click(screen.getByRole('button', { name: 'Внутри' }));
      expect(close).not.toHaveBeenCalled();
      await user.keyboard('{Escape}');
      expect(close).toHaveBeenCalledTimes(1);
      await user.click(dialog.parentElement!);
      expect(close).toHaveBeenCalledTimes(2);
      rerender(
        <Modal isOpen={false} onClose={close}>
          Текст
        </Modal>,
      );
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(trigger).toHaveFocus();
      unmount();
      await user.keyboard('{Escape}');
      expect(close).toHaveBeenCalledTimes(2);
    } finally {
      trigger.remove();
    }
  });
});
