import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { authService } from '../../services/authService';
import Button from '../UI/button/Button/Button';
import TextField from '../UI/input/TextField/TextField';
import cls from './ActivationResendForm.module.css';

export default function ActivationResendForm({
  initialEmail = '',
}: {
  initialEmail?: string;
}) {
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const {
    register,
    handleSubmit,
    formState: { isSubmitting, errors },
  } = useForm<{ email: string }>({ defaultValues: { email: initialEmail } });

  const submit = async ({ email }: { email: string }) => {
    setMessage('');
    setError('');
    try {
      const response = await authService.resendActivation(email);
      setMessage(response.message);
    } catch (error) {
      setError(
        error instanceof Error ? error.message : 'Не удалось запросить письмо',
      );
    }
  };

  return (
    <form
      className={cls.form}
      aria-label="Повторное письмо"
      onSubmit={handleSubmit(submit)}
    >
      <h2>Не пришло письмо?</h2>
      <p>
        Запросите новую ссылку для подтверждения почты. Между отправками нужна
        пауза.
      </p>
      <TextField
        label="Почта для подтверждения"
        type="email"
        autoComplete="email"
        maxLength={254}
        error={errors.email?.message}
        {...register('email', { required: 'Введите адрес почты' })}
      />
      {message && <p role="status">{message}</p>}
      {error && (
        <p role="alert" className={cls.error}>
          {error}
        </p>
      )}
      <Button type="submit" variant="outlined" disabled={isSubmitting}>
        {isSubmitting ? 'Отправляем…' : 'Отправить письмо повторно'}
      </Button>
    </form>
  );
}
