import type { Product } from "../models/product";
import { apiFetch } from "./api";

export async function getProducts() {
  return apiFetch<Product[]>("/products");
}

export async function getProductById(id: number) {
  return apiFetch<Product>(`/products/${id}`);
}

export async function postProduct(product: Omit<Product, "id">) {
  return apiFetch<Product>("/products", {
    method: "POST",
    body: JSON.stringify(product),
    headers: {
      "Content-Type": "application/json",
    },
  });
}

// TODO: rewrite methods with id
export async function updateProduct(product: Product) {
  return apiFetch<Product>("/products", {
    method: "PUT",
    body: JSON.stringify(product),
    headers: {
      "Content-Type": "application/json",
    },
  });
}

export async function deleteProduct(product: Product) {
  return apiFetch<Product>("/products", {
    method: "DELETE",
    body: JSON.stringify(product),
    headers: {
      "Content-Type": "application/json",
    },
  });
}
