import { useState } from "react";
import type { Product, UpdateProductRequest } from "../../models/product";
import cls from "./ProductForm.module.css";
import type { ProductFormData } from "./ProductForm.types";
import Checkbox from "../UI/input/Checkbox/Checkbox";
import TextField from "../UI/input/TextInput/TextField";
import Button from "../UI/button/Button/Button";

type ProductFormProps = {
  product: Product | null;
  onSave: (productData: UpdateProductRequest) => void;
};

const ProductForm = ({ product, onSave }: ProductFormProps) => {
  const [formData, setFormData] = useState<ProductFormData>(
    product
      ? { ...product, quantity: product.quantity.toString() }
      : {
          name: "",
          quantity: "",
          unit: "",
          isMarked: false,
        },
  );

  const handleSubmit = (e: React.SubmitEvent) => {
    e.preventDefault();

    if (!formData.name.trim()) return;

    onSave({ ...formData, quantity: Number(formData.quantity) });
  };

  return (
    <form className={cls.form} onSubmit={handleSubmit}>
      <TextField
        label="Название"
        value={formData.name}
        placeholder="Например, Молоко"
        required
        onChange={(e) =>
          setFormData((prev) => ({
            ...prev,
            name: e.target.value,
          }))
        }
      />

      <TextField
        label="Количество"
        type="number"
        step="any"
        min={0}
        value={formData.quantity}
        onChange={(e) =>
          setFormData((prev) => ({
            ...prev,
            quantity: e.target.value,
          }))
        }
      />

      <TextField
        label="Единица измерения"
        placeholder="шт., кг, л..."
        value={formData.unit}
        onChange={(e) =>
          setFormData((prev) => ({
            ...prev,
            unit: e.target.value,
          }))
        }
      />

      {product && (
        <Checkbox
          label="Отметить как купленный"
          checked={formData.isMarked}
          onChange={(e) =>
            setFormData((prev) => ({
              ...prev,
              isMarked: e.target.checked,
            }))
          }
        />
      )}

      <Button type="submit">Сохранить</Button>
    </form>
  );
};

export default ProductForm;
