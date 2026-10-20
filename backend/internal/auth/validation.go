package auth

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid authentication fields" }

func validateEmail(email *string) string {
	*email = strings.TrimSpace(*email)
	if len(*email) == 0 || len(*email) > 254 {
		return "Введите email длиной до 254 символов"
	}
	for _, char := range *email {
		if char < 33 || char > 126 {
			return "Введите корректный email латинскими символами"
		}
	}
	*email = strings.ToLower(*email)
	address, err := mail.ParseAddress(*email)
	parts := strings.Split(*email, "@")
	if err != nil || address.Address != *email || len(parts) != 2 ||
		len(parts[0]) > 64 || !strings.Contains(parts[1], ".") {
		return "Некорректный email"
	}
	return ""
}

func validatePassword(password string, registration bool) string {
	if password == "" {
		return "Введите пароль"
	}
	if !utf8.ValidString(password) || len(password) > 72 {
		return "Пароль должен занимать не более 72 байт UTF-8"
	}
	if registration && utf8.RuneCountInString(password) < 6 {
		return "Минимум 6 символов"
	}
	return ""
}

func validateAuthFields(req any) error {
	fields := map[string]string{}
	var email *string
	switch value := req.(type) {
	case *RegistrationRequest:
		value.Name = strings.TrimSpace(value.Name)
		length := utf8.RuneCountInString(value.Name)
		if !utf8.ValidString(value.Name) || length < 2 || length > 20 {
			fields["name"] = "Имя должно содержать от 2 до 20 символов"
		}
		if strings.ContainsFunc(value.Name, unicode.IsControl) {
			fields["name"] = "Управляющие символы в имени запрещены"
		}
		fields["password"] = validatePassword(value.Password, true)
		email = &value.Email
	case *LoginRequest:
		fields["password"] = validatePassword(value.Password, false)
		email = &value.Email
	case *ResendActivationRequest:
		email = &value.Email
	}
	if email != nil {
		fields["email"] = validateEmail(email)
	}
	for field, message := range fields {
		if message == "" {
			delete(fields, field)
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}
