package recipe

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

var errInvalidJSON = errors.New("invalid recipe JSON")

func decodeSaveRequest(c *gin.Context) (SaveRequest, error) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, MaxJSONBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return SaveRequest{}, err
		}
		return SaveRequest{}, errInvalidJSON
	}
	if !utf8.Valid(data) {
		return SaveRequest{}, errInvalidJSON
	}
	fields, err := decodeObject(data, "title", "cookingTimeMinutes", "categoryCode", "ingredients", "steps")
	if err != nil {
		return SaveRequest{}, err
	}
	if raw, ok := fields["ingredients"]; ok {
		if err := checkArray(raw, "ingredients", MaxIngredients, "name", "quantity", "unit"); err != nil {
			return SaveRequest{}, err
		}
	}
	if raw, ok := fields["steps"]; ok {
		if err := checkArray(raw, "steps", MaxSteps, "text"); err != nil {
			return SaveRequest{}, err
		}
	}
	var req SaveRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return SaveRequest{}, errInvalidJSON
	}
	return req, nil
}

func decodeObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errInvalidJSON
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errInvalidJSON
		}
		key, ok := token.(string)
		if !ok {
			return nil, errInvalidJSON
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, errInvalidJSON
		}
		known := false
		for _, field := range allowed {
			if key == field {
				known = true
				break
			}
		}
		if !known {
			return nil, errInvalidJSON
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, errInvalidJSON
		}
		if key != "quantity" && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, errInvalidJSON
		}
		fields[key] = raw
	}
	if err := finishJSON(decoder, '}'); err != nil {
		return nil, err
	}
	return fields, nil
}

func checkArray(data []byte, field string, max int, allowed ...string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return errInvalidJSON
	}
	count := 0
	for decoder.More() {
		count++
		if count > max {
			return invalid(field)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return errInvalidJSON
		}
		if _, err := decodeObject(raw, allowed...); err != nil {
			return err
		}
	}
	return finishJSON(decoder, ']')
}

func finishJSON(decoder *json.Decoder, closing json.Delim) error {
	if token, err := decoder.Token(); err != nil || token != closing {
		return errInvalidJSON
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errInvalidJSON
	}
	return nil
}
