import type { LoginRequest, RegistrationRequest } from '../../models/auth';
import type { ReactNode } from 'react';

export type AuthMode = 'login' | 'registration';

type BaseFormProps = {
  footer?: ReactNode;
  loading?: boolean;
  error?: string | null;
};

type LoginFormProps = {
  mode: 'login';
  onSubmit: (data: LoginRequest) => Promise<void>;
} & BaseFormProps;

type RegistrationFormProps = {
  mode: 'registration';
  onSubmit: (data: RegistrationRequest) => Promise<void>;
} & BaseFormProps;

export type AuthFormProps = LoginFormProps | RegistrationFormProps;

export type AuthFormData = LoginRequest & {
  name?: string;
};
