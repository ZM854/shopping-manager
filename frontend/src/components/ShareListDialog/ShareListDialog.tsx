import { useCallback, useRef, useState } from 'react';
import type { CreateShoppingListShareRequest } from '../../models/shoppingList';
import { useSubmission } from '../../hooks/useSubmission';
import { useSessionActionGuard } from '../../hooks/useSessionActionGuard';
import { useAuth } from '../../hooks/useAuth';
import { shoppingService } from '../../services/shoppingService';
import { createShareToken, shareURL } from '../../shared/shoppingSharing';
import Modal from '../UI/modal/Modal/Modal';
import Button from '../UI/button/Button/Button';
import Checkbox from '../UI/input/Checkbox/Checkbox';
import TextField from '../UI/input/TextField/TextField';
import cls from './ShareListDialog.module.css';

type Props = { listId: string; onClose: () => void };

export default function ShareListDialog(props: Props) {
  const { operation } = useAuth();
  const [attempt, setAttempt] = useState(0);
  if (operation) return null;
  return (
    <ShareListForm
      key={attempt}
      {...props}
      onNew={() => setAttempt(attempt + 1)}
    />
  );
}

function ShareListForm({
  listId,
  onClose,
  onNew,
}: Props & { onNew: () => void }) {
  const submission = useSubmission();
  const guard = useSessionActionGuard();
  const [preserveMarks, setPreserveMarks] = useState(false);
  const [url, setURL] = useState<string | null>(null);
  const [copyNotice, setCopyNotice] = useState<string | null>(null);
  const [copying, setCopying] = useState(false);
  const attempt = useRef<{
    key: string;
    body: CreateShoppingListShareRequest;
  } | null>(null);
  const close = useCallback(() => {
    if (!submission.pending) onClose();
  }, [submission.pending, onClose]);

  function create() {
    void submission.submit(async (key) => {
      const check = guard();
      if (attempt.current?.key !== key) {
        attempt.current = {
          key,
          body: { token: createShareToken(), preserveMarks },
        };
      }
      const { body } = attempt.current;
      await shoppingService.createShare(listId, body, key);
      check();
      setURL(shareURL(body.token));
    });
  }

  async function copy() {
    if (!url || copying) return;
    let check: () => void;
    try {
      check = guard();
    } catch {
      return;
    }
    setCopying(true);
    let message = 'Ссылка скопирована.';
    try {
      if (!navigator.clipboard?.writeText)
        throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(url);
    } catch {
      message =
        'Не удалось скопировать автоматически. Выделите ссылку и скопируйте её вручную.';
    }
    try {
      check();
      setCopyNotice(message);
      setCopying(false);
    } catch {
      // Закрытый диалог или сменившаяся сессия больше не получают результат.
    }
  }

  return (
    <Modal isOpen onClose={close} ariaLabel="Поделиться списком">
      <div className={cls.content}>
        <h2>Поделиться списком</h2>
        <p>
          Ссылка сохранит снимок списка. Получатель добавит отдельную копию и
          сможет редактировать её самостоятельно.
        </p>
        {url ? (
          <>
            <p role="status">
              Ссылка готова. Последующие изменения исходного списка её не
              изменят.
            </p>
            <TextField
              label="Ссылка на копию списка"
              value={url}
              readOnly
              autoComplete="off"
              spellCheck={false}
              onFocus={(event) => event.currentTarget.select()}
              onClick={(event) => event.currentTarget.select()}
            />
            <p>
              Ссылка действует без срока. Любой её обладатель после входа сможет
              добавить копию.
            </p>
            <p>
              {preserveMarks
                ? 'Отметки покупки сохранены.'
                : 'Отметки покупки в копии сброшены.'}
            </p>
            <Button onClick={() => void copy()} disabled={copying}>
              {copying ? 'Копирование…' : 'Копировать ссылку'}
            </Button>
            {copyNotice && <p role="status">{copyNotice}</p>}
            <Button variant="outlined" onClick={onNew} disabled={copying}>
              Создать новую ссылку
            </Button>
          </>
        ) : (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              create();
            }}
            className={cls.form}
          >
            <Checkbox
              label="Сохранить отметки покупки"
              checked={preserveMarks}
              disabled={submission.pending || submission.retry}
              onChange={(event) => setPreserveMarks(event.target.checked)}
            />
            {submission.error && <p role="alert">{submission.error}</p>}
            {submission.retry && (
              <p>
                Результат запроса неизвестен. Повтор отправит прежнее действие с
                теми же настройками.
              </p>
            )}
            <Button type="submit" disabled={submission.pending}>
              {submission.pending
                ? 'Создание…'
                : submission.retry
                  ? 'Повторить'
                  : 'Создать ссылку'}
            </Button>
          </form>
        )}
        <Button
          variant="outlined"
          onClick={close}
          disabled={submission.pending}
        >
          Закрыть
        </Button>
      </div>
    </Modal>
  );
}
