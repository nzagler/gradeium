import { Route, Routes } from "react-router-dom"

import { AppShell } from "@/app/app-shell"
import { DashboardPage } from "@/features/dashboard/page"
import { AddGamePage, GameDetailPage, GamesPage } from "@/features/games/page"
import { AddMoviePage, MovieDetailPage, MoviesPage } from "@/features/movies/page"
import { NotFoundPage } from "@/features/not-found/page"
import { SettingsPage } from "@/features/settings/page"
import { SetupGate } from "@/features/setup/setup-gate"
import { AddTVPage, TVDetailPage, TVPage } from "@/features/tv/page"
import { ThemeProvider } from "@/features/theme/theme-provider"

export function App() {
  return (
    <ThemeProvider>
      <SetupGate>
        <Routes>
          <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="games" element={<GamesPage />} />
          <Route path="games/backlog" element={<GamesPage backlog />} />
          <Route path="games/add" element={<AddGamePage />} />
          <Route path="games/:id" element={<GameDetailPage />} />
          <Route path="movies" element={<MoviesPage />} />
          <Route path="movies/backlog" element={<MoviesPage backlog />} />
          <Route path="movies/add" element={<AddMoviePage />} />
          <Route path="movies/:id" element={<MovieDetailPage />} />
          <Route path="tv" element={<TVPage />} />
          <Route path="tv/backlog" element={<TVPage backlog />} />
          <Route path="tv/add" element={<AddTVPage />} />
          <Route path="tv/:id" element={<TVDetailPage />} />
          <Route path="settings/:section?" element={<SettingsPage />} />
          <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </SetupGate>
    </ThemeProvider>
  )
}
