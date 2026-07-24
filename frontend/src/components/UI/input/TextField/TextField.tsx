import { forwardRef, useId, type InputHTMLAttributes } from "react";
import cls from "./TextField.module.css";

type TextFieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label?: string;
  error?: string;
};

const TextField = forwardRef<HTMLInputElement, TextFieldProps>(
  ({ label, id, className, error, ...props }: TextFieldProps, ref) => {
    const textFieldId = useId();
    const inputId = id ?? textFieldId;

    return (
      <div className={cls.field}>
        {label && (
          <label htmlFor={inputId} className={cls.label}>
            {label}
          </label>
        )}
        <input
          ref={ref}
          id={inputId}
          aria-invalid={!!error}
          className={[cls.input, error ? cls.errorInput : "", className ?? ""]
            .filter(Boolean)
            .join(" ")}
          {...props}
        />
        {error && (
          <span role="alert" className={cls.error}>
            {error}
          </span>
        )}
      </div>
    );
  },
);

TextField.displayName = "TextField";

export default TextField;
