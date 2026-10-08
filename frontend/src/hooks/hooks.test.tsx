import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useModal } from './useModal';
import { useScaffold, useScaffoldState } from './useScaffold';
import { ScaffoldActionContext } from '../context/scaffoldContext/scaffoldContext';
import type { PropsWithChildren } from 'react';
describe('UI hooks', () => {
  it('открывает и закрывает модальное окно', () => {
    const { result } = renderHook(useModal);
    expect(result.current.isOpen).toBe(false);
    act(() => result.current.open());
    expect(result.current.isOpen).toBe(true);
    act(() => result.current.close());
    expect(result.current.isOpen).toBe(false);
  });
  it('обновляет scaffold и очищает настройки после unmount', () => {
    const actions = { setFab: vi.fn(), setTopBar: vi.fn() };
    const wrapper = ({ children }: PropsWithChildren) => (
      <ScaffoldActionContext.Provider value={actions}>
        {children}
      </ScaffoldActionContext.Provider>
    );
    const config = {
      fab: { icon: <span>+</span>, onClick: vi.fn() },
      topBar: { title: 'Покупки' },
    };
    const { unmount } = renderHook(() => useScaffold(config), { wrapper });
    expect(actions.setFab).toHaveBeenCalledWith(config.fab);
    expect(actions.setTopBar).toHaveBeenCalledWith(config.topBar);
    unmount();
    expect(actions.setFab).toHaveBeenLastCalledWith(null);
    expect(actions.setTopBar).toHaveBeenLastCalledWith(null);
  });
  it('scaffold hooks требуют провайдеры', () => {
    expect(() => renderHook(useScaffoldState)).toThrow('ScaffoldStateProvider');
    expect(() => renderHook(() => useScaffold({}))).toThrow(
      'ScaffoldActionProvider',
    );
  });
});
