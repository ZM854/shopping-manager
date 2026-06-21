package product

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrProductNotFound = errors.New("Product not found")

type Repository struct {
	db *pgxpool.Pool
	log *slog.Logger
}

func (r *Repository) GetProduct(ctx context.Context, id int64) (Product, error) {
	const query = `
		SELECT id, name, quantity, isMarked, unit
		FROM products
		WHERE id = $1
	`
	
	start := time.Now()

	var product Product

	err := r.db.QueryRow(ctx, query, id).Scan(
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
		r.log.Error("failed to get product", "product_id", id, "error", err)
		return Product{}, err
	}

	r.log.Debug("get product completed", "id", id, "duration", time.Since(start))

	return product, nil
}

func (r *Repository) GetProducts(ctx context.Context) ([]Product, error) {
	const query = `
		SELECT id, name, quantity, isMarked, unit
		FROM products
		ORDER BY id
	`
	start := time.Now()

	rows, err := r.db.Query(ctx, query)

	if err != nil {
		r.log.Error("failed to get products", "error", err)
		return nil, err
	}
	defer rows.Close()

	products := make([]Product, 0)

	for rows.Next() {
		var product Product

		err:= rows.Scan(
			&product.ID,
			&product.Name,
			&product.Quantity,
			&product.IsMarked,
			&product.Unit,
		)

		if err != nil {
			r.log.Error("failed to scan product row", "error", err)
			return nil, err
		}
		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		r.log.Error("rows iteration failed")
		return nil, err
	}

	r.log.Debug("get products completed", "count", len(products), "duration", time.Since(start))

	return products, nil
}

func (r *Repository) PostProduct(ctx context.Context, product CreateProductRequest) (Product, error) {
	const query = `
		INSERT INTO products (
			name,
			quantity,
			isMarked,
			unit
		)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	start := time.Now()

	newProduct := Product{
		Name: product.Name,
		Quantity: product.Quantity,
		IsMarked: product.IsMarked,
		Unit: product.Unit,
	}

	err := r.db.QueryRow(ctx, query, product.Name, product.Quantity, product.IsMarked, product.Unit).Scan(&newProduct.ID)

	if err != nil {
		r.log.Error("failed to create product", "product_name",product.Name, "error", err)
		return Product{}, err
	}

	r.log.Debug("product created", "product_id", newProduct.ID, "duration", time.Since(start) )
	
	return newProduct, nil
}

func (r *Repository) UpdateProduct(ctx context.Context, id int64, updatedProduct UpdateProductRequest) (Product, error) {
	const query = `
		UPDATE products
		SET
			name = $2,
			quantity = $3,
			isMarked = $4,
			unit = $5
		WHERE id = $1
		RETURNING id, name, quantity, isMarked, unit
	`

	start := time.Now()

	var product Product

	err := r.db.QueryRow(
		ctx, 
		query, 
		id, 
		updatedProduct.Name, 
		updatedProduct.Quantity, 
		updatedProduct.IsMarked, 
		updatedProduct.Unit,
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
		r.log.Error("failed to update product", "product_id", id, "error", err)
		return Product{}, err
	}

	r.log.Debug("product updated", "product_id", id, "duration", time.Since(start))

	return Product{}, nil
}

func (r *Repository) DeleteProduct(ctx context.Context, id int64) error {
	const query = `
		DELETE FROM products
		WHERE id = $1
	`

	start := time.Now()

	tag, err := r.db.Exec(ctx, query, id)

	if err != nil {
		r.log.Error("failed to delete product", "product_id", id, "error", err)
		return err
	}


	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	r.log.Debug("product deleted", "product_id", id, "duration", time.Since(start) )

	return nil
}

func NewRepository(db *pgxpool.Pool, log *slog.Logger) *Repository {
	return &Repository{
		db: db,
		log: log.With("component", "repository"),
	}
}
