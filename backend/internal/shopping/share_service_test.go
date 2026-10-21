package shopping

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func unitShareToken(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

type shareTestTransaction struct {
	replayTransaction
	preserve bool
	created  bool
}

func (tx *shareTestTransaction) GetShare(context.Context, int64, int64) (Share, error) {
	return Share{ID: 42}, tx.fail("get-share")
}

func (tx *shareTestTransaction) CreateShare(
	_ context.Context, _, _ int64, _ []byte, preserve bool,
) (Share, error) {
	tx.created = true
	tx.preserve = preserve
	return Share{ID: 42, PreserveMarks: preserve}, tx.fail("create-share")
}

func (tx *shareTestTransaction) CopyListToShare(context.Context, int64, int64, int64) error {
	return tx.fail("copy-source")
}

func (tx *shareTestTransaction) FindShare(context.Context, []byte) (int64, string, error) {
	return 42, "Снимок", tx.fail("find-share")
}

func (tx *shareTestTransaction) CopyShareToList(context.Context, int64, int64, int64) error {
	return tx.fail("copy-share")
}

func TestShareTokenValidationAndHash(t *testing.T) {
	for _, value := range []byte{0, 1, 127, 255} {
		got, err := hashShareToken(unitShareToken(value))
		want := sha256.Sum256(bytes.Repeat([]byte{value}, 32))
		if err != nil || !bytes.Equal(got, want[:]) {
			t.Fatal("Изменились каноническое кодирование или SHA-256 секрета")
		}
	}
	token := unitShareToken(1)
	for _, value := range []string{
		"", token[1:], token + "=", " " + token, token + " ",
		"+" + token[1:], "/" + token[1:], "я" + token[2:],
		token[:42] + "B", token[:42] + "\n",
	} {
		var validation *ValidationError
		if _, err := hashShareToken(value); !errors.As(err, &validation) || validation.Fields["token"] == "" {
			t.Fatal("Неканонический секрет принят или попал в текст ошибки")
		}
	}
}

func TestShareValidationBeforeTransaction(t *testing.T) {
	s := NewService(&forbiddenStore{t: t})
	ctx := context.Background()
	req := CreateShareRequest{Token: unitShareToken(1), PreserveMarks: ptr(false)}
	for _, operation := range []func() error{
		func() error {
			_, err := s.CreateShare(ctx, 0, 1, "share-action-0001", req)
			return err
		},
		func() error {
			_, err := s.CreateShare(ctx, 1, -1, "share-action-0001", req)
			return err
		},
		func() error {
			_, err := s.CreateShare(ctx, 1, 1, "bad", req)
			return err
		},
		func() error {
			_, err := s.CreateShare(ctx, 1, 1, "share-action-0001", CreateShareRequest{Token: req.Token})
			return err
		},
		func() error {
			_, err := s.CreateShare(ctx, 1, 1, "share-action-0001", CreateShareRequest{PreserveMarks: ptr(false)})
			return err
		},
		func() error {
			_, err := s.ImportShare(ctx, -1, "share-action-0001", ImportShareRequest{Token: req.Token})
			return err
		},
		func() error {
			_, err := s.ImportShare(ctx, 1, "bad", ImportShareRequest{Token: req.Token})
			return err
		},
		func() error {
			_, err := s.ImportShare(ctx, 1, "share-action-0001", ImportShareRequest{Token: " " + req.Token})
			return err
		},
	} {
		var validation *ValidationError
		if err := operation(); !errors.As(err, &validation) {
			t.Fatalf("Ожидалась валидация до открытия транзакции: %v", err)
		}
	}
}

func TestShareServicePropagatesFailuresSafely(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("SQL dependency secret must stay private")
	for _, mode := range []string{"create", "import"} {
		stages := []string{"begin", "claim", "lock", "create-share", "copy-source", "save"}
		if mode == "import" {
			stages = []string{"begin", "claim", "find-share", "create", "copy-share", "save"}
		}
		for _, stage := range stages {
			t.Run(mode+"/"+stage, func(t *testing.T) {
				tx := &shareTestTransaction{replayTransaction: replayTransaction{errAt: stage, err: sentinel}}
				store := transactionStore{tx: tx}
				if stage == "begin" {
					store.beginErr = sentinel
				}
				s := NewService(store)
				var id int64
				var err error
				if mode == "create" {
					var result Share
					result, err = s.CreateShare(ctx, 1, 2, "share-action-0001",
						CreateShareRequest{Token: unitShareToken(1), PreserveMarks: ptr(false)})
					id = result.ID
				} else {
					var result List
					result, err = s.ImportShare(ctx, 1, "share-action-0001", ImportShareRequest{Token: unitShareToken(1)})
					id = result.ID
				}
				if !errors.Is(err, sentinel) || strings.Contains(err.Error(), "secret") || id != 0 {
					t.Fatalf("Ошибка потеряна, раскрыта или выдан незафиксированный результат: %v", err)
				}
			})
		}
	}
}

func TestShareServiceReplaysAndDomainErrors(t *testing.T) {
	ctx := context.Background()
	for _, preserve := range []bool{false, true} {
		tx := &shareTestTransaction{}
		req := CreateShareRequest{Token: unitShareToken(1), PreserveMarks: &preserve}
		original, err := NewService(transactionStore{tx: tx}).CreateShare(ctx, 1, 2, "share-action-0001", req)
		if err != nil || !tx.created || !tx.saved || tx.preserve != preserve {
			t.Fatal("Явный выбор отметок не принят", err)
		}
		payload, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		tx = &shareTestTransaction{replayTransaction: replayTransaction{
			replay: Replay{ResourceID: ptr(original.ID), Response: payload},
			errAt:  "lock", err: errors.New("оригинал удалён"),
		}}
		replayed, err := NewService(transactionStore{tx: tx}).CreateShare(ctx, 1, 2, "share-action-0001", req)
		if err != nil || !reflect.DeepEqual(original, replayed) || tx.created || tx.saved {
			t.Fatal("Повтор создания снимка зависит от оригинала или создаёт новый снимок", err)
		}
	}

	for _, importMode := range []bool{false, true} {
		run := func(tx *shareTestTransaction) error {
			s := NewService(transactionStore{tx: tx})
			if importMode {
				_, err := s.ImportShare(ctx, 1, "share-action-0001", ImportShareRequest{Token: unitShareToken(1)})
				return err
			}
			_, err := s.CreateShare(ctx, 1, 2, "share-action-0001",
				CreateShareRequest{Token: unitShareToken(1), PreserveMarks: ptr(false)})
			return err
		}
		tx := &shareTestTransaction{replayTransaction: replayTransaction{replay: Replay{Hash: "другое тело"}}}
		if err := run(tx); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal("Пропущен конфликт ключа", err)
		}
		lookup := "get-share"
		if importMode {
			lookup = "get"
		}
		tx = &shareTestTransaction{replayTransaction: replayTransaction{
			replay: Replay{ResourceID: ptr(int64(42))}, errAt: lookup, err: ErrNotFound,
		}}
		if err := run(tx); !errors.Is(err, ErrResourceDeleted) {
			t.Fatal("Не проверено существование или владение результатом", err)
		}
		tx = &shareTestTransaction{replayTransaction: replayTransaction{
			replay: Replay{ResourceID: ptr(int64(42)), Response: []byte("invalid JSON")},
		}}
		if err := run(tx); err == nil {
			t.Fatal("Повреждённый исходный ответ принят")
		}
	}

	original := List{ID: 42, Name: "Импортированная копия"}
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	tx := &shareTestTransaction{replayTransaction: replayTransaction{
		replay: Replay{ResourceID: ptr(original.ID), Response: payload},
		errAt:  "find-share", err: ErrNotFound,
	}}
	replayed, err := NewService(transactionStore{tx: tx}).ImportShare(
		ctx, 1, "share-action-0001", ImportShareRequest{Token: unitShareToken(1)})
	if err != nil || !reflect.DeepEqual(replayed, original) || tx.saved {
		t.Fatal("Повтор импорта зависит от существования снимка или изменяет результат", err)
	}
}

func TestShareDTOContainsOnlySafeMetadata(t *testing.T) {
	data, err := json.Marshal(Share{ID: 9007199254740993, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "9007199254740993", "createdAt": "2026-01-01T00:00:00Z", "preserveMarks": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("DTO раскрыл лишние поля или изменил строковый BIGINT")
	}
}

func TestShareCancellationKeepsDependencyDetailsPrivate(t *testing.T) {
	cause := errors.Join(errors.New("dependency secret"), context.DeadlineExceeded)
	err := safeShareError("import", cause)
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "secret") {
		t.Fatal("Отмена потеряна или раскрывает текст зависимости", err)
	}
}
