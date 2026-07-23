import { createContext } from "react";

import type { LoginRequest, RegistrationRequest } from "../models/auth";

import type { User } from "../models/user";

export interface AuthContextValue {
  user: User | null;

  isAuthenticated: boolean;

  isLoading: boolean;

  login(data: LoginRequest): Promise<void>;

  register(data: RegistrationRequest): Promise<void>;

  logout(): Promise<void>;

  refresh(): Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);
