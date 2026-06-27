import { useState } from "react";
import type { Product, UpdateProductRequest } from "../../models/product";
import cls from "./ProductForm.module.css";

type ProductFormProps = {
  product: Product;
  onSave: (productData: UpdateProductRequest) => void;
};

const ProductForm = ({ product, onSave }: ProductFormProps) => {
  const [formData, setFormData] = useState(product);

  const handleSubmit = (e: React.SubmitEvent) => {
    e.preventDefault();
    if (!formData.name.trim()) {
      return;
    }
    onSave({
      name: formData.name,
      quantity: formData.quantity,
      isMarked: formData.isMarked,
      unit: formData.unit,
    });
  };

  return (
    <form className={cls.form} onSubmit={handleSubmit}>
      <input
        type="text"
        value={formData.name}
        onChange={(e) =>
          setFormData((prev) => ({ ...prev, name: e.target.value }))
        }
      />
      <input
        type="number"
        value={formData.quantity}
        onChange={(e) =>
          setFormData((prev) => ({ ...prev, quantity: Number(e.target.value) }))
        }
      />
      <input
        type="text"
        value={formData.unit}
        onChange={(e) =>
          setFormData((prev) => ({ ...prev, unit: e.target.value }))
        }
      />
      <input
        type="checkbox"
        checked={formData.isMarked}
        onChange={(e) =>
          setFormData((prev) => ({ ...prev, isMarked: e.target.checked }))
        }
      />
      <input type="submit" value="Сохранить" />
    </form>
  );
};

export default ProductForm;
