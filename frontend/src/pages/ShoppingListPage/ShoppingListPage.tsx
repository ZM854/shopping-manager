import { useCallback, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import ProductForm from '../../components/ProductForm/ProductForm';
import ProductList from '../../components/ProductList/ProductList';
import Modal from '../../components/UI/modal/Modal/Modal';
import Button from '../../components/UI/button/Button/Button';
import AlertDialog from '../../components/AlertDialog/AlertDialog';
import ShareListDialog from '../../components/ShareListDialog/ShareListDialog';
import { useShoppingResource } from '../../hooks/useShoppingResource';
import { useScaffold } from '../../hooks/useScaffold';
import { shoppingService } from '../../services/shoppingService';
import type {
  ShoppingItem,
  CreateShoppingItemRequest,
  UpdateShoppingItemRequest,
} from '../../models/shoppingList';
import cls from './ShoppingListPage.module.css';

function ShoppingListPage({ listId }: { listId: string }) {
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [list, items] = await Promise.all([
        shoppingService.getList(listId, signal),
        shoppingService.getItems(listId, signal),
      ]);
      return { list, items };
    },
    [listId],
  );
  const resource = useShoppingResource(load);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<ShoppingItem | null>(null);
  const [clearing, setClearing] = useState(false);
  const [sharing, setSharing] = useState(false);
  useScaffold({
    topBar: { title: resource.data?.list.name ?? 'Список покупок' },
    fab: null,
  });

  async function save(data: CreateShoppingItemRequest, key: string) {
    if (editing) {
      const patch: UpdateShoppingItemRequest = {};
      if (data.name !== editing.name) patch.name = data.name;
      if (data.quantity !== editing.quantity) patch.quantity = data.quantity;
      if (data.unit !== editing.unit) patch.unit = data.unit;
      if (data.isMarked !== editing.isMarked) patch.isMarked = data.isMarked;
      if (Object.keys(patch).length > 0) {
        await resource.mutate(
          () => shoppingService.updateItem(listId, editing.id, patch),
          (current, item) => ({
            ...current,
            items: current.items.map((value) =>
              value.id === item.id ? item : value,
            ),
          }),
        );
      }
    } else {
      await resource.mutate(
        () => shoppingService.createItem(listId, data, key),
        (current, item) => ({
          ...current,
          items: current.items.some((value) => value.id === item.id)
            ? current.items
            : [...current.items, item],
        }),
      );
    }
    setFormOpen(false);
  }

  async function updateItem(id: string, patch: UpdateShoppingItemRequest) {
    try {
      await resource.mutate(
        () => shoppingService.updateItem(listId, id, patch),
        (current, item) => ({
          ...current,
          items: current.items.map((value) =>
            value.id === item.id ? item : value,
          ),
        }),
      );
    } catch {
      /* resource показывает ошибку, состояние позиции сохраняется. */
    }
  }

  async function removeItem(id: string) {
    try {
      await resource.mutate(
        () => shoppingService.deleteItem(listId, id),
        (current) => ({
          ...current,
          items: current.items.filter((item) => item.id !== id),
        }),
      );
    } catch {
      /* resource показывает ошибку, состояние позиции сохраняется. */
    }
  }

  async function markAll(isMarked: boolean) {
    try {
      await resource.mutate(
        () => shoppingService.markAll(listId, isMarked),
        (current) => ({
          ...current,
          items: current.items.map((item) => ({ ...item, isMarked })),
        }),
      );
    } catch {
      /* resource показывает ошибку, отметки сохраняются. */
    }
  }

  async function clear() {
    try {
      await resource.mutate(
        () => shoppingService.clearItems(listId),
        (current) => ({ ...current, items: [] }),
      );
      setClearing(false);
    } catch {
      /* Диалог остаётся открытым для повтора. */
    }
  }

  return (
    <section className={cls.page}>
      <Link to="/lists">Все списки</Link>
      {resource.loading && <p role="status">Загрузка списка…</p>}
      {resource.error && <p role="alert">{resource.error}</p>}
      {!resource.loading && (
        <Button
          disabled={resource.pending}
          onClick={() => void resource.reload()}
        >
          Повторить загрузку
        </Button>
      )}
      {resource.data && (
        <>
          <div className={cls.actions}>
            <Button
              disabled={resource.pending}
              onClick={() => setSharing(true)}
            >
              Поделиться
            </Button>
            <Button
              disabled={resource.pending}
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
            >
              Добавить товар
            </Button>
            <Button
              disabled={resource.pending}
              onClick={() => void markAll(true)}
            >
              Вычеркнуть всё
            </Button>
            <Button
              disabled={resource.pending}
              onClick={() => void markAll(false)}
            >
              Снять все отметки
            </Button>
            <Button
              disabled={resource.pending}
              tone="danger"
              onClick={() => setClearing(true)}
            >
              Очистить список
            </Button>
          </div>
          <ProductList
            products={resource.data.items}
            pending={resource.pending}
            editProduct={(item) => {
              setEditing(item);
              setFormOpen(true);
            }}
            updateProduct={(id, patch) => void updateItem(id, patch)}
            deleteProduct={(id) => void removeItem(id)}
          />
        </>
      )}
      <Modal
        isOpen={formOpen}
        onClose={() => {
          if (!resource.pending) setFormOpen(false);
        }}
        ariaLabel={editing ? 'Редактировать товар' : 'Добавить товар'}
      >
        <ProductForm product={editing} onSave={save} />
      </Modal>
      {sharing && (
        <ShareListDialog listId={listId} onClose={() => setSharing(false)} />
      )}
      <Modal
        isOpen={clearing}
        onClose={() => {
          if (!resource.pending) setClearing(false);
        }}
        ariaLabel="Подтверждение очистки списка"
      >
        <AlertDialog
          title="Очистить весь список?"
          message="Другие списки сохранятся."
          onDiscard={() => setClearing(false)}
          onConfirm={() => void clear()}
          pending={resource.pending}
          error={resource.error}
          confirmText="Удалить"
          isDanger
        />
      </Modal>
    </section>
  );
}

export default function ShoppingListRoute() {
  const { listId = '' } = useParams();
  const valid =
    /^[1-9][0-9]{0,18}$/.test(listId) &&
    listId.trim() === listId &&
    (listId.length < 19 || listId <= '9223372036854775807');
  if (!valid)
    return (
      <section>
        <p role="alert">Некорректный ID списка</p>
        <Link to="/lists">Все списки</Link>
      </section>
    );
  // key сбрасывает формы и локальные мутации при переходе к другому списку.
  return <ShoppingListPage key={listId} listId={listId} />;
}
