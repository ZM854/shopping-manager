import type {
  Recipe,
  RecipeCategory,
  RecipeDraft,
  SaveRecipeRequest,
} from '../models/recipe';
import { trimAuth } from './authValidation';
import { InputError, normalizeQuantity } from './shoppingValidation';

export const recipeLimits = {
  title: 200,
  ingredientName: 200,
  unit: 32,
  ingredients: 200,
  steps: 100,
  stepText: 20000,
  totalText: 50000,
  bodyBytes: 1048576,
};

function invalid(field: string, message: string): never {
  throw new InputError(message, { [field]: message });
}

function normalizeText(
  value: string,
  field: string,
  max: number,
  required = true,
) {
  if (/\p{Cc}/u.test(value)) invalid(field, 'Управляющие символы запрещены');
  const text = trimAuth(value);
  if ((required && !text) || [...text].length > max)
    invalid(field, `Введите от ${required ? 1 : 0} до ${max} символов`);
  return text;
}

export function normalizeStepText(value: string) {
  return trimAuth(value.replace(/\r\n?/g, '\n'));
}

export function recipeToDraft(recipe?: Recipe): RecipeDraft {
  if (!recipe)
    return {
      title: '',
      cookingTimeMinutes: '30',
      categoryCode: '',
      ingredients: [{ name: '', quantity: '', unit: '' }],
      steps: [{ text: '' }],
    };
  return {
    title: recipe.title,
    cookingTimeMinutes: String(recipe.cookingTimeMinutes),
    categoryCode: recipe.categoryCode,
    ingredients: recipe.ingredients.map(({ name, quantity, unit }) => ({
      name,
      quantity: quantity ?? '',
      unit,
    })),
    steps: recipe.steps.map(({ text }) => ({ text })),
  };
}

export function normalizeRecipe(
  draft: RecipeDraft,
  categories: RecipeCategory[],
): SaveRecipeRequest {
  const title = normalizeText(draft.title, 'title', recipeLimits.title);
  const minutes = Number(draft.cookingTimeMinutes);
  if (
    !/^[1-9][0-9]*$/.test(draft.cookingTimeMinutes) ||
    !Number.isInteger(minutes) ||
    minutes < 5 ||
    minutes > 1440 ||
    minutes % 5 !== 0
  )
    invalid(
      'cookingTimeMinutes',
      'Выберите время от 5 до 1440 минут с шагом 5',
    );
  if (!categories.some(({ code }) => code === draft.categoryCode))
    invalid('categoryCode', 'Выберите категорию блюда');
  if (
    !draft.ingredients.length ||
    draft.ingredients.length > recipeLimits.ingredients
  )
    invalid('ingredients', 'Добавьте от 1 до 200 ингредиентов');
  if (!draft.steps.length || draft.steps.length > recipeLimits.steps)
    invalid('steps', 'Добавьте от 1 до 100 шагов');
  const ingredients = draft.ingredients.map((ingredient, index) => {
    const prefix = `ingredients.${index}.`;
    const name = normalizeText(
      ingredient.name,
      `${prefix}name`,
      recipeLimits.ingredientName,
    );
    let quantity: string | null;
    try {
      quantity = normalizeQuantity(ingredient.quantity);
    } catch {
      invalid(
        `${prefix}quantity`,
        'Количество: 0.001…999999999.999, до трёх знаков после точки',
      );
    }
    const unit = normalizeText(
      ingredient.unit,
      `${prefix}unit`,
      recipeLimits.unit,
      false,
    );
    return { name, quantity, unit };
  });
  let total = 0;
  const steps = draft.steps.map((step, index) => {
    const text = normalizeStepText(step.text);
    const field = `steps.${index}.text`;
    for (const char of step.text.replace(/\r\n?/g, '\n')) {
      if (/\p{Cc}/u.test(char) && char !== '\n' && char !== '\t')
        invalid(
          field,
          'В описании допустимы переносы строк и табуляция, прочие управляющие символы запрещены',
        );
    }
    const length = [...text].length;
    if (!length || length > recipeLimits.stepText)
      invalid(field, 'Описание шага: от 1 до 20000 символов');
    total += length;
    if (total > recipeLimits.totalText)
      invalid('steps', 'Все шаги вместе: не более 50000 символов');
    return { text };
  });
  const result = {
    title,
    cookingTimeMinutes: minutes,
    categoryCode: draft.categoryCode,
    ingredients,
    steps,
  };
  if (
    new TextEncoder().encode(JSON.stringify(result)).length >
    recipeLimits.bodyBytes
  )
    invalid('steps', 'Рецепт превышает допустимый размер запроса 1 МиБ');
  return result;
}
