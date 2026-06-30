import type { Product, UpdateProductRequest } from "../../models/product";
import IconButton from "../UI/button/ActionButton/IconButton";
import Checkbox from "../UI/input/Checkbox/Checkbox";
import DeleteIcon from "../UI/svg/DeleteIcon/DeleteIcon";
import EditIcon from "../UI/svg/EditIcon/EditIcon";
import cls from "./ProductCard.module.css";

type ProductCardProps = {
  product: Product;
  editProduct: (product: Product) => void;
  updateProduct: (id: number, ProductData: UpdateProductRequest) => void;
  deleteProduct: (id: number) => void;
};

const ProductCard = ({
  product,
  editProduct,
  updateProduct,
  deleteProduct,
}: ProductCardProps) => {
  return (
    <div className={cls.card}>
      <div className={cls.card_info}>
        <Checkbox
          label=""
          name="isMarked"
          id={`${product.id}`}
          checked={product.isMarked}
          onChange={(e) => {
            updateProduct(product.id, {
              ...product,
              isMarked: e.target.checked,
            });
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
        <IconButton onClick={() => deleteProduct(product.id)}>
          <DeleteIcon className={cls.icon} />
        </IconButton>
        <IconButton onClick={() => editProduct(product)}>
          <EditIcon className={cls.icon} />
        </IconButton>
      </div>
    </div>
  );
};

export default ProductCard;
