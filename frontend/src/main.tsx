import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { ThemeProvider } from "next-themes"
import { RouterProvider } from "react-router"

import { router } from "@/app/router"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import "./index.css"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <TooltipProvider>
        <RouterProvider router={router} />
        <Toaster richColors position="top-center" />
      </TooltipProvider>
    </ThemeProvider>
  </StrictMode>
)
