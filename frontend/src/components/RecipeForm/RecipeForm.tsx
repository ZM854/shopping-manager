import { useId, useRef, useState } from 'react';
import { useFieldArray, useForm, useWatch } from 'react-hook-form';
import type {
  Recipe,
  RecipeCategory,
  RecipeDraft,
  SaveRecipeRequest,
} from '../../models/recipe';
import { useSubmission } from '../../hooks/useSubmission';
import { useSessionActionGuard } from '../../hooks/useSessionActionGuard';
import { ApiError } from '../../services/api';
import {
  normalizeRecipe,
  normalizeStepText,
  recipeLimits,
  recipeToDraft,
} from '../../shared/recipeValidation';
import Button from '../UI/button/Button/Button';
import TextField from '../UI/input/TextField/TextField';
import cls from './RecipeForm.module.css';

type Props = {
  recipe?: Recipe;
  categories: RecipeCategory[];
  onSave: (body: SaveRecipeRequest, key: string) => Promise<Recipe>;
  onSaved: (recipe: Recipe) => void;
  onCancel: () => void;
};

const times = Array.from({ length: 288 }, (_, i) => (i + 1) * 5);

export default function RecipeForm({
  recipe,
  categories,
  onSave,
  onSaved,
  onCancel,
}: Props) {
  const { register, control, getValues } = useForm<RecipeDraft>({
    defaultValues: recipeToDraft(recipe),
  });
  const ingredients = useFieldArray({ control, name: 'ingredients' });
  const steps = useFieldArray({ control, name: 'steps' });
  const texts = useWatch({ control, name: 'steps' });
  const submission = useSubmission();
  const guard = useSessionActionGuard();
  const prefix = useId();
  const attempt = useRef<{ key: string; body: SaveRecipeRequest } | null>(null);
  const [conflict, setConflict] = useState(false);
  const [notice, setNotice] = useState('');
  const locked = submission.pending || submission.retry || conflict;
  const errors = submission.fieldErrors;
  const length = (text: string) => [...normalizeStepText(text)].length;
  const total = texts.reduce((sum, step) => sum + length(step.text), 0);

  function save() {
    if (conflict) return;
    void submission.submit(async (key) => {
      const check = guard();
      if (attempt.current?.key !== key) {
        attempt.current = {
          key,
          body: normalizeRecipe(getValues(), categories),
        };
      }
      try {
        const result = await onSave(attempt.current.body, key);
        check();
        onSaved(result);
      } catch (error) {
        check();
        if (
          error instanceof ApiError &&
          (error.code === 'resource_deleted' ||
            error.code === 'idempotency_conflict')
        )
          setConflict(true);
        throw error;
      }
    });
  }

  return (
    <form
      className={cls.form}
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <fieldset disabled={locked} className={cls.group}>
        <legend>Основные сведения</legend>
        <TextField
          label="Название рецепта"
          {...register('title')}
          error={errors.title}
          required
        />
        <label htmlFor={`${prefix}-time`}>Время приготовления</label>
        <select
          id={`${prefix}-time`}
          {...register('cookingTimeMinutes')}
          aria-invalid={!!errors.cookingTimeMinutes}
        >
          {times.map((time) => (
            <option key={time} value={time}>
              {time} мин
            </option>
          ))}
        </select>
        <p className={cls.hint}>Общее время, включая ожидание и выпечку.</p>
        {errors.cookingTimeMinutes && (
          <p role="alert">{errors.cookingTimeMinutes}</p>
        )}
        <label htmlFor={`${prefix}-category`}>Категория блюда</label>
        <select
          id={`${prefix}-category`}
          {...register('categoryCode')}
          aria-invalid={!!errors.categoryCode}
        >
          <option value="">Выберите категорию</option>
          {categories.map((category) => (
            <option key={category.code} value={category.code}>
              {category.name}
            </option>
          ))}
        </select>
        {errors.categoryCode && <p role="alert">{errors.categoryCode}</p>}
      </fieldset>
      <fieldset disabled={locked} className={cls.group}>
        <legend>Ингредиенты</legend>
        <p className={cls.hint}>
          Количество и единицу можно указать независимо или оставить пустыми.
        </p>
        {ingredients.fields.map((field, i) => (
          <fieldset key={field.id} className={cls.row}>
            <legend>Ингредиент {i + 1}</legend>
            <TextField
              label={`Название ингредиента ${i + 1}`}
              {...register(`ingredients.${i}.name`)}
              error={errors[`ingredients.${i}.name`]}
              required
            />
            <TextField
              label={`Количество ингредиента ${i + 1}`}
              {...register(`ingredients.${i}.quantity`)}
              error={errors[`ingredients.${i}.quantity`]}
              inputMode="decimal"
              placeholder="Например, 0.125"
            />
            <TextField
              label={`Единица ингредиента ${i + 1}`}
              {...register(`ingredients.${i}.unit`)}
              error={errors[`ingredients.${i}.unit`]}
              placeholder="г, шт., по вкусу"
            />
            <Button
              type="button"
              variant="outlined"
              disabled={ingredients.fields.length === 1}
              onClick={() => ingredients.remove(i)}
              aria-label={`Удалить ингредиент ${i + 1}`}
            >
              Удалить
            </Button>
          </fieldset>
        ))}
        {errors.ingredients && <p role="alert">{errors.ingredients}</p>}
        <Button
          type="button"
          variant="outlined"
          disabled={ingredients.fields.length >= recipeLimits.ingredients}
          onClick={() =>
            ingredients.append({ name: '', quantity: '', unit: '' })
          }
        >
          Добавить ингредиент
        </Button>
      </fieldset>
      <fieldset disabled={locked} className={cls.group}>
        <legend>Шаги приготовления</legend>
        <p className={cls.hint}>
          Можно описать весь рецепт одним шагом. До 20000 символов на шаг и
          50000 суммарно.
        </p>
        {steps.fields.map((field, i) => (
          <fieldset key={field.id} className={cls.row}>
            <legend>Шаг {i + 1}</legend>
            <label htmlFor={`${prefix}-${field.id}`}>
              Описание шага {i + 1}
            </label>
            <textarea
              id={`${prefix}-${field.id}`}
              {...register(`steps.${i}.text`)}
              aria-invalid={!!errors[`steps.${i}.text`]}
              aria-describedby={`${prefix}-${field.id}-count`}
              required
            />
            <small id={`${prefix}-${field.id}-count`}>
              {length(texts[i]?.text ?? '')} / 20000 символов
            </small>
            {errors[`steps.${i}.text`] && (
              <p role="alert">{errors[`steps.${i}.text`]}</p>
            )}
            <div className={cls.actions}>
              <Button
                type="button"
                variant="outlined"
                disabled={i === 0}
                aria-label={`Шаг ${i + 1}: выше`}
                onClick={() => {
                  steps.move(i, i - 1);
                  setNotice(`Шаг перемещён на позицию ${i}`);
                }}
              >
                Выше
              </Button>
              <Button
                type="button"
                variant="outlined"
                disabled={i === steps.fields.length - 1}
                aria-label={`Шаг ${i + 1}: ниже`}
                onClick={() => {
                  steps.move(i, i + 1);
                  setNotice(`Шаг перемещён на позицию ${i + 2}`);
                }}
              >
                Ниже
              </Button>
              <Button
                type="button"
                variant="outlined"
                disabled={steps.fields.length === 1}
                aria-label={`Удалить шаг ${i + 1}`}
                onClick={() => steps.remove(i)}
              >
                Удалить
              </Button>
            </div>
          </fieldset>
        ))}
        {errors.steps && <p role="alert">{errors.steps}</p>}
        <p>Всего: {total} / 50000 символов</p>
        <Button
          type="button"
          variant="outlined"
          disabled={steps.fields.length >= recipeLimits.steps}
          onClick={() => steps.append({ text: '' })}
        >
          Добавить шаг
        </Button>
      </fieldset>
      {notice && <p role="status">{notice}</p>}
      {submission.error && <p role="alert">{submission.error}</p>}
      {submission.retry && (
        <p>
          Результат запроса неизвестен. Повтор отправит прежние данные;
          редактирование пока заблокировано.
        </p>
      )}
      {conflict && (
        <>
          <p>
            Прежний запрос не может быть повторён. Начните новое сохранение,
            чтобы отправить рецепт отдельным действием.
          </p>
          <Button
            type="button"
            onClick={() => {
              attempt.current = null;
              setConflict(false);
              submission.reset();
            }}
          >
            Начать новое сохранение
          </Button>
        </>
      )}
      <div className={cls.actions}>
        <Button type="submit" disabled={submission.pending || conflict}>
          {submission.pending
            ? 'Сохранение…'
            : submission.retry
              ? 'Повторить сохранение'
              : 'Сохранить рецепт'}
        </Button>
        <Button
          type="button"
          variant="outlined"
          disabled={submission.pending}
          onClick={onCancel}
        >
          Отмена
        </Button>
      </div>
    </form>
  );
}
