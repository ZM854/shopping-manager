import Button from '../UI/button/Button/Button.tsx';
import cls from './AlertDialog.module.css';
interface AlertDialogProps {
  title: string;
  message?: string;
  onDiscard: () => void;
  onConfirm: () => void;
  discardText?: string;
  confirmText?: string;
  isDanger?: boolean;
  pending?: boolean;
  error?: string | null;
}

const AlertDialog = ({
  title,
  message,
  onConfirm,
  onDiscard,
  discardText = 'Отменить',
  confirmText = 'Продолжить',
  isDanger = false,
  pending = false,
  error,
}: AlertDialogProps) => {
  return (
    <div className={cls.alertDialog}>
      <h2 className={cls.title}>{title}</h2>
      {message && <p className={cls.message}>{message}</p>}
      {error && <p role="alert">{error}</p>}
      <div className={cls.buttons}>
        <Button variant="outlined" onClick={onDiscard} disabled={pending}>
          {discardText}
        </Button>
        <Button
          tone={isDanger ? 'danger' : 'default'}
          onClick={onConfirm}
          disabled={pending}
        >
          {pending ? 'Выполнение…' : confirmText}
        </Button>
      </div>
    </div>
  );
};

export default AlertDialog;
