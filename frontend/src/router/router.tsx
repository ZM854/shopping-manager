import { createBrowserRouter } from "react-router-dom";

import Layout from "../layout/Layout";

import RequireAuth from "./RequireAuth";

import ShoppingListPage from "../pages/ShoppingListPage/ShoppingListPage";
import ProfilePage from "../pages/ProfilePage/ProfilePage";

import LoginPage from "../pages/LoginPage/LoginPage";
import RegistrationPage from "../pages/RegistrationPage/RegistrationPage";
import ActivationPage from "../pages/ActivationPage/ActivationPage";

export const router = createBrowserRouter([
  {
    path: "/login",
    element: <LoginPage />,
  },
  {
    path: "/registration",
    element: <RegistrationPage />,
  },
  {
    path: "/activation/:token",
    element: <ActivationPage />,
  },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <Layout />,
        children: [
          {
            path: "/",
            element: <ShoppingListPage />,
          },
          {
            path: "/profile",
            element: <ProfilePage />,
          },
        ],
      },
    ],
  },
]);
