import type { Product, UpdateProductRequest } from "../../models/product";
import ProductCard from "../ProductCard/ProductCard";
import cls from "./ProductList.module.css";

type ProductListProps = {
  products: Product[];
  updateProduct: (id: number, productData: UpdateProductRequest) => void;
  deleteProduct: (id: number) => void;
  error: string | null;
};

const ProductList = ({
  products,
  updateProduct,
  error,
  deleteProduct: deleteProduct,
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
            updateProduct={updateProduct}
            deleteProduct={deleteProduct}
          />
        ))
      )}
    </div>
  );
};

export default ProductList;
