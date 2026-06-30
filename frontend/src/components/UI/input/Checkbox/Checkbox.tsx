import { useId } from "react";
import CheckIcon from "../../svg/CheckIcon/CheckIcon";
import cls from "./Checkbox.module.css";

type CheckboxProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label?: string;
};

const Checkbox = ({ label, id, ...props }: CheckboxProps) => {
  const checkboxId = useId();

  const inputId = id ?? checkboxId;

  return (
    <label htmlFor={inputId} className={cls.container}>
      <input id={inputId} type="checkbox" className={cls.input} {...props} />

      <span className={cls.box}>
        <CheckIcon className={cls.icon} />
      </span>

      {label ? <span>{label}</span> : null}
    </label>
  );
};

export default Checkbox;
