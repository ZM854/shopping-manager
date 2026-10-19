import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { authService } from '../../services/authService';
import { ApiError } from '../../services/api';
import Button from '../../components/UI/button/Button/Button';
import ActivationResendForm from '../../components/ActivationResendForm/ActivationResendForm';
import cls from './ActivationPage.module.css';

export default function ActivationPage() {
  const { token: urlToken } = useParams();
  const [token, setToken] = useState(urlToken ?? '');
  const [status, setStatus] = useState<
    'ready' | 'pending' | 'success' | 'invalid' | 'error'
  >('ready');
  const [error, setError] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    if (urlToken) navigate('/activation', { replace: true });
  }, [urlToken, navigate]);

  const activate = async () => {
    if (status === 'pending') return;
    setStatus('pending');
    setError('');
    try {
      await authService.activate(token);
      setToken('');
      setStatus('success');
    } catch (error) {
      setStatus(
        error instanceof ApiError && error.status === 400 ? 'invalid' : 'error',
      );
      setError(
        error instanceof Error ? error.message : 'Не удалось подтвердить почту',
      );
    }
  };

  return (
    <main className={cls.container}>
      <section className={cls.card}>
        <h1>Подтверждение почты</h1>
        {status === 'success' ? (
          <>
            <p role="status">
              Почта подтверждена. Теперь можно войти в аккаунт.
            </p>
            <Link to="/login" state={{ activated: true }}>
              Перейти ко входу
            </Link>
          </>
        ) : (
          <>
            <p>
              {status === 'invalid'
                ? 'Запросите новое письмо для подтверждения почты.'
                : token
                  ? 'Нажмите кнопку, чтобы подтвердить почту.'
                  : 'Откройте ссылку из письма или запросите новое письмо ниже.'}
            </p>
            {error && (
              <p role="alert" className={cls.error}>
                {error}
              </p>
            )}
            {token && status !== 'invalid' && (
              <Button onClick={activate} disabled={status === 'pending'}>
                {status === 'pending' ? 'Подтверждаем…' : 'Подтвердить почту'}
              </Button>
            )}
            <Link to="/login">Вернуться ко входу</Link>
          </>
        )}
      </section>
      {status !== 'success' && <ActivationResendForm />}
    </main>
  );
}
