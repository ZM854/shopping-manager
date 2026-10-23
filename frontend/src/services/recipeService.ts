import type {
  Recipe,
  RecipeCategory,
  RecipeSummary,
  SaveRecipeRequest,
} from '../models/recipe';
import { apiFetch } from './api';

const recipePath = (id: string) => `/recipes/${encodeURIComponent(id)}`;

export const recipeService = {
  getCategories: (signal?: AbortSignal) =>
    apiFetch<RecipeCategory[]>('/recipe-categories', { signal }),

  getRecipes: (signal?: AbortSignal) =>
    apiFetch<RecipeSummary[]>('/recipes', { signal }),

  getRecipe: (id: string, signal?: AbortSignal) =>
    apiFetch<Recipe>(recipePath(id), { signal }),

  createRecipe: (body: SaveRecipeRequest, key: string) =>
    apiFetch<Recipe>('/recipes', {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body: JSON.stringify(body),
    }),

  updateRecipe: (id: string, body: SaveRecipeRequest) =>
    apiFetch<Recipe>(recipePath(id), {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
    
  deleteRecipe: (id: string) =>
    apiFetch<void>(recipePath(id), { method: 'DELETE' }),
};
