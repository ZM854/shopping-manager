import { useCallback } from 'react';
import { Link } from 'react-router-dom';
import { useResource } from '../../hooks/useResource';
import { useScaffold } from '../../hooks/useScaffold';
import { recipeService } from '../../services/recipeService';
import Button from '../../components/UI/button/Button/Button';
import cls from './RecipesPage.module.css';

export default function RecipesPage() {
  const load = useCallback(async (signal: AbortSignal) => {
    const [recipes, categories] = await Promise.all([
      recipeService.getRecipes(signal),
      recipeService.getCategories(signal),
    ]);
    return { recipes, categories };
  }, []);
  const resource = useResource(load);
  useScaffold({ topBar: { title: 'Мои рецепты' }, fab: null });
  return (
    <section className={cls.page}>
      <Link to="/recipes/new">Создать рецепт</Link>
      {resource.loading && <p role="status">Загрузка рецептов…</p>}
      {resource.error && <p role="alert">{resource.error}</p>}
      {!resource.loading && resource.data === null && (
        <Button onClick={() => void resource.reload()}>
          Повторить загрузку
        </Button>
      )}
      {resource.data && (
        <>
          {resource.data.recipes.length === 0 && (
            <p>У вас пока нет рецептов.</p>
          )}
          <ul className={cls.list}>
            {resource.data.recipes.map((recipe) => (
              <li key={recipe.id}>
                <Link to={`/recipes/${recipe.id}`}>{recipe.title}</Link>
                <p>
                  {recipe.cookingTimeMinutes} мин ·{' '}
                  {
                    resource.data?.categories.find(
                      ({ code }) => code === recipe.categoryCode,
                    )?.name
                  }
                </p>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
