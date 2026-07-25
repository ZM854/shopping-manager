import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../../hooks/useAuth";
import AuthForm from "../../components/AuthForm/AuthForm";
import { useState } from "react";
import type { LoginRequest } from "../../models/auth";
import cls from "./LoginPage.module.css";

const LoginPage = () => {
  const { login } = useAuth();

  const navigate = useNavigate();
  const location = useLocation();

  const [error, setError] = useState<string | null>(null);

  const registrationSuccess = location.state?.registered;

  const email = location.state?.email;

  const handleSubmit = async (data: LoginRequest) => {
    try {
      setError(null);
      await login(data);

      const from = location.state?.from?.pathname ?? "/";

      navigate(from, {
        replace: true,
      });
    } catch (error) {
      if (error instanceof Error) {
        setError(error.message);
      } else {
        setError("Не удалось выполнить вход");
      }
    }
  };

  return (
    <div className={cls.container}>
      {registrationSuccess && email && (
        <div className={cls.info}>
          Мы отправили письмо с подтверждением на {email}
        </div>
      )}
      <AuthForm mode="login" error={error} onSubmit={handleSubmit} />
    </div>
  );
};

export default LoginPage;
