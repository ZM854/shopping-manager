import { useCallback, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { useResource } from '../../hooks/useResource';
import { useSessionActionGuard } from '../../hooks/useSessionActionGuard';
import { useScaffold } from '../../hooks/useScaffold';
import { recipeService } from '../../services/recipeService';
import { ApiError } from '../../services/api';
import Button from '../../components/UI/button/Button/Button';
import Modal from '../../components/UI/modal/Modal/Modal';
import AlertDialog from '../../components/AlertDialog/AlertDialog';
import cls from '../RecipesPage/RecipesPage.module.css';

function RecipeContent({ id }: { id: string }) {
  const navigate = useNavigate();
  const guard = useSessionActionGuard();
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [recipe, categories] = await Promise.all([
        recipeService.getRecipe(id, signal),
        recipeService.getCategories(signal),
      ]);
      return { recipe, categories };
    },
    [id],
  );
  const resource = useResource(load);
  const [deleting, setDeleting] = useState(false);
  const uncertain = useRef(false);
  const recipe = resource.data?.recipe;
  useScaffold({ topBar: { title: recipe?.title ?? 'Рецепт' }, fab: null });
  const close = useCallback(() => {
    if (!resource.pending) setDeleting(false);
  }, [resource.pending]);

  async function remove() {
    try {
      const check = guard();
      await resource.mutate(
        async () => {
          try {
            if (uncertain.current) {
              try {
                await recipeService.getRecipe(id);
              } catch (error) {
                if (error instanceof ApiError && error.status === 404) return;
                throw error;
              }
            }
            await recipeService.deleteRecipe(id);
          } catch (error) {
            if (
              !(error instanceof ApiError) ||
              error.status >= 500 ||
              error.code === 'origin_forbidden'
            )
              uncertain.current = true;
            throw error;
          }
        },
        (current) => current,
      );
      check();
      navigate('/recipes', { replace: true });
    } catch {
      // resource показывает отказ; закрытая страница и другая сессия результат не применяют.
    }
  }

  return (
    <section className={cls.page}>
      <Link to="/recipes">Все рецепты</Link>
      {resource.loading && <p role="status">Загрузка рецепта…</p>}
      {resource.error && !deleting && <p role="alert">{resource.error}</p>}
      {!resource.loading && !resource.data && (
        <Button onClick={() => void resource.reload()}>
          Повторить загрузку
        </Button>
      )}
      {recipe && (
        <>
          <p>
            {recipe.cookingTimeMinutes} мин ·{' '}
            {
              resource.data?.categories.find(
                ({ code }) => code === recipe.categoryCode,
              )?.name
            }
          </p>
          <div className={cls.actions}>
            <Link to={`/recipes/${id}/edit`}>Редактировать рецепт</Link>
            <Button
              tone="danger"
              disabled={resource.pending}
              onClick={() => setDeleting(true)}
            >
              Удалить рецепт
            </Button>
          </div>
          <h2>Ингредиенты</h2>
          <ul className={cls.ingredients}>
            {recipe.ingredients.map((ingredient) => (
              <li key={ingredient.position}>
                {[ingredient.name, ingredient.quantity, ingredient.unit]
                  .filter((value) => value !== null && value !== '')
                  .join(' — ')}
              </li>
            ))}
          </ul>
          <h2>Приготовление</h2>
          <ol className={cls.steps}>
            {recipe.steps.map((step) => (
              <li key={step.position} value={step.position}>
                <p className={cls.stepText}>{step.text}</p>
              </li>
            ))}
          </ol>
        </>
      )}
      <Modal isOpen={deleting} onClose={close} ariaLabel="Удалить рецепт">
        <AlertDialog
          title={`Удалить рецепт «${recipe?.title}»?`}
          message="Ингредиенты и шаги также будут удалены."
          onDiscard={close}
          onConfirm={() => void remove()}
          confirmText="Удалить"
          isDanger
          pending={resource.pending}
          error={resource.error}
        />
      </Modal>
    </section>
  );
}

export default function RecipePage() {
  const { recipeId = '' } = useParams();
  return <RecipeContent key={recipeId} id={recipeId} />;
}
