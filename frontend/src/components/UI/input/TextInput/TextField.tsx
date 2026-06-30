import { useId } from "react";
import cls from "./TextField.module.css";

type TextFieldProps = React.InputHTMLAttributes<HTMLInputElement> & {
  label: string;
};

const TextField = ({ label, id, className, ...props }: TextFieldProps) => {
  const textFieldId = useId();
  const inputId = id ?? textFieldId;

  return (
    <div className={cls.field}>
      <label htmlFor={inputId} className={cls.label}>
        {label}
      </label>

      <input
        id={inputId}
        className={`${cls.input} ${className ?? ""}`}
        {...props}
      />
    </div>
  );
};

TextField.displayName = "TextField";

export default TextField;
