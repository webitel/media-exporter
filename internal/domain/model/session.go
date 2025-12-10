package model

import "github.com/webitel/media-exporter/internal/errors"

type Session struct {
	userID int64
	domain int64
	token  string
}

func NewSession(userID, domain int64, token string) (*Session, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}
	if domain == 0 {
		return nil, errors.New("domainID is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}
	return &Session{userID: userID, domain: domain, token: token}, nil
}

func (s *Session) UserID() int64   { return s.userID }
func (s *Session) DomainID() int64 { return s.domain }
func (s *Session) Token() string   { return s.token }
