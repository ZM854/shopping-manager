import type { ButtonHTMLAttributes, ReactNode } from 'react';
import cls from './Button.module.css';

type ButtonProps = {
  children: ReactNode;
  variant?: 'outlined' | 'filled';
  tone?: 'default' | 'danger';
} & ButtonHTMLAttributes<HTMLButtonElement>;

const Button = ({
  children,
  variant = 'filled',
  tone = 'default',
  className,
  ...props
}: ButtonProps) => {
  return (
    <button
      className={`${cls.button} ${cls[variant]} ${cls[tone]} ${className ?? ''}`}
      {...props}
    >
      {children}
    </button>
  );
};

export default Button;
