import type {
  ShoppingItem,
  UpdateShoppingItemRequest,
} from '../../models/shoppingList';
import ProductCard from '../ProductCard/ProductCard';
import cls from './ProductList.module.css';

type ProductListProps = {
  products: ShoppingItem[];
  pending?: boolean;
  editProduct: (product: ShoppingItem) => void;
  updateProduct: (id: string, data: UpdateShoppingItemRequest) => void;
  deleteProduct: (id: string) => void;
};

const ProductList = ({
  products,
  pending = false,
  editProduct,
  updateProduct,
  deleteProduct,
}: ProductListProps) => (
  <div className={cls.product_list}>
    {products.length === 0 ? (
      <p className={cls.feedback}>Список покупок пока пуст.</p>
    ) : (
      products.map((product) => (
        <ProductCard
          key={product.id}
          product={product}
          pending={pending}
          editProduct={editProduct}
          updateProduct={updateProduct}
          deleteProduct={deleteProduct}
        />
      ))
    )}
  </div>
);

export default ProductList;
