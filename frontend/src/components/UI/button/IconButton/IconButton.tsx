import type { ButtonHTMLAttributes, ReactNode } from 'react';
import cls from './IconButton.module.css';

type IconButtonProps = {
  children: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>;

const IconButton = ({
  children,
  className = '',
  ...props
}: IconButtonProps) => {
  return (
    <button className={`${cls.button} ${className}`} {...props}>
      {children}
    </button>
  );
};

export default IconButton;
