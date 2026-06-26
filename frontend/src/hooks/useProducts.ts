import { useEffect, useState } from "react";
import type { CreateProductRequest, Product } from "../models/product";
import {
  deleteProduct,
  getProducts,
  postProduct,
  updateProduct,
} from "../services/productService";

export function useProducts() {
  const [products, setProducts] = useState<Product[]>([]);
  const [error, setError] = useState<string | null>(null);

  const toggleMark = async (id: number, isMarked: boolean) => {
    const productToUpdate = products.find((product) => product.id === id);

    if (!productToUpdate) {
      setError("product not found");
      return;
    }

    try {
      const { id: _, ...productData } = {
        ...productToUpdate,
        isMarked,
      };

      await updateProduct(id, productData);

      setProducts((prev) =>
        prev.map((product) =>
          product.id === id ? { ...product, isMarked } : product,
        ),
      );
    } catch (e) {
      console.log(e);
    }
  };

  const createProduct = async (productData: CreateProductRequest) => {
    try {
      const product = await postProduct(productData);
      setProducts((prev) => [...prev, product]);
      return product;
    } catch (error) {
      setError("failed to create product");
      throw error;
    }
  };

  const removeProduct = async (id: number) => {
    try {
      await deleteProduct(id);
      setProducts((prev) => prev.filter((product) => product.id !== id));
    } catch (error) {
      setError("failed to delete product");
      throw error;
    }
  };

  useEffect(() => {
    getProducts()
      .then(setProducts)
      .catch(() => setError("failed to load products"))
      .finally();
  }, []);

  return {
    products,
    error,
    toggleMark,
    createProduct,
    removeProduct,
  };
}
