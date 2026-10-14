import { createBrowserRouter } from "react-router"

import { AppLayout } from "@/app/app-layout"
import { HomePage } from "@/app/home-page"
import { modules } from "@/app/modules"
import { NotFoundPage } from "@/app/not-found-page"

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppLayout />,
    children: [
      { index: true, element: <HomePage /> },
      ...modules.flatMap((m) => m.routes),
      { path: "*", element: <NotFoundPage />, handle: { title: "页面不存在" } },
    ],
  },
])
