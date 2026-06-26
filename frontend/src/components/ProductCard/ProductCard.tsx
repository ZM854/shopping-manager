import type { Product } from "../../models/product";
import ActionButton from "../UI/button/ActionButton/ActionButton";
import DeleteIcon from "../UI/svg/DeleteIcon/DeleteIcon";
import EditIcon from "../UI/svg/EditIcon/EditIcon";
import cls from "./ProductCard.module.css";

type ProductCardProps = {
  product: Product;
  onMarkChange: (id: number, isMarked: boolean) => void;
  removeProduct: (id: number) => void;
};

const ProductCard = ({
  product,
  onMarkChange,
  removeProduct,
}: ProductCardProps) => {
  return (
    <div className={cls.card}>
      <div className={cls.card_info}>
        <input
          type="checkbox"
          name="isMarked"
          id={`${product.id}`}
          checked={product.isMarked}
          onChange={(e) => {
            onMarkChange(product.id, e.target.checked);
          }}
        />
        <div>
          <h3>{product.name}</h3>
          <span>
            {product.quantity} {product.unit}
          </span>
        </div>
      </div>
      <div className={cls.card_controls}>
        <ActionButton onClick={() => removeProduct(product.id)}>
          <DeleteIcon className={cls.icon} />
        </ActionButton>
        <ActionButton>
          <EditIcon className={cls.icon} />
        </ActionButton>
      </div>
    </div>
  );
};

export default ProductCard;
