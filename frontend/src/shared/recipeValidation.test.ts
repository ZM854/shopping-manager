import { describe, expect, it } from 'vitest';
import { normalizeRecipe, recipeToDraft } from './recipeValidation';
import { InputError } from './shoppingValidation';
import type { RecipeDraft } from '../models/recipe';

const categories = [{ code: 'baking', name: 'Выпечка' }];
function draft(): RecipeDraft {
  return {
    title: ' Пирог ',
    cookingTimeMinutes: '30',
    categoryCode: 'baking',
    ingredients: [
      { name: ' Мука ', quantity: '2.500', unit: ' г ' },
      { name: 'Соль', quantity: '', unit: 'по вкусу' },
    ],
    steps: [{ text: ' Смешать\r\n\tИспечь ' }],
  };
}
function fieldError(value: RecipeDraft, path: string) {
  try {
    normalizeRecipe(value, categories);
    throw new Error('Приняты невалидные данные');
  } catch (error) {
    expect(error).toBeInstanceOf(InputError);
    expect((error as InputError).fieldErrors?.[path]).toBeTruthy();
  }
}

describe('Валидация рецептов', () => {
  it('нормализует независимые количество/единицу и переносы, сохраняя исходный черновик и дубликаты', () => {
    const value = draft();
    value.ingredients.push({
      name: 'Мука',
      quantity: '999999999.999',
      unit: '',
    });
    value.ingredients.push({ name: 'Вода', quantity: '', unit: '' });
    const before = JSON.stringify(value);
    const result = normalizeRecipe(value, categories);
    expect(result).toEqual({
      title: 'Пирог',
      cookingTimeMinutes: 30,
      categoryCode: 'baking',
      ingredients: [
        { name: 'Мука', quantity: '2.5', unit: 'г' },
        { name: 'Соль', quantity: null, unit: 'по вкусу' },
        { name: 'Мука', quantity: '999999999.999', unit: '' },
        { name: 'Вода', quantity: null, unit: '' },
      ],
      steps: [{ text: 'Смешать\n\tИспечь' }],
    });
    result.steps[0].text = 'Правка';
    result.ingredients[0].quantity = '1';
    expect(JSON.stringify(value)).toBe(before);
  });
  it('считает Unicode-кодовые точки и допускает большой одиночный шаг', () => {
    const value = draft();
    value.title = '🍲'.repeat(200);
    value.steps = [{ text: '🍲'.repeat(20000) }];
    expect(normalizeRecipe(value, categories).steps[0].text).toHaveLength(
      40000,
    );
    value.title += 'я';
    fieldError(value, 'title');
    value.title = 'Пирог';
    value.steps[0].text += 'я';
    fieldError(value, 'steps.0.text');
  });
  it('проверяет общий лимит 50000, число строк и отсутствие управляющих символов до trim', () => {
    const value = draft();
    value.ingredients = Array.from({ length: 200 }, () => ({
      name: 'Мука',
      quantity: '0.001',
      unit: '',
    }));
    value.steps = [
      { text: 'я'.repeat(20000) },
      { text: 'я'.repeat(20000) },
      { text: 'я'.repeat(10000) },
    ];
    expect(normalizeRecipe(value, categories).ingredients).toHaveLength(200);
    value.steps[2].text += 'я';
    fieldError(value, 'steps');
    value.steps = Array.from({ length: 100 }, () => ({ text: 'Шаг' }));
    expect(normalizeRecipe(value, categories).steps).toHaveLength(100);
    value.steps.push({ text: 'Ещё' });
    fieldError(value, 'steps');
    value.steps = [{ text: '\vСмешать' }];
    fieldError(value, 'steps.0.text');
    value.steps = [{ text: 'Смешать\rИспечь' }];
    expect(normalizeRecipe(value, categories).steps[0].text).toBe(
      'Смешать\nИспечь',
    );
    value.title = '\u0085Пирог';
    fieldError(value, 'title');
  });
  it.each(['0', '1.0001', '1000000000', '01', '-1', '1e2', ' 2', '2,5'])(
    'отвергает количество %s с путём поля',
    (quantity) => {
      const value = draft();
      value.ingredients[0].quantity = quantity;
      fieldError(value, 'ingredients.0.quantity');
    },
  );
  it.each(['0', '4', '6', '1445', '30.0', '3e1', '', '05'])(
    'отвергает время %s',
    (time) => {
      const value = draft();
      value.cookingTimeMinutes = time;
      fieldError(value, 'cookingTimeMinutes');
    },
  );
  it('проверяет обязательность полей и границы текста', () => {
    const value = draft();
    value.categoryCode = 'unknown';
    fieldError(value, 'categoryCode');
    value.categoryCode = 'baking';
    value.cookingTimeMinutes = '1440';
    expect(normalizeRecipe(value, categories).cookingTimeMinutes).toBe(1440);
    value.cookingTimeMinutes = '5';
    expect(normalizeRecipe(value, categories).cookingTimeMinutes).toBe(5);
    value.ingredients[0].name = 'я'.repeat(201);
    fieldError(value, 'ingredients.0.name');
    value.ingredients[0].name = '';
    fieldError(value, 'ingredients.0.name');
    value.ingredients[0].name = 'Мука';
    value.ingredients[0].unit = 'я'.repeat(33);
    fieldError(value, 'ingredients.0.unit');
    value.ingredients[0].unit = '\tг';
    fieldError(value, 'ingredients.0.unit');
    value.ingredients = [];
    fieldError(value, 'ingredients');
    value.ingredients = Array.from({ length: 201 }, () => ({
      name: 'Мука',
      quantity: '',
      unit: '',
    }));
    fieldError(value, 'ingredients');
    value.ingredients = draft().ingredients;
    value.steps = [];
    fieldError(value, 'steps');
    value.steps = [{ text: ' \r\n\t ' }];
    fieldError(value, 'steps.0.text');
  });
  it('создаёт пустую форму без ID и обнуляет количество при отображении null', () => {
    expect(recipeToDraft().ingredients).toEqual([
      { name: '', quantity: '', unit: '' },
    ]);
  });
});
