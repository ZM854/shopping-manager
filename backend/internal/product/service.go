package product

import (
	"context"
	"log/slog"
)

type ProductRepositoryInterface interface {
	GetProducts(ctx context.Context, userID int64) ([]Product, error)

	GetProduct(
		ctx context.Context,
		userID int64,
		productID int64,
	) (Product, error)

	CreateProduct(
		ctx context.Context,
		userID int64,
		req CreateProductRequest,
	) (Product, error)

	UpdateProduct(
		ctx context.Context,
		userID int64,
		productID int64,
		req UpdateProductRequest,
	) (Product, error)

	DeleteProduct(
		ctx context.Context,
		userID int64,
		productID int64,
	) error
}

type ProductService struct {
	repository ProductRepositoryInterface
	log        *slog.Logger
}

func NewProductService(
	repository ProductRepositoryInterface,
	log *slog.Logger,
) *ProductService {
	return &ProductService{
		repository: repository,
		log: log.With(
			"component", "service",
			"entity", "product",
		),
	}
}

func (s *ProductService) GetProducts(
	ctx context.Context,
	userID int64,
) ([]Product, error) {
	return s.repository.GetProducts(ctx, userID)
}

func (s *ProductService) GetProduct(
	ctx context.Context,
	userID int64,
	productID int64,
) (Product, error) {
	return s.repository.GetProduct(ctx, userID, productID)
}

func (s *ProductService) CreateProduct(
	ctx context.Context,
	userID int64,
	req CreateProductRequest,
) (Product, error) {
	return s.repository.CreateProduct(ctx, userID, req)
}

func (s *ProductService) UpdateProduct(
	ctx context.Context,
	userID int64,
	productID int64,
	req UpdateProductRequest,
) (Product, error) {
	return s.repository.UpdateProduct(ctx, userID, productID, req)
}

func (s *ProductService) DeleteProduct(
	ctx context.Context,
	userID int64,
	productID int64,
) error {
	return s.repository.DeleteProduct(ctx, userID, productID)
}