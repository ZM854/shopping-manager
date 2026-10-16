import type {
  ShoppingList,
  ShoppingItem,
  CreateShoppingListRequest,
  UpdateShoppingListRequest,
  CreateShoppingItemRequest,
  UpdateShoppingItemRequest,
} from '../models/shoppingList';
import { apiFetch } from './api';

const listPath = (listId: string) =>
  `/shopping-lists/${encodeURIComponent(listId)}`;

const itemPath = (listId: string, itemId: string) =>
  `${listPath(listId)}/items/${encodeURIComponent(itemId)}`;

export const shoppingService = {
  getLists: (signal?: AbortSignal) =>
    apiFetch<ShoppingList[]>('/shopping-lists', { signal }),

  getList: (listId: string, signal?: AbortSignal) =>
    apiFetch<ShoppingList>(listPath(listId), { signal }),

  createList: (body: CreateShoppingListRequest, key: string) =>
    apiFetch<ShoppingList>('/shopping-lists', {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body: JSON.stringify(body),
    }),

  updateList: (listId: string, body: UpdateShoppingListRequest) =>
    apiFetch<ShoppingList>(listPath(listId), {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),

  deleteList: (listId: string) =>
    apiFetch<void>(listPath(listId), { method: 'DELETE' }),

  getItems: (listId: string, signal?: AbortSignal) =>
    apiFetch<ShoppingItem[]>(`${listPath(listId)}/items`, { signal }),

  getItem: (listId: string, itemId: string, signal?: AbortSignal) =>
    apiFetch<ShoppingItem>(itemPath(listId, itemId), { signal }),

  createItem: (listId: string, body: CreateShoppingItemRequest, key: string) =>
    apiFetch<ShoppingItem>(`${listPath(listId)}/items`, {
      method: 'POST',
      headers: { 'Idempotency-Key': key },
      body: JSON.stringify(body),
    }),

  updateItem: (
    listId: string,
    itemId: string,
    body: UpdateShoppingItemRequest,
  ) =>
    apiFetch<ShoppingItem>(itemPath(listId, itemId), {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),

  deleteItem: (listId: string, itemId: string) =>
    apiFetch<void>(itemPath(listId, itemId), { method: 'DELETE' }),

  clearItems: (listId: string) =>
    apiFetch<void>(`${listPath(listId)}/items`, { method: 'DELETE' }),

  markAll: (listId: string, isMarked: boolean) =>
    apiFetch<void>(`${listPath(listId)}/items/mark-all`, {
      method: 'PATCH',
      body: JSON.stringify({ isMarked }),
    }),
};
