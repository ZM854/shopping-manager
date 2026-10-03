import type { ButtonHTMLAttributes, ReactNode } from 'react';
import cls from './IconButton.module.css';

type IconButtonProps = {
  children: ReactNode;
  variant?: 'filled' | 'ghost';
  tone?: 'default' | 'danger';
} & ButtonHTMLAttributes<HTMLButtonElement>;

const IconButton = ({
  children,
  className = '',
  variant = 'ghost',
  tone = 'default',
  ...props
}: IconButtonProps) => {
  return (
    <button
      className={`${cls.button} ${cls[variant]} ${cls[tone]} ${className}`}
      {...props}
    >
      {children}
    </button>
  );
};

export default IconButton;
