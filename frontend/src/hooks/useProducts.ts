import { useEffect, useState } from "react";
import type {
  CreateProductRequest,
  Product,
  UpdateProductRequest,
} from "../models/product";
import ProductService from "../services/productService";

export function useProducts() {
  const [products, setProducts] = useState<Product[]>([]);
  const [error, setError] = useState<string | null>(null);

  const createProduct = async (productData: CreateProductRequest) => {
    try {
      const product = await ProductService.postProduct(productData);
      setProducts((prev) => [...prev, product]);
      return product;
    } catch (error) {
      setError("failed to create product");
      throw error;
    }
  };

  const updateProduct = async (
    id: number,
    productData: UpdateProductRequest,
  ) => {
    const productToUpdate = products.find((product) => product.id === id);

    if (!productToUpdate) {
      setError("product not found");
      return;
    }

    try {
      const updatedProduct = await ProductService.updateProduct(
        id,
        productData,
      );

      setProducts((prev) =>
        prev.map((product) =>
          product.id === id ? { ...updatedProduct } : product,
        ),
      );
    } catch (e) {
      console.log(e);
    }
  };

  const deleteProduct = async (id: number) => {
    try {
      await ProductService.deleteProduct(id);
      setProducts((prev) => prev.filter((product) => product.id !== id));
    } catch (error) {
      setError("failed to delete product");
      throw error;
    }
  };

  useEffect(() => {
    ProductService.getProducts()
      .then(setProducts)
      .catch(() => setError("failed to load products"))
      .finally();
  }, []);

  return {
    products,
    error,
    createProduct,
    updateProduct,
    deleteProduct,
  };
}
