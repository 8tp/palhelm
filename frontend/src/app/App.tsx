import { Navigate, Route, Routes } from "react-router";
import { lazy, Suspense, type ReactNode } from "react";
import { useAuth } from "./AuthProvider";
import { Shell } from "../components/Shell";
import { HelmMark } from "../components/icons";
import Login from "../routes/login/Login";

const Dashboard = lazy(() => import("../routes/dashboard/Dashboard"));
const PlayersRoute = lazy(() => import("../routes/players/Players"));
const PalsRoute = lazy(() => import("../routes/pals/Pals"));
const ConsoleRoute = lazy(() => import("../routes/console/Console"));
const MapRoute = lazy(() => import("../routes/map/Map"));
const BackupsRoute = lazy(() => import("../routes/backups/Backups"));
const ConfigRoute = lazy(() => import("../routes/config/Config"));
const SettingsRoute = lazy(() => import("../routes/settings/Settings"));
const EventsRoute = lazy(() => import("../routes/events/Events"));
const DiagnosticsRoute = lazy(() => import("../routes/diagnostics/Diagnostics"));

function FullPageLoader() {
  return (
    <div className="page-loader" aria-busy="true" aria-label="Loading Palhelm">
      <HelmMark size={48} className="helm-mark" wheelClassName="wheel" />
    </div>
  );
}

function RouteLoader() {
  return (
    <div className="route-loader" role="status" aria-busy="true" aria-label="Loading page">
      <HelmMark size={40} className="helm-mark" wheelClassName="wheel" />
      <span>Loading page…</span>
    </div>
  );
}

function lazyRoute(element: ReactNode) {
  return <Suspense fallback={<RouteLoader />}>{element}</Suspense>;
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
        <Route index element={lazyRoute(<Dashboard />)} />
        <Route path="players" element={lazyRoute(<PlayersRoute />)} />
        <Route path="pals" element={lazyRoute(<PalsRoute />)} />
        <Route path="map" element={lazyRoute(<MapRoute />)} />
        <Route path="events" element={lazyRoute(<EventsRoute />)} />
        <Route path="console" element={lazyRoute(<ConsoleRoute />)} />
        <Route path="backups" element={lazyRoute(<BackupsRoute />)} />
        <Route path="config" element={lazyRoute(<ConfigRoute />)} />
        <Route path="diagnostics" element={lazyRoute(<DiagnosticsRoute />)} />
        <Route path="settings" element={lazyRoute(<SettingsRoute />)} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
