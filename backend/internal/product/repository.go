package product

type Repository struct {
	products []Product
}

func (r *Repository) GetProduct(id int64) (Product, bool) {
	for _, product := range r.products {
		if product.ID == id {
			return product, true
		}
	}
	return Product{}, false
}

func (r *Repository) GetProducts() []Product {
	return r.products
}

func (r *Repository) PostProduct(product CreateProductRequest) Product {
	newProduct := Product{
		ID: r.products[len(r.products)-1].ID + 1,
		Name: product.Name,
		Quantity: product.Quantity,
		IsMarked: product.IsMarked,
		Unit: product.Unit,
	}
	r.products = append(r.products, newProduct)
	return newProduct
}

func (r *Repository) UpdateProduct(id int64, updatedProduct UpdateProductRequest) (Product, bool) {
	for i, product := range r.products {
		if product.ID == id {
			r.products[i].IsMarked = updatedProduct.IsMarked
			r.products[i].Name = updatedProduct.Name
			r.products[i].Unit = updatedProduct.Unit
			r.products[i].Quantity = updatedProduct.Quantity
			return r.products[i], true
		}
	}
	return Product{}, false
}

func (r *Repository) DeleteProduct(id int64) (bool) {
	for i, product := range r.products {
		if product.ID == id {
			r.products = append(r.products[:i], r.products[i+1:]...)
			return true
		}
	}
	return false
}

func NewRepository() *Repository {
	return &Repository{
		products: []Product{
			{
				ID:       1,
				Name:     "Молоко",
				IsMarked: false,
				Quantity: 1,
				Unit:     "л",
			},
			{
				ID:       2,
				Name:     "Хлеб",
				IsMarked: false,
				Quantity: 1,
				Unit:     "шт",
			},
			{
				ID:       3,
				Name:     "Яйца",
				IsMarked: false,
				Quantity: 10,
				Unit:     "шт",
			},
			{
				ID:       4,
				Name:     "Яблоки",
				IsMarked: false,
				Quantity: 1.5,
				Unit:     "кг",
			},
			{
				ID:       5,
				Name:     "Сыр",
				IsMarked: false,
				Quantity: 300,
				Unit:     "г",
			},
			{
				ID:       6,
				Name:     "Куриное филе",
				IsMarked: false,
				Quantity: 500,
				Unit:     "г",
			},
			{
				ID:       7,
				Name:     "Картофель",
				IsMarked: false,
				Quantity: 2,
				Unit:     "кг",
			},
			{
				ID:       8,
				Name:     "Лук",
				IsMarked: false,
				Quantity: 3,
				Unit:     "шт",
			},
			{
				ID:       9,
				Name:     "Масло сливочное",
				IsMarked: false,
				Quantity: 180,
				Unit:     "г",
			},
			{
				ID:       10,
				Name:     "Гречка",
				IsMarked: false,
				Quantity: 800,
				Unit:     "г",
			},
		},
	}
}
