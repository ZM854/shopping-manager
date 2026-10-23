import { useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useResource } from '../../hooks/useResource';
import { useScaffold } from '../../hooks/useScaffold';
import { recipeService } from '../../services/recipeService';
import RecipeForm from '../../components/RecipeForm/RecipeForm';
import Button from '../../components/UI/button/Button/Button';
import cls from '../RecipesPage/RecipesPage.module.css';

function RecipeEditor({ id }: { id?: string }) {
  const navigate = useNavigate();
  const load = useCallback(
    async (signal: AbortSignal) => {
      const [categories, recipe] = await Promise.all([
        recipeService.getCategories(signal),
        id ? recipeService.getRecipe(id, signal) : Promise.resolve(undefined),
      ]);
      return { categories, recipe };
    },
    [id],
  );
  const resource = useResource(load);
  const title = id ? 'Редактирование рецепта' : 'Новый рецепт';
  useScaffold({ topBar: { title }, fab: null });
  return (
    <section className={cls.page}>
      {resource.loading && <p role="status">Загрузка редактора…</p>}
      {resource.error && <p role="alert">{resource.error}</p>}
      {!resource.loading && !resource.data && (
        <Button onClick={() => void resource.reload()}>
          Повторить загрузку
        </Button>
      )}
      {resource.data && (
        <RecipeForm
          recipe={resource.data.recipe}
          categories={resource.data.categories}
          onSave={(body, key) =>
            id
              ? recipeService.updateRecipe(id, body)
              : recipeService.createRecipe(body, key)
          }
          onSaved={(recipe) =>
            navigate(`/recipes/${recipe.id}`, { replace: true })
          }
          onCancel={() => navigate(id ? `/recipes/${id}` : '/recipes')}
        />
      )}
    </section>
  );
}

export default function RecipeEditorPage() {
  const { recipeId } = useParams();
  return <RecipeEditor key={recipeId ?? 'create'} id={recipeId} />;
}
