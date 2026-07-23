import type {
  AuthResponse,
  LoginRequest,
  RegistrationRequest,
} from "../models/auth";

import { apiFetch } from "./api";

export class AuthService {
  async login(data: LoginRequest): Promise<AuthResponse> {
    return apiFetch<AuthResponse>("/login", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async register(data: RegistrationRequest): Promise<AuthResponse> {
    return apiFetch<AuthResponse>("/registration", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async refresh(): Promise<AuthResponse> {
    return apiFetch<AuthResponse>("/refresh", {
      method: "POST",
    });
  }

  async activate(token: string): Promise<void> {
    return apiFetch<void>(`/activation/${token}`, {
      method: "GET",
    });
  }

  async logout(): Promise<void> {
    return apiFetch<void>("/logout", {
      method: "POST",
    });
  }
}

export const authService = new AuthService();
