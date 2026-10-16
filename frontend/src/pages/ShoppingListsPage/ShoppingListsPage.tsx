import { useCallback, useState } from 'react';
import { Link } from 'react-router-dom';
import type { ShoppingList } from '../../models/shoppingList';
import { shoppingService } from '../../services/shoppingService';
import { useShoppingResource } from '../../hooks/useShoppingResource';
import { useScaffold } from '../../hooks/useScaffold';
import ListForm from '../../components/ListForm/ListForm';
import AlertDialog from '../../components/AlertDialog/AlertDialog';
import Modal from '../../components/UI/modal/Modal/Modal';
import Button from '../../components/UI/button/Button/Button';
import cls from './ShoppingListsPage.module.css';

export default function ShoppingListsPage() {
  const load = useCallback(
    (signal: AbortSignal) => shoppingService.getLists(signal),
    [],
  );
  const resource = useShoppingResource(load);
  const [form, setForm] = useState<ShoppingList | 'create' | null>(null);
  const [deleting, setDeleting] = useState<ShoppingList | null>(null);
  useScaffold({ topBar: { title: 'Списки покупок' }, fab: null });

  async function save(name: string, key: string) {
    if (!form) return;
    await resource.mutate(
      () =>
        form === 'create'
          ? shoppingService.createList({ name }, key)
          : shoppingService.updateList(form.id, { name }),
      (lists, result) =>
        form === 'create'
          ? lists.some((list) => list.id === result.id)
            ? lists
            : [...lists, result]
          : lists.map((list) => (list.id === result.id ? result : list)),
    );
    setForm(null);
  }

  async function remove() {
    if (!deleting) return;
    try {
      await resource.mutate(
        () => shoppingService.deleteList(deleting.id),
        (lists) => lists.filter((list) => list.id !== deleting.id),
      );
      setDeleting(null);
    } catch {
      // Ошибка уже показана resource; диалог остаётся открытым для повтора.
    }
  }

  return (
    <section className={cls.page}>
      {resource.loading && <p role="status">Загрузка списков…</p>}
      {resource.error && <p role="alert">{resource.error}</p>}
      {!resource.loading && resource.data === null && (
        <Button onClick={() => void resource.reload()}>
          Повторить загрузку
        </Button>
      )}
      {resource.data !== null && (
        <>
          <Button disabled={resource.pending} onClick={() => setForm('create')}>
            Создать список
          </Button>
          {resource.data.length === 0 && <p>У вас пока нет списков покупок.</p>}
          <ul className={cls.lists}>
            {resource.data.map((list) => (
              <li key={list.id}>
                <Link to={`/lists/${list.id}`}>{list.name}</Link>
                <Button
                  disabled={resource.pending}
                  onClick={() => setForm(list)}
                  aria-label={`Переименовать ${list.name}`}
                >
                  Переименовать
                </Button>
                <Button
                  disabled={resource.pending}
                  tone="danger"
                  onClick={() => setDeleting(list)}
                  aria-label={`Удалить список ${list.name}`}
                >
                  Удалить
                </Button>
              </li>
            ))}
          </ul>
        </>
      )}
      <Modal
        isOpen={form !== null}
        onClose={() => {
          if (!resource.pending) setForm(null);
        }}
        ariaLabel={
          form === 'create' ? 'Создать список' : 'Переименовать список'
        }
      >
        <ListForm
          initialName={form && form !== 'create' ? form.name : ''}
          onSave={save}
        />
      </Modal>
      <Modal
        isOpen={deleting !== null}
        onClose={() => {
          if (!resource.pending) setDeleting(null);
        }}
        ariaLabel="Удалить список"
      >
        <AlertDialog
          title={`Удалить список «${deleting?.name}»?`}
          message="Все его позиции будут удалены. Другие списки сохранятся."
          onDiscard={() => setDeleting(null)}
          onConfirm={() => void remove()}
          pending={resource.pending}
          error={resource.error}
          isDanger
          confirmText="Удалить"
        />
      </Modal>
    </section>
  );
}
