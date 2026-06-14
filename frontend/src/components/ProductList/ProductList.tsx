import { useProducts } from "../../hooks/useProducts";
import ProductCard from "../ProductCard/ProductCard";
import cls from "./ProductList.module.css";

const ProductList = () => {
  const { products, error, toggleMark } = useProducts();

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
          />
        ))
      )}
    </div>
  );
};

export default ProductList;
