import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthProvider } from './AuthProvider';
import { useAuth } from '../../hooks/useAuth';
import { authService } from '../../services/authService';
import { clearAccessToken, getAccessToken } from '../../services/tokenStorage';
vi.mock('../../services/authService', () => ({
  authService: {
    refresh: vi.fn(),
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  },
}));
vi.mock('../../shared/logger', () => ({
  logger: { info: vi.fn(), debug: vi.fn(), error: vi.fn() },
}));
const service = vi.mocked(authService);
const response = {
  user: {
    id: 1,
    name: 'Имя',
    email: 'user@example.test',
    isEmailVerified: true,
  },
  accessToken: 'test-access',
  refreshToken: 'test-refresh',
};
beforeEach(() => {
  vi.resetAllMocks();
  clearAccessToken();
});
describe('AuthProvider', () => {
  it('загружает сессию и очищает её после выхода даже при отказе сервера', async () => {
    service.refresh.mockResolvedValue(response);
    service.logout.mockRejectedValue(new Error('offline'));
    const { result } = renderHook(useAuth, { wrapper: AuthProvider });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.user).toEqual(response.user);
    expect(result.current.isAuthenticated).toBe(true);
    expect(getAccessToken()).toBe('test-access');
    await act(async () => {
      await result.current.logout();
    });
    expect(result.current.user).toBeNull();
    expect(getAccessToken()).toBeNull();
  });
  it('завершает начальную загрузку без сессии и обрабатывает успешный вход', async () => {
    service.refresh.mockRejectedValue(new Error('no session'));
    service.login.mockResolvedValue(response);
    const { result } = renderHook(useAuth, { wrapper: AuthProvider });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isAuthenticated).toBe(false);
    const credentials = {
      email: 'user@example.test',
      password: 'test-password',
    };
    await act(async () => {
      await result.current.login(credentials);
    });
    expect(service.login).toHaveBeenCalledWith(credentials);
    expect(result.current.user).toEqual(response.user);
    service.register.mockResolvedValue(response);
    const data = { ...credentials, name: 'Имя' };
    await act(async () => {
      await result.current.register(data);
    });
    expect(service.register).toHaveBeenCalledWith(data);
    service.refresh.mockResolvedValue({
      ...response,
      accessToken: 'new-access',
    });
    await act(async () => {
      await result.current.refresh();
    });
    expect(getAccessToken()).toBe('new-access');
  });
  it('useAuth требует провайдера', () => {
    expect(() => renderHook(useAuth)).toThrow('useAuth');
  });
});
