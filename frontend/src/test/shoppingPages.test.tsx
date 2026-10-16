import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter, Navigate, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Layout from '../layout/Layout';
import ShoppingListsPage from '../pages/ShoppingListsPage/ShoppingListsPage';
import ShoppingListPage from '../pages/ShoppingListPage/ShoppingListPage';
import { shoppingService } from '../services/shoppingService';
import type { ShoppingItem, ShoppingList } from '../models/shoppingList';
import { ApiError } from '../services/api';

vi.mock('../services/shoppingService', () => ({
  shoppingService: {
    getLists: vi.fn(),
    getList: vi.fn(),
    getItems: vi.fn(),
    createList: vi.fn(),
    updateList: vi.fn(),
    deleteList: vi.fn(),
    createItem: vi.fn(),
    updateItem: vi.fn(),
    deleteItem: vi.fn(),
    clearItems: vi.fn(),
    markAll: vi.fn(),
  },
}));
const api = vi.mocked(shoppingService);
const firstId = '9007199254740993';
const secondId = '9007199254740994';
let lists: ShoppingList[];
let items: Record<string, ShoppingItem[]>;

function metadata(id: string, name: string): ShoppingList {
  return {
    id,
    name,
    createdAt: '2026-10-13T12:00:00Z',
    updatedAt: '2026-10-13T12:00:00Z',
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  lists = [
    metadata(firstId, 'Первый список'),
    metadata(secondId, 'Второй список'),
  ];
  items = {
    [firstId]: [
      {
        id: '9223372036854775807',
        listId: firstId,
        name: 'Молоко',
        quantity: '0.125',
        unit: 'л',
        isMarked: false,
      },
    ],
    [secondId]: [
      {
        id: '3',
        listId: secondId,
        name: 'Хлеб',
        quantity: null,
        unit: '',
        isMarked: true,
      },
    ],
  };
  api.getLists.mockImplementation(async () => [...lists]);
  api.getList.mockImplementation(async (id) => ({
    ...lists.find((list) => list.id === id)!,
  }));
  api.getItems.mockImplementation(async (id) => [...items[id]]);
  api.createList.mockImplementation(async ({ name }) => {
    const list = metadata(`10${lists.length}`, name);
    lists.push(list);
    items[list.id] = [];
    return list;
  });
  api.updateList.mockImplementation(async (id, { name }) => {
    const list = { ...lists.find((list) => list.id === id)!, name };
    lists = lists.map((value) => (value.id === id ? list : value));
    return list;
  });
  api.deleteList.mockImplementation(async (id) => {
    lists = lists.filter((list) => list.id !== id);
  });
  api.createItem.mockImplementation(async (listId, data) => {
    const item = {
      id: `10${items[listId].length}`,
      listId,
      name: data.name,
      quantity: data.quantity ?? null,
      unit: data.unit ?? '',
      isMarked: data.isMarked ?? false,
    };
    items[listId].push(item);
    return item;
  });
  api.updateItem.mockImplementation(async (listId, id, data) => {
    const item = {
      ...items[listId].find((value) => value.id === id)!,
      ...data,
    };
    items[listId] = items[listId].map((value) =>
      value.id === id ? item : value,
    );
    return item;
  });
  api.deleteItem.mockImplementation(async (listId, id) => {
    items[listId] = items[listId].filter((item) => item.id !== id);
  });
  api.clearItems.mockImplementation(async (listId) => {
    items[listId] = [];
  });
  api.markAll.mockImplementation(async (listId, isMarked) => {
    items[listId] = items[listId].map((item) => ({ ...item, isMarked }));
  });
});

function show(path = '/lists') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Link to={`/lists/${secondId}`}>Перейти во второй</Link>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Navigate to="/lists" replace />} />
          <Route path="/lists" element={<ShoppingListsPage />} />
          <Route path="/lists/:listId" element={<ShoppingListPage />} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}
async function saveDialog(
  user: ReturnType<typeof userEvent.setup>,
  label: string,
  value: string,
) {
  const dialog = screen.getByRole('dialog');
  const input = within(dialog).getByLabelText(label);
  await user.clear(input);
  await user.type(input, value);
  await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
  );
}

describe('Страницы нескольких списков', () => {
  it('повтор создания уже загруженной позиции не дублирует её в UI', async () => {
    api.createItem.mockResolvedValueOnce(items[firstId][0]);
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    await screen.findByRole('heading', { name: 'Молоко' });
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    await saveDialog(user, 'Название', 'Молоко');
    expect(screen.getAllByRole('heading', { name: 'Молоко' })).toHaveLength(1);
  });
  it('ошибки загрузки и удаления списка видны; повтор не затрагивает другие списки', async () => {
    api.getLists.mockRejectedValueOnce(new Error('Ошибка загрузки'));
    const user = userEvent.setup({ delay: null });
    show();
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Ошибка загрузки',
    );
    await user.click(
      screen.getByRole('button', { name: 'Повторить загрузку' }),
    );
    await screen.findByRole('link', { name: 'Первый список' });
    api.deleteList.mockRejectedValueOnce(new ApiError(500, 'Ошибка удаления'));
    await user.click(
      screen.getByRole('button', { name: 'Удалить список Первый список' }),
    );
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    expect(
      await within(screen.getByRole('dialog')).findByRole('alert'),
    ).toHaveTextContent('Ошибка удаления');
    expect(screen.getByRole('link', { name: 'Первый список' })).toBeVisible();
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('link', { name: 'Второй список' })).toBeVisible();
  });

  it('ошибки редактирования, удаления позиции и массовых отметок сохраняют данные', async () => {
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    await screen.findByRole('heading', { name: 'Молоко' });
    api.updateItem.mockRejectedValueOnce(
      new ApiError(400, 'Ошибка поля', 'validation_failed', {
        name: 'Исправьте имя',
      }),
    );
    await user.click(
      screen.getByRole('button', { name: 'Редактировать Молоко' }),
    );
    const dialog = screen.getByRole('dialog');
    await user.clear(within(dialog).getByLabelText('Название'));
    await user.type(within(dialog).getByLabelText('Название'), 'Кефир');
    await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    expect(await within(dialog).findByText('Исправьте имя')).toBeVisible();
    expect(within(dialog).getByLabelText('Название')).not.toBeDisabled();
    expect(screen.getByRole('heading', { name: 'Молоко' })).toBeVisible();
    await user.keyboard('{Escape}');
    api.deleteItem.mockRejectedValueOnce(
      new ApiError(500, 'Удаление не выполнено'),
    );
    await user.click(screen.getByRole('button', { name: 'Удалить Молоко' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Удаление не выполнено',
    );
    api.markAll.mockRejectedValueOnce(
      new ApiError(500, 'Отметки не сохранены'),
    );
    await user.click(screen.getByRole('button', { name: 'Вычеркнуть всё' }));
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Отметки не сохранены',
      ),
    );
    expect(
      screen.getByRole('checkbox', { name: 'Куплено: Молоко' }),
    ).not.toBeChecked();
  });
  it('корень открывает списки; создаёт два, переименовывает и удаляет с подтверждением', async () => {
    lists = [];
    const user = userEvent.setup({ delay: null });
    show('/');
    expect(
      await screen.findByText('У вас пока нет списков покупок.'),
    ).toBeVisible();
    for (const name of ['Дом', 'Поездка']) {
      await user.click(screen.getByRole('button', { name: 'Создать список' }));
      await saveDialog(user, 'Название списка', name);
      expect(screen.getByRole('link', { name })).toBeVisible();
    }
    const keys = api.createList.mock.calls.map(([, key]) => key);
    expect(keys[0]).not.toBe(keys[1]);
    await user.click(screen.getByRole('button', { name: 'Переименовать Дом' }));
    await saveDialog(user, 'Название списка', 'Неделя');
    await user.click(
      screen.getByRole('button', { name: 'Удалить список Неделя' }),
    );
    await user.click(screen.getByRole('button', { name: 'Отменить' }));
    expect(api.deleteList).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole('button', { name: 'Удалить список Неделя' }),
    );
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole('link', { name: 'Неделя' }),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('link', { name: 'Поездка' })).toBeVisible();
  });

  it('CRUD и массовые действия ограничены выбранным списком; PATCH не отправляет неизменённые поля', async () => {
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    expect(
      await screen.findByRole('heading', { name: 'Молоко' }),
    ).toBeVisible();
    await user.click(screen.getByRole('checkbox', { name: 'Куплено: Молоко' }));
    expect(api.updateItem).toHaveBeenLastCalledWith(
      firstId,
      '9223372036854775807',
      { isMarked: true },
    );
    await user.click(
      screen.getByRole('button', { name: 'Редактировать Молоко' }),
    );
    await saveDialog(user, 'Название', 'Кефир');
    expect(api.updateItem).toHaveBeenLastCalledWith(
      firstId,
      '9223372036854775807',
      { name: 'Кефир' },
    );
    await user.click(
      screen.getByRole('button', { name: 'Редактировать Кефир' }),
    );
    const quantity = within(screen.getByRole('dialog')).getByLabelText(
      'Количество',
    );
    await user.clear(quantity);
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Сохранить',
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
    expect(api.updateItem).toHaveBeenLastCalledWith(
      firstId,
      '9223372036854775807',
      { quantity: null },
    );
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    await user.type(
      within(screen.getByRole('dialog')).getByLabelText('Количество'),
      '999999999.999',
    );
    await saveDialog(user, 'Название', 'Яйца');
    expect(api.createItem).toHaveBeenCalledWith(
      firstId,
      expect.objectContaining({ quantity: '999999999.999' }),
      expect.any(String),
    );
    await user.click(screen.getByRole('button', { name: 'Вычеркнуть всё' }));
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Куплено: Яйца' }),
      ).toBeChecked(),
    );
    await user.click(screen.getByRole('button', { name: 'Снять все отметки' }));
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Куплено: Яйца' }),
      ).not.toBeChecked(),
    );
    await user.click(screen.getByRole('button', { name: 'Удалить Яйца' }));
    await waitFor(() =>
      expect(
        screen.queryByRole('heading', { name: 'Яйца' }),
      ).not.toBeInTheDocument(),
    );
    await user.click(screen.getByRole('button', { name: 'Очистить список' }));
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    expect(await screen.findByText('Список покупок пока пуст.')).toBeVisible();
    expect(items[secondId][0].isMarked).toBe(true);
    await user.click(screen.getByRole('link', { name: 'Перейти во второй' }));
    expect(await screen.findByRole('heading', { name: 'Хлеб' })).toBeVisible();
    expect(api.getItems).toHaveBeenLastCalledWith(
      secondId,
      expect.any(AbortSignal),
    );
  });

  it('ошибки чтения позволяют повтор, неправильный ID не отправляется', async () => {
    api.getItems.mockRejectedValueOnce(new Error('offline'));
    const user = userEvent.setup({ delay: null });
    const view = show(`/lists/${firstId}`);
    expect(await screen.findByRole('alert')).toHaveTextContent('offline');
    await user.click(
      screen.getByRole('button', { name: 'Повторить загрузку' }),
    );
    expect(
      await screen.findByRole('heading', { name: 'Молоко' }),
    ).toBeVisible();
    view.unmount();
    api.getItems.mockClear();
    show('/lists/9223372036854775808');
    expect(screen.getByRole('alert')).toHaveTextContent('Некорректный ID');
    expect(api.getItems).not.toHaveBeenCalled();
  });

  it('сбой создания сохраняет форму, тело и ключ; двойная отправка блокируется', async () => {
    api.createItem.mockRejectedValueOnce(new Error('Потеряна связь'));
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    await screen.findByRole('heading', { name: 'Молоко' });
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    const dialog = screen.getByRole('dialog');
    await user.type(within(dialog).getByLabelText('Название'), 'Сыр');
    await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      'Потеряна связь',
    );
    expect(within(dialog).getByLabelText('Название')).toBeDisabled();
    const pending = deferred<ShoppingItem>();
    api.createItem.mockReturnValueOnce(pending.promise);
    await user.click(within(dialog).getByRole('button', { name: 'Повторить' }));
    expect(
      within(dialog).getByRole('button', { name: 'Сохранение…' }),
    ).toBeDisabled();
    expect(api.createItem.mock.calls[0]).toEqual(api.createItem.mock.calls[1]);
    await act(async () =>
      pending.resolve({ ...items[firstId][0], id: '4', name: 'Сыр' }),
    );
    expect(await screen.findByRole('heading', { name: 'Сыр' })).toBeVisible();
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument(),
    );
  });

  it('очистка ждёт ответ; ошибка оставляет данные и диалог для повтора', async () => {
    const pending = deferred<void>();
    api.clearItems.mockReturnValueOnce(pending.promise);
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    await screen.findByRole('heading', { name: 'Молоко' });
    await user.click(screen.getByRole('button', { name: 'Очистить список' }));
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'Молоко' })).toBeVisible();
    await act(async () => pending.reject(new ApiError(500, 'Ошибка очистки')));
    expect(
      within(screen.getByRole('dialog')).getByRole('alert'),
    ).toHaveTextContent('Ошибка очистки');
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Удалить',
      }),
    );
    expect(await screen.findByText('Список покупок пока пуст.')).toBeVisible();
  });

  it('позднее чтение первого списка не попадает во второй', async () => {
    const late = deferred<ShoppingItem[]>();
    api.getItems.mockReturnValueOnce(late.promise);
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    expect(screen.getByRole('status')).toHaveTextContent('Загрузка');
    await user.click(screen.getByRole('link', { name: 'Перейти во второй' }));
    expect(await screen.findByRole('heading', { name: 'Хлеб' })).toBeVisible();
    expect(api.getItems.mock.calls[0][1]?.aborted).toBe(true);
    await act(async () => late.resolve(items[firstId]));
    expect(
      screen.queryByRole('heading', { name: 'Молоко' }),
    ).not.toBeInTheDocument();
  });

  it('поздняя мутация не изменяет второй список и не закрывает его новую форму', async () => {
    const late = deferred<ShoppingItem>();
    api.createItem.mockReturnValueOnce(late.promise);
    const user = userEvent.setup({ delay: null });
    show(`/lists/${firstId}`);
    await screen.findByRole('heading', { name: 'Молоко' });
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    await user.type(
      within(screen.getByRole('dialog')).getByLabelText('Название'),
      'Сыр',
    );
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Сохранить',
      }),
    );
    await user.click(screen.getByRole('link', { name: 'Перейти во второй' }));
    await screen.findByRole('heading', { name: 'Хлеб' });
    await user.click(screen.getByRole('button', { name: 'Добавить товар' }));
    await act(async () => late.resolve({ ...items[firstId][0], name: 'Сыр' }));
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(
      screen.queryByRole('heading', { name: 'Сыр' }),
    ).not.toBeInTheDocument();
  });
});
