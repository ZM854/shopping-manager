import type {
  AuthResponse,
  LoginRequest,
  RegistrationRequest,
  RegistrationResponse,
} from '../models/auth';

import { apiFetch } from './api';

export class AuthService {
  async login(data: LoginRequest): Promise<AuthResponse> {
    return apiFetch<AuthResponse>('/login', {
      method: 'POST',
      body: JSON.stringify(data),
      skipRefresh: true,
    });
  }

  async register(data: RegistrationRequest): Promise<RegistrationResponse> {
    return apiFetch<RegistrationResponse>('/registration', {
      method: 'POST',
      body: JSON.stringify(data),
      skipRefresh: true,
    });
  }

  async refresh(): Promise<AuthResponse> {
    return apiFetch<AuthResponse>('/refresh', {
      method: 'POST',
      skipRefresh: true,
    });
  }

  async activate(token: string): Promise<void> {
    return apiFetch<void>('/activation', {
      method: 'POST',
      body: JSON.stringify({ token }),
      skipRefresh: true,
    });
  }

  async resendActivation(email: string): Promise<{ message: string }> {
    return apiFetch<{ message: string }>('/activation/resend', {
      method: 'POST',
      body: JSON.stringify({ email }),
      skipRefresh: true,
    });
  }

  async logout(): Promise<void> {
    return apiFetch<void>('/logout', {
      method: 'POST',
    });
  }
}

export const authService = new AuthService();
