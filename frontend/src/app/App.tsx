import { Navigate, Route, Routes } from "react-router";
import type { ReactNode } from "react";
import { useAuth } from "./AuthProvider";
import { Shell } from "../components/Shell";
import { HelmMark } from "../components/icons";
import Login from "../routes/login/Login";
import Dashboard from "../routes/dashboard/Dashboard";
import PlayersRoute from "../routes/players/Players";
import ConsoleRoute from "../routes/console/Console";
import MapRoute from "../routes/map/Map";
import BackupsRoute from "../routes/backups/Backups";
import ConfigRoute from "../routes/config/Config";
import SettingsRoute from "../routes/settings/Settings";
import EventsRoute from "../routes/events/Events";

function FullPageLoader() {
  return (
    <div className="page-loader" aria-busy="true" aria-label="Loading Palhelm">
      <HelmMark size={48} className="helm-mark" wheelClassName="wheel" />
    </div>
  );
}

function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useAuth();
  if (status === "loading") return <FullPageLoader />;
  if (status === "unauthenticated") return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Shell />
          </RequireAuth>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="players" element={<PlayersRoute />} />
        <Route path="map" element={<MapRoute />} />
        <Route path="events" element={<EventsRoute />} />
        <Route path="console" element={<ConsoleRoute />} />
        <Route path="backups" element={<BackupsRoute />} />
        <Route path="config" element={<ConfigRoute />} />
        <Route path="settings" element={<SettingsRoute />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
