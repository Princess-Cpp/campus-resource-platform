import { createBrowserRouter } from "react-router"

import { AppLayout } from "@/app/app-layout"
import { HomePage } from "@/app/home-page"
import { modules } from "@/app/modules"
import { NotFoundPage } from "@/app/not-found-page"
import { LoginPage } from "@/modules/auth/pages/login-page"
import { RegisterPage } from "@/modules/auth/pages/register-page"
import { RequireAuth } from "@/modules/common/require-auth"

export const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  { path: "/register", element: <RegisterPage /> },
  {
    element: <RequireAuth />,
    children: [
      {
        path: "/",
        element: <AppLayout />,
        children: [
          { index: true, element: <HomePage /> },
          ...modules.flatMap((m) => m.routes),
          { path: "*", element: <NotFoundPage />, handle: { title: "页面不存在" } },
        ],
      },
    ],
  },
])
