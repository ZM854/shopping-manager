import type { ButtonHTMLAttributes, ReactNode } from "react";
import cls from "./IconButton.module.css";

type ActionButtonProps = {
  children: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>;

const IconButton = ({ children, ...props }: ActionButtonProps) => {
  return (
    <button className={cls.button} {...props}>
      {children}
    </button>
  );
};

export default IconButton;
