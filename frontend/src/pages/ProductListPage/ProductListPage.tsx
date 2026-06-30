import { useState } from "react";
import ProductForm from "../../components/ProductForm/ProductForm";
import ProductList from "../../components/ProductList/ProductList";
import IconButton from "../../components/UI/button/ActionButton/IconButton";
import Modal from "../../components/UI/modal/Modal/Modal";
import AddIcon from "../../components/UI/svg/AddIcon/AddIcon";
import { useModal } from "../../hooks/useModal";
import { useProducts } from "../../hooks/useProducts";
import type { Product, UpdateProductRequest } from "../../models/product";
import cls from "./ProductListPage.module.css";

const ProductListPage = () => {
  const { products, error, createProduct, updateProduct, deleteProduct } =
    useProducts();
  const modal = useModal();
  const [editingProduct, setEditingProduct] = useState<Product | null>(null);

  const handleEdit = (product: Product) => {
    setEditingProduct(product);
    modal.open();
  };

  const handleCreate = () => {
    setEditingProduct(null);
    modal.open();
  };

  const handleModalClose = () => {
    setEditingProduct(null);
    modal.close();
  };

  const handleFormSave = async (productData: UpdateProductRequest) => {
    if (editingProduct) {
      await updateProduct(editingProduct.id, {
        name: productData.name,
        quantity: productData.quantity,
        unit: productData.unit,
        isMarked: productData.isMarked,
      });
    } else {
      await createProduct({
        name: productData.name,
        quantity: productData.quantity,
        unit: productData.unit,
      });
    }
    setEditingProduct(null);
    modal.close();
  };

  return (
    <div>
      <ProductList
        products={products}
        error={error}
        editProduct={handleEdit}
        updateProduct={updateProduct}
        deleteProduct={deleteProduct}
      />
      <IconButton onClick={handleCreate}>
        <AddIcon className={cls.addIcon} />
      </IconButton>

      <Modal isOpen={modal.isOpen} onClose={handleModalClose}>
        <ProductForm product={editingProduct} onSave={handleFormSave} />
      </Modal>
    </div>
  );
};

export default ProductListPage;
