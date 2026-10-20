import type {
  AuthResponse,
  LoginRequest,
  RegistrationRequest,
  RegistrationResponse,
} from '../models/auth';

import { requestJSON } from './http';

export class AuthService {
  async login(data: LoginRequest): Promise<AuthResponse> {
    return requestJSON<AuthResponse>('/login', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async register(data: RegistrationRequest): Promise<RegistrationResponse> {
    return requestJSON<RegistrationResponse>('/registration', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  refresh(): Promise<AuthResponse> {
    return requestJSON<AuthResponse>('/refresh', { method: 'POST' });
  }

  async activate(token: string): Promise<void> {
    return requestJSON<void>('/activation', {
      method: 'POST',
      body: JSON.stringify({ token }),
    });
  }

  async resendActivation(email: string): Promise<{ message: string }> {
    return requestJSON<{ message: string }>('/activation/resend', {
      method: 'POST',
      body: JSON.stringify({ email }),
    });
  }

  async logout(): Promise<void> {
    return requestJSON<void>('/logout', {
      method: 'POST',
    });
  }
}

export const authService = new AuthService();
