import cls from './TopAppBar.module.css';
import type { ReactNode } from 'react';

interface TopAppBarProps {
  title?: string;
  actionIcon?: ReactNode;
  onActionButtonClick?: () => void;
}

const TopAppBar = ({
  title,
  actionIcon,
  onActionButtonClick,
}: TopAppBarProps) => {
  return (
    <header className={cls.topAppBar}>
      <div className={cls.leftSlot}>
        {title && <h1 className={cls.title}>{title}</h1>}
      </div>
      <div className={cls.rightSlot}>
        {actionIcon && (
          <button
            className={cls.iconButton}
            type="button"
            onClick={onActionButtonClick}
          >
            {actionIcon}
          </button>
        )}
      </div>
    </header>
  );
};

export default TopAppBar;
