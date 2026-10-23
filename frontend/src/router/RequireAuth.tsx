import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { useAuth } from '../hooks/useAuth';

export default function RequireAuth() {
  const { isAuthenticated, isLoading, status, scope, user } = useAuth();
  const location = useLocation();

  if (isLoading || (status === 'error' && !user)) {
    return null;
  }

  if (!isAuthenticated && status !== 'error') {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return (
    <div hidden={status === 'error'} inert={status === 'error'}>
      <Outlet key={scope} />
    </div>
  );
}
