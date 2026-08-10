import { Outlet } from 'react-router-dom';
import cls from './Layout.module.css';
import BottomNavigation from './BottomNavigation/BottomNavigation';
import { useScaffoldState } from '../hooks/useScaffold.ts';
import IconButton from '../components/UI/button/IconButton/IconButton.tsx';
import ScaffoldProvider from '../context/scaffoldContext/ScaffoldProvider.tsx';

const LayoutContent = () => {
  const { fab } = useScaffoldState();

  return (
    <div className={cls.layout}>
      <main className={cls.content}>
        <Outlet />
      </main>
      {fab && (
        <IconButton className={cls.fab} type="button" onClick={fab.onClick}>
          {fab.icon}
        </IconButton>
      )}
      <BottomNavigation />
    </div>
  );
};

const Layout = () => {
  return (
    <ScaffoldProvider>
      <LayoutContent />
    </ScaffoldProvider>
  );
};

export default Layout;
