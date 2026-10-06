package testdb

import (
	"strings"
	"testing"
)

func TestParseConfigRejectsUnsafeDatabase(t *testing.T) {
	for _, url := range []string{
		"",
		"host=localhost dbname=shopping_manager_test user=shopping_manager_test",
		"postgres://postgres:private-secret@localhost/products_db",
		"postgres://shopping_manager_test:private-secret@localhost/products_db",
		"postgres://postgres:private-secret@localhost/shopping_manager_test",
		"postgres://shopping_manager_test:private-secret@localhost/%zz",
	} {
		cfg, err := ParseConfig(url)
		if err == nil || cfg != nil {
			t.Errorf("Небезопасное подключение принято")
		}
		if err != nil && strings.Contains(err.Error(), "private-secret") {
			t.Error("Ошибка раскрывает пароль")
		}
	}
}

func TestParseConfigAcceptsDedicatedDatabase(t *testing.T) {
	for _, scheme := range []string{"postgres", "postgresql"} {
		cfg, err := ParseConfig(scheme + "://shopping_manager_test:local-test-only@localhost:55432/shopping_manager_test?sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConnConfig.Database != DatabaseName || cfg.ConnConfig.User != UserName || cfg.ConnConfig.ConnectTimeout == 0 {
			t.Fatal("Некорректная тестовая конфигурация")
		}
	}
}
