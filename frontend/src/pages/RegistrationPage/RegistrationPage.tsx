import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../hooks/useAuth';
import AuthForm from '../../components/AuthForm/AuthForm';
import cls from './RegistrationPage.module.css';
import type { RegistrationRequest } from '../../models/auth';
import { ApiError } from '../../services/api';
import ActivationResendForm from '../../components/ActivationResendForm/ActivationResendForm';

export default function RegistrationPage() {
  const { register } = useAuth();

  const navigate = useNavigate();

  const [error, setError] = useState<string | null>(null);
  const [resendEmail, setResendEmail] = useState<string | null>(null);

  const handleSubmit = async (data: RegistrationRequest) => {
    try {
      setError(null);
      setResendEmail(null);

      await register(data);

      navigate('/login', {
        replace: true,
        state: {
          email: data.email,
          registered: true,
        },
      });
    } catch (error) {
      if (
        error instanceof ApiError &&
        error.code === 'activation_delivery_failed'
      ) {
        setResendEmail(data.email);
      }
      if (error instanceof Error) {
        setError(error.message);
      } else {
        setError('Не удалось зарегистрироваться');
      }
    }
  };

  return (
    <div className={cls.container}>
      <AuthForm mode="registration" error={error} onSubmit={handleSubmit} />
      {resendEmail && (
        <ActivationResendForm key={resendEmail} initialEmail={resendEmail} />
      )}
    </div>
  );
}
