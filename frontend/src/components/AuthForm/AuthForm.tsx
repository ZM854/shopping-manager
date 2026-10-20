import { Link } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import type { LoginRequest, RegistrationRequest } from '../../models/auth';
import type { AuthFormData, AuthFormProps } from './AuthForm.types';
import Button from '../UI/button/Button/Button';
import cls from './AuthForm.module.css';
import TextField from '../UI/input/TextField/TextField';
import {
  trimAuth,
  normalizeEmail,
  nameError,
  emailError,
  passwordError,
} from '../../shared/authValidation';

export default function AuthForm({
  mode,
  loading = false,
  error,
  onSubmit,
}: AuthFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<AuthFormData>();

  const isLogin = mode === 'login';

  const handleFormSubmit = (data: AuthFormData) => {
    if (isLogin) {
      return onSubmit(data as LoginRequest);
    } else {
      return onSubmit(data as RegistrationRequest);
    }
  };

  return (
    <div className={cls.container}>
      <div className={cls.card}>
        <h1 className={cls.title}>{isLogin ? 'Вход' : 'Регистрация'}</h1>

        <form
          noValidate
          className={cls.form}
          onSubmit={handleSubmit(handleFormSubmit)}
        >
          {!isLogin && (
            <TextField
              label="Имя"
              type="text"
              autoComplete="given-name"
              error={errors.name?.message}
              {...register('name', {
                required: 'Введите имя',
                setValueAs: trimAuth,
                validate: (value) => nameError(value || ''),
              })}
            />
          )}
          <TextField
            label="Email"
            type="email"
            autoComplete="email"
            error={errors.email?.message}
            {...register('email', {
              required: 'Введите email',
              setValueAs: normalizeEmail,
              validate: emailError,
            })}
          />
          <TextField
            label="Пароль"
            type="password"
            autoComplete={isLogin ? 'current-password' : 'new-password'}
            error={errors.password?.message}
            {...register('password', {
              required: 'Введите пароль',

              validate: (value) => passwordError(value, !isLogin),
            })}
          />
          {error && (
            <p role="alert" className={cls.serverError}>
              {error}
            </p>
          )}
          <Button type="submit" disabled={loading || isSubmitting}>
            {isLogin ? 'Войти' : 'Зарегистрироваться'}
          </Button>
        </form>

        <div className={cls.footer}>
          {isLogin ? (
            <>
              Нет аккаунта?
              <Link to="/registration">Зарегистрироваться</Link>
            </>
          ) : (
            <>
              Уже зарегистрированы?
              <Link to="/login">Войти</Link>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
