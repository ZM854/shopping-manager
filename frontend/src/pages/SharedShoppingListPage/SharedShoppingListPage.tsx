import { useEffect, useRef, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import type { LoginRequest } from '../../models/auth';
import { useAuth } from '../../hooks/useAuth';
import { useSubmission } from '../../hooks/useSubmission';
import { useSessionActionGuard } from '../../hooks/useSessionActionGuard';
import { errorMessage } from '../../hooks/useResource';
import { readShareFragment } from '../../shared/shoppingSharing';
import { InputError } from '../../shared/shoppingValidation';
import { ApiError } from '../../services/api';
import { shoppingService } from '../../services/shoppingService';
import AuthForm from '../../components/AuthForm/AuthForm';
import ActivationResendForm from '../../components/ActivationResendForm/ActivationResendForm';
import Modal from '../../components/UI/modal/Modal/Modal';
import Button from '../../components/UI/button/Button/Button';
import AlertDialog from '../../components/AlertDialog/AlertDialog';
import cls from './SharedShoppingListPage.module.css';

export default function SharedShoppingListPage() {
  const location = useLocation();
  const token = readShareFragment(location.hash);
  return (
    <main className={cls.page}>
      <h1>Получение списка</h1>
      {token ? (
        <SharedListSession key={location.key} token={token} />
      ) : (
        <div className={cls.card}>
          <p role="alert">
            Некорректная ссылка. Откройте полную ссылку, полученную от автора
            списка.
          </p>
          <Link to="/lists" replace>
            К спискам
          </Link>
        </div>
      )}
    </main>
  );
}

function SharedListSession({ token }: { token: string }) {
  const { user, status, operation, scope } = useAuth();
  if (operation === 'logout') return <p role="status">Выполняется выход…</p>;
  if (user) {
    // Сохранять попытку при временной ошибке refresh; смена identity сбрасывает её.
    return (
      <ImportConfirmation
        key={`${scope}:${user.id}`}
        token={token}
        ready={status === 'authenticated' && !operation}
      />
    );
  }
  if (status === 'guest' || operation === 'login') return <SharingLogin />;
  return (
    <div className={cls.card}>
      <p role="status">
        {status === 'error'
          ? 'Для продолжения повторите проверку сессии.'
          : 'Проверяем сессию…'}
      </p>
      <Link to="/lists" replace>
        Отмена
      </Link>
    </div>
  );
}

function SharingLogin() {
  const { login, operation } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const active = useRef(false);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  async function submit(data: LoginRequest) {
    setError(null);
    try {
      await login(data);
    } catch (error) {
      if (active.current) setError(errorMessage(error));
    }
  }

  return (
    <>
      <p>
        Войдите, чтобы добавить независимую копию списка. После входа
        потребуется подтверждение.
      </p>
      <AuthForm
        mode="login"
        error={error}
        loading={operation === 'login'}
        onSubmit={submit}
        footer={
          <>
            Нет аккаунта?{' '}
            <Link to="/registration" target="_blank" rel="noopener noreferrer">
              Зарегистрироваться в новой вкладке
            </Link>
          </>
        }
      />
      <ActivationResendForm />
      <Link to="/lists" replace>
        Отмена
      </Link>
    </>
  );
}

function ImportConfirmation({
  token,
  ready,
}: {
  token: string;
  ready: boolean;
}) {
  const [attempt, setAttempt] = useState(0);
  return (
    <ImportAttempt
      key={attempt}
      token={token}
      ready={ready}
      onNew={() => setAttempt(attempt + 1)}
    />
  );
}

function ImportAttempt({
  token,
  ready,
  onNew,
}: {
  token: string;
  ready: boolean;
  onNew: () => void;
}) {
  const submission = useSubmission();
  const guard = useSessionActionGuard();
  const navigate = useNavigate();
  const body = useRef({ token });
  const [failure, setFailure] = useState<
    'unavailable' | 'deleted' | 'conflict' | null
  >(null);

  function cancel() {
    if (!submission.pending) navigate('/lists', { replace: true });
  }

  function confirm() {
    if (!ready) return;
    void submission.submit(async (key) => {
      const check = guard();
      try {
        const list = await shoppingService.importShare(body.current, key);
        check();
        navigate(`/lists/${encodeURIComponent(list.id)}`, { replace: true });
      } catch (error) {
        check();
        if (error instanceof ApiError) {
          if (error.status === 404) {
            setFailure('unavailable');
            throw new InputError(
              'Ссылка недоступна. Проверьте её у отправителя.',
            );
          }
          if (error.code === 'resource_deleted') {
            setFailure('deleted');
            throw new InputError(
              'Копия, созданная этой попыткой, уже удалена. Можно подтвердить добавление новой копии.',
            );
          }
          if (error.code === 'idempotency_conflict') {
            setFailure('conflict');
            throw new InputError(
              'Эта попытка конфликтует с предыдущим действием. Подтвердите новое добавление.',
            );
          }
        }
        throw error;
      }
    });
  }

  if (!ready)
    return <p role="status">Для продолжения повторите проверку сессии.</p>;
  if (failure)
    return (
      <div className={cls.card}>
        <p role="alert">{submission.error}</p>
        {failure !== 'unavailable' && (
          <Button onClick={onNew}>Добавить новую копию</Button>
        )}
        <Button variant="outlined" onClick={cancel}>
          К спискам
        </Button>
      </div>
    );
  return (
    <>
      <p>Добавление происходит только после вашего подтверждения.</p>
      <Modal isOpen onClose={cancel} ariaLabel="Добавление копии списка">
        <AlertDialog
          title="Добавить копию списка в ваши списки?"
          message="Добавится отдельная копия из снимка по ссылке. Вы сможете редактировать её самостоятельно. Каждое новое подтверждение после открытия ссылки может создать ещё одну копию."
          onDiscard={cancel}
          onConfirm={confirm}
          discardText="Отмена"
          confirmText={submission.retry ? 'Повторить' : 'Добавить'}
          pending={submission.pending}
          error={submission.error}
        />
        {submission.retry && (
          <p className={cls.retry}>
            Результат неизвестен. «Повторить» отправит то же действие. Уход со
            страницы не отменяет уже принятый сервером запрос.
          </p>
        )}
      </Modal>
    </>
  );
}
