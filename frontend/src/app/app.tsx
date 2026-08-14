import { Route, Routes } from "react-router-dom"

import { AppShell } from "@/app/app-shell"
import { DashboardPage } from "@/features/dashboard/page"
import { GamesPage } from "@/features/games/page"
import { MoviesPage } from "@/features/movies/page"
import { NotFoundPage } from "@/features/not-found/page"
import { SettingsPage } from "@/features/settings/page"
import { TVPage } from "@/features/tv/page"

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<DashboardPage />} />
        <Route path="games" element={<GamesPage />} />
        <Route path="movies" element={<MoviesPage />} />
        <Route path="tv" element={<TVPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
