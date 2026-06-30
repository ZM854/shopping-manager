import type { ButtonHTMLAttributes, ReactNode } from "react";
import cls from "./Button.module.css";

type ButtonProps = {
  children: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>;

const Button = ({ children, ...props }: ButtonProps) => {
  return (
    <button className={cls.button} {...props}>
      {children}
    </button>
  );
};

export default Button;
