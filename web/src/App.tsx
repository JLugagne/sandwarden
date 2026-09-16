import { Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "@/components/Layout";
import { KitsPage } from "@/pages/KitsPage";
import { ProfilesPage } from "@/pages/ProfilesPage";
import { SandboxDetailPage } from "@/pages/SandboxDetailPage";
import { SandboxesPage } from "@/pages/SandboxesPage";
import { SecretsPage } from "@/pages/SecretsPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { SkillsPage } from "@/pages/SkillsPage";
import { TrafficPage } from "@/pages/TrafficPage";

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<SandboxesPage />} />
        <Route path="sandboxes/:name" element={<SandboxDetailPage />} />
        <Route path="profiles" element={<ProfilesPage />} />
        <Route path="skills" element={<SkillsPage />} />
        <Route path="kits" element={<KitsPage />} />
        <Route path="traffic" element={<TrafficPage />} />
        <Route path="secrets" element={<SecretsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
