import cls from './TopAppBar.module.css';
import type { ReactNode } from 'react';

interface TopAppBarProps {
  title?: string;
  actionIcon?: ReactNode;
  onActionButtonClick?: () => void;
  actionLabel?: string;
}

const TopAppBar = ({
  title,
  actionIcon,
  onActionButtonClick,
  actionLabel = 'Действие',
}: TopAppBarProps) => {
  return (
    <header className={cls.topAppBar}>
      <div className={cls.content}>
        <div className={cls.leftSlot}>
          {title && <h1 className={cls.title}>{title}</h1>}
        </div>
        <div className={cls.rightSlot}>
          {actionIcon && (
            <button
              className={cls.iconButton}
              type="button"
              onClick={onActionButtonClick}
              aria-label={actionLabel}
            >
              {actionIcon}
            </button>
          )}
        </div>
      </div>
    </header>
  );
};

export default TopAppBar;
