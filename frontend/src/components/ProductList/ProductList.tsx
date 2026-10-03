import type { Product, UpdateProductRequest } from "../../models/product";
import ProductCard from "../ProductCard/ProductCard";
import cls from "./ProductList.module.css";

type ProductListProps = {
  products: Product[];
  editProduct: (product: Product) => void;
  updateProduct: (id: number, productData: UpdateProductRequest) => void;
  deleteProduct: (id: number) => void;
  error: string | null;
};

const ProductList = ({
  products,
  editProduct,
  updateProduct,
  deleteProduct,
  error,
}: ProductListProps) => {
  return (
    <div className={cls.product_list}>
      {error ? (
        <p role="alert" className={`${cls.feedback} ${cls.error}`}>
          Не удалось загрузить список покупок. Попробуйте ещё раз позже.
        </p>
      ) : products.length === 0 ? (
        <p className={cls.feedback}>Список покупок пока пуст.</p>
      ) : (
        products.map((product) => (
          <ProductCard
            key={product.id}
            product={product}
            editProduct={editProduct}
            updateProduct={updateProduct}
            deleteProduct={deleteProduct}
          />
        ))
      )}
    </div>
  );
};

export default ProductList;
