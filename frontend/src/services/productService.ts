import type {
  CreateProductRequest,
  Product,
  UpdateProductRequest,
} from "../models/product";
import { apiFetch } from "./api";

export default class ProductService {
  static async getProducts() {
    return apiFetch<Product[]>("/products");
  }

  static async getProductById(id: number) {
    return apiFetch<Product>(`/products/${id}`);
  }

  static async postProduct(product: CreateProductRequest) {
    return apiFetch<Product>("/products", {
      method: "POST",
      body: JSON.stringify(product),
      headers: {
        "Content-Type": "application/json",
      },
    });
  }

  static async updateProduct(id: number, product: UpdateProductRequest) {
    return apiFetch<Product>(`/products/${id}`, {
      method: "PUT",
      body: JSON.stringify(product),
      headers: {
        "Content-Type": "application/json",
      },
    });
  }

  static async deleteProduct(id: number) {
    return apiFetch<Product>(`/products/${id}`, {
      method: "DELETE",
    });
  }
}
