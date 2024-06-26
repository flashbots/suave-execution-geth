package api

import (
	"context"
	"math/big"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
)

var _ API = (*Server)(nil)

// SessionManager is the backend that manages the session state of the builder API.
type SessionManager interface {
	NewSession(context.Context, *BuildBlockArgs) (string, error)
	AddTransaction(sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error)
	AddTransactions(sessionId string, txs types.Transactions) ([]*SimulateTransactionResult, error)
	AddBundles(sessionId string, bundles []*Bundle) ([]*SimulateBundleResult, error)
	BuildBlock(sessionId string) error
	Bid(sessionId string, blsPubKey phase0.BLSPubKey) (*SubmitBlockRequest, error)
	GetBalance(sessionId string, addr common.Address) (*big.Int, error)
	Call(sessionId string, transactionArgs *ethapi.TransactionArgs) ([]byte, error)
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

func (s *Server) NewSession(ctx context.Context, args *BuildBlockArgs) (string, error) {
	return s.sessionMngr.NewSession(ctx, args)
}

func (s *Server) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error) {
	return s.sessionMngr.AddTransaction(sessionId, tx)
}

func (s *Server) AddTransactions(ctx context.Context, sessionId string, txs types.Transactions) ([]*SimulateTransactionResult, error) {
	return s.sessionMngr.AddTransactions(sessionId, txs)
}

func (s *Server) AddBundles(ctx context.Context, sessionId string, bundles []*Bundle) ([]*SimulateBundleResult, error) {
	return s.sessionMngr.AddBundles(sessionId, bundles)
}

func (s *Server) BuildBlock(ctx context.Context, sessionId string) error {
	return s.sessionMngr.BuildBlock(sessionId)
}

func (s *Server) Bid(ctx context.Context, sessionId string, blsPubKey phase0.BLSPubKey) (*SubmitBlockRequest, error) {
	return s.sessionMngr.Bid(sessionId, blsPubKey)
}

func (s *Server) GetBalance(ctx context.Context, sessionId string, addr common.Address) (*big.Int, error) {
	return s.sessionMngr.GetBalance(sessionId, addr)
}

func (s *Server) Call(ctx context.Context, sessionId string, transactionArgs *ethapi.TransactionArgs) (hexutil.Bytes, error) {
	res, err := s.sessionMngr.Call(sessionId, transactionArgs)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(res), nil
}

// TODO: Remove
type MockServer struct {
}

func (s *MockServer) NewSession(ctx context.Context, args *BuildBlockArgs) (string, error) {
	return "", nil
}

func (s *MockServer) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error) {
	return &SimulateTransactionResult{}, nil
}

func (s *MockServer) BuildBlock(ctx context.Context) error {
	return nil
}

func (s *MockServer) GetBalance(ctx context.Context, sessionId string, addr common.Address) (*big.Int, error) {
	return big.NewInt(0), nil
}
