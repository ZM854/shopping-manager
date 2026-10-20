import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

export default function RequireAuth() {
  const { isAuthenticated, isLoading, status, scope } = useAuth();
  const location = useLocation();

  if (isLoading || status === 'error') {
    return null;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <Outlet key={scope} />;
}
