// DTO API списков покупок; ID и количество передаются строками.
export type ShoppingList = {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
};

export type ShoppingItem = {
  id: string;
  listId: string;
  name: string;
  quantity: string | null;
  unit: string;
  isMarked: boolean;
};

export type CreateShoppingListRequest = { name: string };
export type UpdateShoppingListRequest = { name: string };

export type CreateShoppingItemRequest = {
  name: string;
  quantity?: string | null;
  unit?: string;
  isMarked?: boolean;
};

export type UpdateShoppingItemRequest = {
  name?: string;
  quantity?: string | null;
  unit?: string;
  isMarked?: boolean;
};

export type MarkAllShoppingItemsRequest = { isMarked: boolean };

export type ShoppingError = {
  code: string;
  message: string;
  fieldErrors?: Record<string, string>;
};

export type ShoppingListShare = {
  id: string;
  createdAt: string;
  preserveMarks: boolean;
};

export type CreateShoppingListShareRequest = {
  token: string;
  preserveMarks: boolean;
};

export type ImportShoppingListShareRequest = { token: string };
