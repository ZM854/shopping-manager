export type Product = {
  id: number;
  name: string;
  isMarked: boolean;
  quantity: number;
  unit: string;
};

export type CreateProductRequest = {
  name: string;
  isMarked: boolean;
  quantity: number;
  unit: string;
};

export type UpdateProductRequest = {
  name: string;
  isMarked: boolean;
  quantity: number;
  unit: string;
};
