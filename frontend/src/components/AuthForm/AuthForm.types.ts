import type { LoginRequest } from "../../models/auth";

export type AuthMode = "login" | "registration";

export interface AuthFormProps {
  mode: AuthMode;

  loading?: boolean;

  error?: string | null;

  onSubmit(data: LoginRequest): Promise<void>;
}
