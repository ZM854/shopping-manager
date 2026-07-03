package auth

import "log/slog"

type MailService struct {
	log *slog.Logger
}

func NewMailService(log *slog.Logger) *MailService {
	return &MailService{
		log: log,
	}
}

func (s *MailService) SendActivationMail(to string, link string) {

}