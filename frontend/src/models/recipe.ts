export type RecipeCategory = { code: string; name: string };

export type RecipeSummary = {
  id: string;
  title: string;
  cookingTimeMinutes: number;
  categoryCode: string;
  createdAt: string;
  updatedAt: string;
};

export type RecipeIngredient = {
  position: number;
  name: string;
  quantity: string | null;
  unit: string;
};

export type RecipeStep = { position: number; text: string };

export type Recipe = RecipeSummary & {
  ingredients: RecipeIngredient[];
  steps: RecipeStep[];
};

export type SaveRecipeRequest = {
  title: string;
  cookingTimeMinutes: number;
  categoryCode: string;
  ingredients: Omit<RecipeIngredient, 'position'>[];
  steps: Omit<RecipeStep, 'position'>[];
};

export type RecipeDraft = Omit<
  SaveRecipeRequest,
  'cookingTimeMinutes' | 'ingredients'
> & {
  cookingTimeMinutes: string;
  ingredients: { name: string; quantity: string; unit: string }[];
};
