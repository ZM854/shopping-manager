import { useState } from 'react';
import ProductForm from '../../components/ProductForm/ProductForm';
import ProductList from '../../components/ProductList/ProductList';
import Modal from '../../components/UI/modal/Modal/Modal';
import AddIcon from '../../components/UI/svg/AddIcon/AddIcon';
import { useModal } from '../../hooks/useModal';
import { useProducts } from '../../hooks/useProducts';
import type { Product, UpdateProductRequest } from '../../models/product';
import { useScaffold } from '../../hooks/useScaffold.ts';
import DeleteIcon from '../../components/UI/svg/DeleteIcon/DeleteIcon.tsx';

const ShoppingListPage = () => {
  const {
    products,
    error,
    createProduct,
    updateProduct,
    deleteProduct,
    deleteAllProducts,
  } = useProducts();
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

  useScaffold({
    fab: {
      onClick: handleCreate,
      icon: <AddIcon />,
    },
    topBar: {
      actionIcon: <DeleteIcon />,
      title: 'Покупки',
      onActionClick: deleteAllProducts,
    },
  });

  return (
    <div>
      <ProductList
        products={products}
        error={error}
        editProduct={handleEdit}
        updateProduct={updateProduct}
        deleteProduct={deleteProduct}
      />

      <Modal isOpen={modal.isOpen} onClose={handleModalClose}>
        <ProductForm product={editingProduct} onSave={handleFormSave} />
      </Modal>
    </div>
  );
};

export default ShoppingListPage;
