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
}

const AlertDialog = ({
  title,
  message,
  onConfirm,
  onDiscard,
  discardText = 'Отменить',
  confirmText = 'Продолжить',
  isDanger = false,
}: AlertDialogProps) => {
  return (
    <div className={cls.alertDialog}>
      <h2 className={cls.title}>{title}</h2>
      {message && <p className={cls.message}>{message}</p>}
      <div className={cls.buttons}>
        <Button variant="outlined" onClick={onDiscard}>
          {discardText}
        </Button>
        <Button tone={isDanger ? 'danger' : 'default'} onClick={onConfirm}>
          {confirmText}
        </Button>
      </div>
    </div>
  );
};

export default AlertDialog;
