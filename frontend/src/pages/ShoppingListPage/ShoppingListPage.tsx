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
import AlertDialog from '../../components/AlertDialog/AlertDialog.tsx';

const ShoppingListPage = () => {
  const {
    products,
    error,
    createProduct,
    updateProduct,
    deleteProduct,
    deleteAllProducts,
  } = useProducts();

  const productFormModal = useModal();
  const dialogModal = useModal();

  const [editingProduct, setEditingProduct] = useState<Product | null>(null);

  const handleEdit = (product: Product) => {
    setEditingProduct(product);
    productFormModal.open();
  };

  const handleCreate = () => {
    setEditingProduct(null);
    productFormModal.open();
  };

  const handleProductModalClose = () => {
    setEditingProduct(null);
    productFormModal.close();
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
    productFormModal.close();
  };

  const handleDialogModalOpen = () => {
    dialogModal.open();
  };

  const handleDialogModalClose = () => {
    dialogModal.close();
  };

  const handleDeleteAllProducts = () => {
    deleteAllProducts();
    dialogModal.close();
  };

  useScaffold({
    fab: {
      onClick: handleCreate,
      icon: <AddIcon />,
    },
    topBar: {
      actionIcon: <DeleteIcon />,
      title: 'Покупки',
      onActionClick: handleDialogModalOpen,
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

      <Modal isOpen={productFormModal.isOpen} onClose={handleProductModalClose}>
        <ProductForm product={editingProduct} onSave={handleFormSave} />
      </Modal>

      <Modal isOpen={dialogModal.isOpen} onClose={handleDialogModalClose}>
        <AlertDialog
          title="Очистить весь список?"
          message="Это действие невозможно отменить."
          onDiscard={handleDialogModalClose}
          onConfirm={handleDeleteAllProducts}
          confirmText="Удалить"
          isDanger={true}
        />
      </Modal>
    </div>
  );
};

export default ShoppingListPage;
