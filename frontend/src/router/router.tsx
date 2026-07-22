import { createBrowserRouter } from "react-router-dom";
import Layout from "../layout/Layout";
import ShoppingListPage from "../pages/ShoppingListPage/ShoppingListPage";
import ProfilePage from "../pages/ProfilePage/ProfilePage";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Layout />,
    children: [
      {
        index: true,
        element: <ShoppingListPage />,
      },
      {
        path: "profile",
        element: <ProfilePage />,
      },
    ],
  },
]);
