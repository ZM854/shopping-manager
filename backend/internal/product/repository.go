package product

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrProductNotFound = errors.New("Product not found")

type Repository struct {
	db *pgxpool.Pool
}

func (r *Repository) GetProduct(ctx context.Context, id int64) (Product, error) {
	const query = `
		SELECT id, name, quantity, isMarked, unit
		FROM products
		WHERE id = $1
	`
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
		return Product{}, err
	}

	return product, nil
}

func (r *Repository) GetProducts(ctx context.Context) ([]Product, error) {
	const query = `
		SELECT id, name, quantity, isMarked, unit
		FROM products
		ORDER BY id
	`
	rows, err := r.db.Query(ctx, query)

	if err != nil {
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
			return nil, err
		}
		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

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

	newProduct := Product{
		Name: product.Name,
		Quantity: product.Quantity,
		IsMarked: product.IsMarked,
		Unit: product.Unit,
	}
	err := r.db.QueryRow(ctx, query, product.Name, product.Quantity, product.IsMarked, product.Unit).Scan(&newProduct.ID)
	
	if err != nil {
		return Product{}, err
	}
	
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
		return Product{}, err
	}

	return Product{}, nil
}

func (r *Repository) DeleteProduct(ctx context.Context, id int64) (error) {
	const query = `
		DELETE FROM products
		WHERE id = $1
	`
	tag, err := r.db.Exec(ctx, query, id)

	if err != nil {
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}

	return nil
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}
