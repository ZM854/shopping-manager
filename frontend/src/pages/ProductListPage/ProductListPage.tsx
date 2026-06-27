import ProductList from "../../components/ProductList/ProductList";
import ActionButton from "../../components/UI/button/ActionButton/ActionButton";
import AddIcon from "../../components/UI/svg/AddIcon/AddIcon";
import { useProducts } from "../../hooks/useProducts";
import cls from "./ProductListPage.module.css";

const ProductListPage = () => {
  const { products, error, createProduct, updateProduct, deleteProduct } =
    useProducts();

  return (
    <div>
      <ProductList
        products={products}
        error={error}
        updateProduct={updateProduct}
        deleteProduct={deleteProduct}
      />
      <ActionButton
        onClick={() =>
          createProduct({
            name: "duncan",
            quantity: 1,
            unit: "football fields",
          })
        }
      >
        <AddIcon className={cls.addIcon} />
      </ActionButton>
    </div>
  );
};

export default ProductListPage;
