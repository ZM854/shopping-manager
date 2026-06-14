import type { ButtonHTMLAttributes, ReactNode } from "react";
import cls from "./ActionButton.module.css";

type ActionButtonProps = {
  children: ReactNode;
} & ButtonHTMLAttributes<HTMLButtonElement>;

const ActionButton = ({ children, ...props }: ActionButtonProps) => {
  return (
    <button className={cls.button} {...props}>
      {children}
    </button>
  );
};

export default ActionButton;
