import type { Product } from "../../models/product";

type ProductCardProps = {
  product: Product;
  onMarkChange: (id: number, isMarked: boolean) => void;
};

const ProductCard = ({ product, onMarkChange }: ProductCardProps) => {
  return (
    <div>
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
  );
};

export default ProductCard;
