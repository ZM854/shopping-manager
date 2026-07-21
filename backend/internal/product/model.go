package product

type Product struct {
	ID       int64   `json:"id"`
	UserID   int64   `json:"-"`
	Name     string  `json:"name"`
	IsMarked bool    `json:"isMarked"`
	Quantity float32 `json:"quantity"`
	Unit     string  `json:"unit"`
}

type CreateProductRequest struct {
	Name     string  `json:"name"`
	IsMarked bool    `json:"isMarked"`
	Quantity float32 `json:"quantity"`
	Unit     string  `json:"unit"`
}

type UpdateProductRequest struct {
	Name     string  `json:"name"`
	IsMarked bool    `json:"isMarked"`
	Quantity float32 `json:"quantity"`
	Unit     string  `json:"unit"`
}
