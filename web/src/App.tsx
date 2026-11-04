import { useEffect } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import { Background } from "./components/ui";
import { useBootstrap, useMe } from "./lib/hooks";
import { LoginPage, SetupPage } from "./pages/Auth";
import HomePage from "./pages/Home";
import SettingsPage from "./pages/Settings";

function Protected({ children }: { children: React.ReactNode }) {
  const { data: me, isLoading } = useMe();
  const { data: boot } = useBootstrap();
  const loc = useLocation();
  if (isLoading) return null;
  if (!me) return <Navigate to={boot?.needsSetup ? "/setup" : "/login"} replace />;
  // Mandatory 2FA: funnel the user to the security page until enrolled.
  if (me.mustEnroll2fa && !loc.pathname.startsWith("/settings")) return <Navigate to="/settings/security" replace />;
  return <>{children}</>;
}

export default function App() {
  const { data: boot } = useBootstrap();
  useEffect(() => {
    document.title = boot?.siteName ?? "YdisksPanel";
  }, [boot?.siteName]);
  return (
    <>
      <Background wallpaper={boot?.wallpaper} />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/settings/*" element={<Protected><SettingsPage /></Protected>} />
        <Route path="/" element={<Protected><HomePage /></Protected>} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
}
