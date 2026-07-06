package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wneessen/go-mail"
)

type MailService struct {
	log *slog.Logger
	client *mail.Client
	from string
}

func NewMailService(
	log *slog.Logger,
	host string,
	port int,
	user string,
	password string,
	from string,
) (*MailService, error) {
	client, err := mail.NewClient(
		host,
		mail.WithPort(port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(user),
		mail.WithPassword(password),
		mail.WithTLSPolicy(mail.TLSMandatory),
	)

	if err != nil {
		return nil, err
	}

	return &MailService{
		log: log,
		client: client,
		from: from,
	}, nil
}

func (s *MailService) SendActivationMail(ctx context.Context, to string, link string) error {
	msg := mail.NewMsg()

	if err := msg.From(s.from); err != nil {
		return err
	}

	if err := msg.To(to); err != nil {
		return err
	}

	msg.Subject("Активация аккаунта")

	msg.SetBodyString(
		mail.TypeTextHTML,
		fmt.Sprintf(`
		<div>
			<h1>Для активации перейдите по ссылке</h1>
			<a href="%s">%s</a>
		</div>
		`, link, link),
	)

	s.log.Info("sending activation email", "to", to)

	if err := s.client.DialAndSendWithContext(ctx, msg); err != nil {
		s.log.Error(
			"failed to send activation email",
			"to", to,
			"error", err,
		)
		return err
	}

	s.log.Info(
		"activation email sent",
		"to", to,
	)

	return nil
}