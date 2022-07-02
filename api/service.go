package api

import (
	"errors"

	"github.com/textileio/go-mail"
)

type Service struct {
	lib *mail.Mail
}

// var _ pb.APIServiceServer = (*Service)(nil)

func (s *Service) SetupMailbox() {

}
