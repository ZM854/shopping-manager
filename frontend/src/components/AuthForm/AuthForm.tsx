import { Link } from "react-router-dom";
import { useForm } from "react-hook-form";
import type { LoginRequest, RegistrationRequest } from "../../models/auth";
import type { AuthFormData, AuthFormProps } from "./AuthForm.types";
import Button from "../UI/button/Button/Button";
import cls from "./AuthForm.module.css";
import TextField from "../UI/input/TextField/TextField";

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

  const isLogin = mode === "login";

  const handleFormSubmit = (data: AuthFormData) => {
    if (isLogin) {
      onSubmit(data as LoginRequest);
    } else {
      onSubmit(data as RegistrationRequest);
    }
  };

  return (
    <div className={cls.container}>
      <div className={cls.card}>
        <h1 className={cls.title}>{isLogin ? "Вход" : "Регистрация"}</h1>

        <form className={cls.form} onSubmit={handleSubmit(handleFormSubmit)}>
          {!isLogin && (
            <TextField
              label="Имя"
              type="text"
              autoComplete="given-name"
              error={errors.name?.message}
              {...register("name", {
                required: "Введите имя",

                minLength: {
                  value: 2,
                  message: "Минимум 2 символа",
                },

                maxLength: {
                  value: 20,
                  message: "Максимум 20 символов",
                },
              })}
            />
          )}
          <TextField
            label="Email"
            type="email"
            autoComplete="email"
            error={errors.email?.message}
            {...register("email", {
              required: "Введите email",
              pattern: {
                value: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
                message: "Некорректный email",
              },
            })}
          />
          <TextField
            label="Пароль"
            type="password"
            autoComplete={isLogin ? "current-password" : "new-password"}
            error={errors.password?.message}
            {...register("password", {
              required: "Введите пароль",

              minLength: {
                value: 6,
                message: "Минимум 6 символов",
              },

              maxLength: {
                value: 72,
                message: "Максимум 72 символа",
              },
            })}
          />
          {error && <p className={cls.error}>{error}</p>}
          <Button type="submit" disabled={loading || isSubmitting}>
            {isLogin ? "Войти" : "Зарегистрироваться"}
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
