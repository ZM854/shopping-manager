package product

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type requestKey struct{}
type productStub struct {
	err        error
	calls      int
	userID, id int64
	ctx        context.Context
	create     CreateProductRequest
	update     UpdateProductRequest
}

func (s *productStub) record(ctx context.Context, user, id int64) {
	s.calls++
	s.ctx, s.userID, s.id = ctx, user, id
}
func (s *productStub) GetProducts(ctx context.Context, user int64) ([]Product, error) {
	s.record(ctx, user, 0)
	return []Product{}, s.err
}
func (s *productStub) GetProduct(ctx context.Context, user, id int64) (Product, error) {
	s.record(ctx, user, id)
	return Product{ID: id, UserID: user, Name: "Молоко", Quantity: 1.5, Unit: "л"}, s.err
}
func (s *productStub) CreateProduct(ctx context.Context, user int64, req CreateProductRequest) (Product, error) {
	s.record(ctx, user, 0)
	s.create = req
	return Product{ID: 7, UserID: user, Name: req.Name, Quantity: req.Quantity, Unit: req.Unit}, s.err
}
func (s *productStub) UpdateProduct(ctx context.Context, user, id int64, req UpdateProductRequest) (Product, error) {
	s.record(ctx, user, id)
	s.update = req
	return Product{ID: id, UserID: user, Name: req.Name, Quantity: req.Quantity, Unit: req.Unit, IsMarked: req.IsMarked}, s.err
}
func (s *productStub) DeleteProduct(ctx context.Context, user, id int64) error {
	s.record(ctx, user, id)
	return s.err
}
func (s *productStub) DeleteAllProducts(ctx context.Context, user int64) error {
	s.record(ctx, user, 0)
	return s.err
}

func TestProductHTTPAndService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	operations := []struct {
		method, path   string
		handler        func(*ProductHandler) gin.HandlerFunc
		success        int
		hasID, hasBody bool
	}{
		{"GET", "/products", func(h *ProductHandler) gin.HandlerFunc { return h.GetProducts }, 200, false, false},
		{"GET", "/products/:id", func(h *ProductHandler) gin.HandlerFunc { return h.GetProduct }, 200, true, false},
		{"POST", "/products", func(h *ProductHandler) gin.HandlerFunc { return h.CreateProduct }, 201, false, true},
		{"PUT", "/products/:id", func(h *ProductHandler) gin.HandlerFunc { return h.UpdateProduct }, 200, true, true},
		{"DELETE", "/products/:id", func(h *ProductHandler) gin.HandlerFunc { return h.DeleteProduct }, 204, true, false},
		{"DELETE", "/products", func(h *ProductHandler) gin.HandlerFunc { return h.DeleteAllProducts }, 204, false, false},
	}
	for _, op := range operations {
		t.Run(op.method+op.path, func(t *testing.T) {
			for _, scenario := range []string{"success", "missing_auth", "wrong_auth_type", "failure", "not_found", "bad_id", "bad_body"} {
				if scenario == "bad_id" && !op.hasID || scenario == "bad_body" && !op.hasBody {
					continue
				}
				t.Run(scenario, func(t *testing.T) {
					stub := &productStub{}
					status := op.success
					switch scenario {
					case "missing_auth", "wrong_auth_type":
						status = 401
					case "failure":
						stub.err = errors.New("private database details")
						status = 500
					case "not_found":
						stub.err = fmt.Errorf("wrapped: %w", ErrProductNotFound)
						status = 404
						if op.method == "POST" || op.method == "GET" && !op.hasID {
							status = 500
						}
					case "bad_id", "bad_body":
						status = 400
					}
					h := NewProductHandler(NewProductService(stub, log), log)
					r := gin.New()
					r.Use(func(c *gin.Context) {
						if scenario != "missing_auth" {
							if scenario == "wrong_auth_type" {
								c.Set("userID", "42")
							} else {
								c.Set("userID", int64(42))
							}
						}
					})
					r.Handle(op.method, op.path, op.handler(h))
					path := strings.ReplaceAll(op.path, ":id", "7")
					if scenario == "bad_id" {
						path = strings.ReplaceAll(op.path, ":id", "bad")
					}
					body := `{"name":"Молоко","quantity":1.5,"unit":"л","isMarked":true,"user_id":99}`
					if scenario == "bad_body" {
						body = `{"quantity":"wrong"}`
					}
					req := httptest.NewRequest(op.method, path, strings.NewReader(body))
					req = req.WithContext(context.WithValue(req.Context(), requestKey{}, "request-context"))
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					if w.Code != status {
						t.Fatalf("status %d, want %d: %s", w.Code, status, w.Body.String())
					}
					if status == 400 || status == 401 {
						if stub.calls != 0 {
							t.Fatal("Invalid request reached repository")
						}
						return
					}
					if stub.calls != 1 || stub.userID != 42 || stub.ctx.Value(requestKey{}) != "request-context" {
						t.Fatal("Authorization or context was not preserved")
					}
					if op.hasID && stub.id != 7 {
						t.Fatal("Wrong product ID")
					}
					if scenario == "success" && op.hasBody {
						if op.method == "POST" && stub.create.Name != "Молоко" || op.method == "PUT" && (!stub.update.IsMarked || stub.update.Quantity != 1.5) {
							t.Fatal("Body was lost")
						}
					}
					if strings.Contains(w.Body.String(), "private database details") || strings.Contains(w.Body.String(), "user_id") {
						t.Fatal("Internal fields leaked")
					}
					if status == 204 && w.Body.Len() != 0 {
						t.Fatal("204 must have no body")
					}
					if scenario == "success" && op.method == "GET" && !op.hasID && strings.TrimSpace(w.Body.String()) != "[]" {
						t.Fatal("Empty collection must be []")
					}
				})
			}
		})
	}
}
