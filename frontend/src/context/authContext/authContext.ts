import { createContext } from 'react';

import type { LoginRequest, RegistrationRequest } from '../../models/auth.ts';

import type { SessionSnapshot } from '../../services/session';

export interface AuthContextValue extends SessionSnapshot {
  isAuthenticated: boolean;

  isLoading: boolean;

  login(data: LoginRequest): Promise<void>;

  register(data: RegistrationRequest): Promise<void>;

  logout(): Promise<void>;

  refresh(): Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);
