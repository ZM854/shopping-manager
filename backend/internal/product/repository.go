package product

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrProductNotFound = errors.New("product not found")

type ProductRepository struct {
	db  *pgxpool.Pool
	log *slog.Logger
}

func (r *ProductRepository) GetProducts(
	ctx context.Context,
	userID int64,
) ([]Product, error) {
	const query = `
		SELECT
			id,
			name,
			quantity,
			is_marked,
			unit
		FROM products
		WHERE user_id = $1
		ORDER BY id
	`

	start := time.Now()

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		r.log.Error(
			"failed to get products",
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}
	defer rows.Close()

	products := make([]Product, 0)

	for rows.Next() {
		var product Product

		err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Quantity,
			&product.IsMarked,
			&product.Unit,
		)

		if err != nil {
			r.log.Error(
				"failed to scan product row",
				"user_id", userID,
				"error", err,
			)
			return nil, err
		}

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		r.log.Error(
			"rows iteration failed",
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}

	r.log.Debug(
		"get products completed",
		"user_id", userID,
		"count", len(products),
		"duration", time.Since(start),
	)

	return products, nil
}

func (r *ProductRepository) GetProduct(
	ctx context.Context,
	userID int64,
	productID int64,
) (Product, error) {
	const query = `
		SELECT
			id,
			name,
			quantity,
			is_marked,
			unit
		FROM products
		WHERE
			id = $1
			AND user_id = $2
	`

	start := time.Now()

	var product Product

	err := r.db.QueryRow(
		ctx,
		query,
		productID,
		userID,
	).Scan(
		&product.ID,
		&product.Name,
		&product.Quantity,
		&product.IsMarked,
		&product.Unit,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrProductNotFound
	}

	if err != nil {
		r.log.Error(
			"failed to get product",
			"user_id", userID,
			"product_id", productID,
			"error", err,
		)
		return Product{}, err
	}

	r.log.Debug(
		"get product completed",
		"user_id", userID,
		"product_id", productID,
		"duration", time.Since(start),
	)

	return product, nil
}

func (r *ProductRepository) CreateProduct(
	ctx context.Context,
	userID int64,
	req CreateProductRequest,
) (Product, error) {
	const query = `
		INSERT INTO products (
			user_id,
			name,
			quantity,
			is_marked,
			unit
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	start := time.Now()

	product := Product{
		UserID:   userID,
		Name:     req.Name,
		Quantity: req.Quantity,
		IsMarked: req.IsMarked,
		Unit:     req.Unit,
	}

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
		req.Name,
		req.Quantity,
		req.IsMarked,
		req.Unit,
	).Scan(&product.ID)

	if err != nil {
		r.log.Error(
			"failed to create product",
			"user_id", userID,
			"product_name", req.Name,
			"error", err,
		)
		return Product{}, err
	}

	r.log.Debug(
		"product created",
		"user_id", userID,
		"product_id", product.ID,
		"duration", time.Since(start),
	)

	return product, nil
}

func (r *ProductRepository) UpdateProduct(
	ctx context.Context,
	userID int64,
	productID int64,
	req UpdateProductRequest,
) (Product, error) {
	const query = `
		UPDATE products
		SET
			name = $3,
			quantity = $4,
			is_marked = $5,
			unit = $6
		WHERE
			id = $1
			AND user_id = $2
		RETURNING
			id,
			name,
			quantity,
			is_marked,
			unit
	`

	start := time.Now()

	var product Product

	err := r.db.QueryRow(
		ctx,
		query,
		productID,
		userID,
		req.Name,
		req.Quantity,
		req.IsMarked,
		req.Unit,
	).Scan(
		&product.ID,
		&product.Name,
		&product.Quantity,
		&product.IsMarked,
		&product.Unit,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrProductNotFound
	}

	if err != nil {
		r.log.Error(
			"failed to update product",
			"user_id", userID,
			"product_id", productID,
			"error", err,
		)
		return Product{}, err
	}

	r.log.Debug(
		"product updated",
		"user_id", userID,
		"product_id", productID,
		"duration", time.Since(start),
	)

	return product, nil
}

func (r *ProductRepository) DeleteProduct(
	ctx context.Context,
	userID int64,
	productID int64,
) error {
	const query = `
		DELETE FROM products
		WHERE
			id = $1
			AND user_id = $2
	`

	start := time.Now()

	tag, err := r.db.Exec(
		ctx,
		query,
		productID,
		userID,
	)

	if err != nil {
		r.log.Error(
			"failed to delete product",
			"user_id", userID,
			"product_id", productID,
			"error", err,
		)
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	r.log.Debug(
		"product deleted",
		"user_id", userID,
		"product_id", productID,
		"duration", time.Since(start),
	)

	return nil
}

func NewProductRepository(
	db *pgxpool.Pool,
	log *slog.Logger,
) *ProductRepository {
	return &ProductRepository{
		db: db,
		log: log.With(
			"component", "repository",
			"entity", "product",
		),
	}
}