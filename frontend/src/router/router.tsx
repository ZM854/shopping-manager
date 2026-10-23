import { createBrowserRouter, Navigate } from 'react-router-dom';

import Layout from '../layout/Layout';

import RequireAuth from './RequireAuth';

import ShoppingListPage from '../pages/ShoppingListPage/ShoppingListPage';
import ShoppingListsPage from '../pages/ShoppingListsPage/ShoppingListsPage';
import RecipesPage from '../pages/RecipesPage/RecipesPage';
import RecipePage from '../pages/RecipePage/RecipePage';
import RecipeEditorPage from '../pages/RecipeEditorPage/RecipeEditorPage';
import ProfilePage from '../pages/ProfilePage/ProfilePage';

import LoginPage from '../pages/LoginPage/LoginPage';
import RegistrationPage from '../pages/RegistrationPage/RegistrationPage';
import ActivationPage from '../pages/ActivationPage/ActivationPage';
import SharedShoppingListPage from '../pages/SharedShoppingListPage/SharedShoppingListPage';

export const router = createBrowserRouter([
  {
    path: '/share',
    element: <SharedShoppingListPage />,
  },
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/registration',
    element: <RegistrationPage />,
  },
  {
    path: '/activation/:token?',
    element: <ActivationPage />,
  },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <Layout />,
        children: [
          {
            path: '/',
            element: <Navigate to="/lists" replace />,
          },
          {
            path: '/lists',
            element: <ShoppingListsPage />,
          },
          {
            path: '/lists/:listId',
            element: <ShoppingListPage />,
          },
          {
            path: '/profile',
            element: <ProfilePage />,
          },
          { path: '/recipes', element: <RecipesPage /> },
          { path: '/recipes/new', element: <RecipeEditorPage /> },
          { path: '/recipes/:recipeId', element: <RecipePage /> },
          { path: '/recipes/:recipeId/edit', element: <RecipeEditorPage /> },
        ],
      },
    ],
  },
]);
