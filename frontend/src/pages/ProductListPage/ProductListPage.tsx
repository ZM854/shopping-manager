import ProductCard from "../../components/ProductCard/ProductCard";
import { useProducts } from "../../hooks/useProducts";
import cls from "./ProductListPage.module.css";

const ProductListPage = () => {
  const { products, error, toggleMark } = useProducts();

  return (
    <div className={cls.productList}>
      <div className={cls.productList}>
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
    </div>
  );
};

export default ProductListPage;
