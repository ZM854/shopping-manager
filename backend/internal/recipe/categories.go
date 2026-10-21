package recipe

import "slices"

type Category struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var categories = [...]Category{
	{Code: "soups", Name: "Супы"},
	{Code: "salads", Name: "Салаты"},
	{Code: "starters", Name: "Закуски"},
	{Code: "main-dishes", Name: "Основные блюда"},
	{Code: "side-dishes", Name: "Гарниры"},
	{Code: "baking", Name: "Выпечка"},
	{Code: "desserts", Name: "Десерты"},
	{Code: "sauces", Name: "Соусы"},
	{Code: "drinks", Name: "Напитки"},
	{Code: "other", Name: "Другое"},
}

func Categories() []Category {
	return slices.Clone(categories[:])
}

func validCategory(code string) bool {
	for _, category := range categories {
		if category.Code == code {
			return true
		}
	}
	return false
}
