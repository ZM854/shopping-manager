import type { Product } from "../../models/product";
import ProductCard from "../ProductCard/ProductCard";
import cls from "./ProductList.module.css";

type ProductListProps = {
  products: Product[];
  toggleMark: (id: number, isMarked: boolean) => void;
  removeProduct: (id: number) => void;
  error: string | null;
};

const ProductList = ({
  products,
  toggleMark,
  error,
  removeProduct,
}: ProductListProps) => {
  return (
    <div className={cls.product_list}>
      {error ? (
        <span>{error}</span>
      ) : (
        products.map((product) => (
          <ProductCard
            key={product.id}
            product={product}
            onMarkChange={toggleMark}
            removeProduct={removeProduct}
          />
        ))
      )}
    </div>
  );
};

export default ProductList;
