package sdk

import (
	"context"

	"github.com/ethereum/go-ethereum/core/types"
)

// SessionManager is the backend that manages the session state of the builder API.
type SessionManager interface {
	NewSession() (string, error)
	AddTransaction(sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error)
}

func NewServer(s SessionManager) *Server {
	api := &Server{
		sessionMngr: s,
	}
	return api
}

type Server struct {
	sessionMngr SessionManager
}

func (s *Server) NewSession(ctx context.Context) (string, error) {
	return s.sessionMngr.NewSession()
}

func (s *Server) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error) {
	return s.sessionMngr.AddTransaction(sessionId, tx)
}

type MockServer struct {
}

func (s *MockServer) NewSession(ctx context.Context) (string, error) {
	return "", nil
}

func (s *MockServer) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error) {
	return &SimulateTransactionResult{}, nil
}
