import { Outlet } from 'react-router-dom';
import cls from './Layout.module.css';
import BottomNavigation from './BottomNavigation/BottomNavigation';

const Layout = () => {
  return (
    <div className={cls.layout}>
      <main className={cls.content}>
        <Outlet />
      </main>

      <BottomNavigation />
    </div>
  );
};

export default Layout;
