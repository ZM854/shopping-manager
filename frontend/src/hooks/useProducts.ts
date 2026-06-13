import { useEffect, useState } from "react";
import type { Product } from "../models/product";
import { getProducts, updateProduct } from "../services/productService";

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

  useEffect(() => {
    getProducts()
      .then(setProducts)
      .catch(() => setError("Ошибка загрузки данных"))
      .finally();
  }, []);

  return {
    products,
    error,
    toggleMark,
  };
}
