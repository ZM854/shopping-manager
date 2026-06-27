import type { ReactNode } from "react";
import cls from "./modal.module.css";

type ModalProps = {
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
};

const Modal = ({ isOpen, onClose, children }: ModalProps) => {
  if (!isOpen) {
    return null;
  }
  return (
    <div className={cls.backdrop} onClick={onClose}>
      <div className={cls.modal} onClick={(e) => e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
};

export default Modal;
