import { useState } from 'react';
import type {
  CreateShoppingItemRequest,
  ShoppingItem,
} from '../../models/shoppingList';
import { useSubmission } from '../../hooks/useSubmission';
import {
  normalizeQuantity,
  normalizeText,
} from '../../shared/shoppingValidation';
import cls from './ProductForm.module.css';
import Checkbox from '../UI/input/Checkbox/Checkbox';
import TextField from '../UI/input/TextField/TextField';
import Button from '../UI/button/Button/Button';

type ProductFormProps = {
  product: ShoppingItem | null;
  onSave: (data: CreateShoppingItemRequest, key: string) => Promise<void>;
};

const ProductForm = ({ product, onSave }: ProductFormProps) => {
  const [name, setName] = useState(product?.name ?? '');
  const [quantity, setQuantity] = useState(product?.quantity ?? '');
  const [unit, setUnit] = useState(product?.unit ?? '');
  const [isMarked, setIsMarked] = useState(product?.isMarked ?? false);
  const submission = useSubmission();

  return (
    <form
      className={cls.form}
      onSubmit={(event) => {
        event.preventDefault();
        void submission.submit(async (key) => {
          const data = {
            name: normalizeText(name, 200),
            quantity: normalizeQuantity(quantity),
            unit: normalizeText(unit, 32, false),
            isMarked,
          };
          await onSave(data, key);
        });
      }}
    >
      <fieldset disabled={submission.pending || submission.retry}>
        <TextField
          label="Название"
          value={name}
          required
          error={submission.fieldErrors.name}
          onChange={(event) => setName(event.target.value)}
        />
        <TextField
          label="Количество"
          type="text"
          inputMode="decimal"
          placeholder="Необязательно, например 0.125"
          value={quantity}
          error={submission.fieldErrors.quantity}
          onChange={(event) => setQuantity(event.target.value)}
        />
        <TextField
          label="Единица измерения"
          value={unit}
          error={submission.fieldErrors.unit}
          onChange={(event) => setUnit(event.target.value)}
        />
        {product && (
          <Checkbox
            label="Отметить как купленный"
            checked={isMarked}
            onChange={(event) => setIsMarked(event.target.checked)}
          />
        )}
      </fieldset>
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
};

export default ProductForm;
