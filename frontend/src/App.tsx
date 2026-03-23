import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { useAuthStore } from '@/stores/auth';
import MainLayout from '@/components/Layout/MainLayout';
import Login from '@/pages/Login';
import Dashboard from '@/pages/Dashboard';
import Tasks from '@/pages/Tasks';
import TaskDetail from '@/pages/TaskDetail';
import Tenants from '@/pages/Tenants';
import TenantDetail from '@/pages/TenantDetail';
import TiDBConfigs from '@/pages/TiDBConfigs';
import S3Configs from '@/pages/S3Configs';
import Statistics from '@/pages/Statistics';

function PrivateRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuthStore();
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" replace />;
}

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="/"
          element={
            <PrivateRoute>
              <MainLayout />
            </PrivateRoute>
          }
        >
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="tasks" element={<Tasks />} />
          <Route path="tasks/:id" element={<TaskDetail />} />
          <Route path="tenants" element={<Tenants />} />
          <Route path="tenants/:id" element={<TenantDetail />} />
          <Route path="tidb-configs" element={<TiDBConfigs />} />
          <Route path="s3-configs" element={<S3Configs />} />
          <Route path="statistics" element={<Statistics />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
