import type { User } from "./user";

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegistrationRequest {
  name: string;
  email: string;
  password: string;
}

export interface AuthResponse {
  user: User;
  accessToken: string;
  refreshToken: string;
}
