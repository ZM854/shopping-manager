import { useState } from 'react';
import { useSubmission } from '../../hooks/useSubmission';
import { normalizeText } from '../../shared/shoppingValidation';
import Button from '../UI/button/Button/Button';
import TextField from '../UI/input/TextField/TextField';

type Props = {
  initialName?: string;
  onSave: (name: string, key: string) => Promise<void>;
};

export default function ListForm({ initialName = '', onSave }: Props) {
  const [name, setName] = useState(initialName);
  const submission = useSubmission();
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void submission.submit(async (key) => {
          await onSave(normalizeText(name, 100), key);
        });
      }}
    >
      <TextField
        label="Название списка"
        value={name}
        required
        disabled={submission.pending || submission.retry}
        error={submission.fieldErrors.name}
        onChange={(event) => setName(event.target.value)}
      />
      {submission.error && <p role="alert">{submission.error}</p>}
      {submission.retry && (
        <p>Повторите отправку того же действия. Данные сохранены.</p>
      )}
      <Button type="submit" disabled={submission.pending}>
        {submission.pending
          ? 'Сохранение…'
          : submission.retry
            ? 'Повторить'
            : 'Сохранить'}
      </Button>
    </form>
  );
}
