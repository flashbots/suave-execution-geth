package builder

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	suave "github.com/ethereum/go-ethereum/suave/core"
	"github.com/ethereum/go-ethereum/suave/sdk"
)

// SuavexBackend is the interface required by Suavex endpoint
type SuavexBackend interface {
	core.ChainContext

	CurrentHeader() *types.Header
	BuildBlockFromTxs(ctx context.Context, buildArgs *suave.BuildBlockArgs, txs types.Transactions) (*types.Block, *big.Int, error)
	BuildBlockFromBundles(ctx context.Context, buildArgs *suave.BuildBlockArgs, bundles []types.SBundle) (*types.Block, *big.Int, error)
	Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error)

	// StateAt returns the state at the given root
	StateAt(root common.Hash) (*state.StateDB, error)

	// Config returns the chain config
	Config() *params.ChainConfig
}

// Suavex is the implementation of the Suavex namespace
type Suavex struct {
	b SuavexBackend

	builderSessionManager *SessionManager
}

func NewSuavex(b SuavexBackend) *Suavex {
	return &Suavex{
		b:                     b,
		builderSessionManager: NewSessionManager(b, &Config{}),
	}
}

func (e *Suavex) BuildEthBlock(ctx context.Context, buildArgs *types.BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error) {
	if buildArgs == nil {
		head := e.b.CurrentHeader()
		buildArgs = &types.BuildBlockArgs{
			Parent:       head.Hash(),
			Timestamp:    head.Time + uint64(12),
			FeeRecipient: common.Address{0x42},
			GasLimit:     30000000,
			Random:       head.Root,
			Withdrawals:  nil,
			Extra:        []byte(""),
			FillPending:  false,
		}
	}

	block, profit, err := e.b.BuildBlockFromTxs(ctx, buildArgs, txs)
	if err != nil {
		return nil, err
	}

	// TODO: we're not adding blobs, but this is not where you would do it anyways
	return engine.BlockToExecutableData(block, profit, nil), nil
}

func (e *Suavex) BuildEthBlockFromBundles(ctx context.Context, buildArgs *types.BuildBlockArgs, bundles []types.SBundle) (*engine.ExecutionPayloadEnvelope, error) {
	if buildArgs == nil {
		head := e.b.CurrentHeader()
		buildArgs = &types.BuildBlockArgs{
			Parent:       head.Hash(),
			Timestamp:    head.Time + uint64(12),
			FeeRecipient: common.Address{0x42},
			GasLimit:     30000000,
			Random:       head.Root,
			Withdrawals:  nil,
			Extra:        []byte(""),
			FillPending:  false,
		}
	}

	block, profit, err := e.b.BuildBlockFromBundles(ctx, buildArgs, bundles)
	if err != nil {
		return nil, err
	}

	// TODO: we're not adding blobs, but this is not where you would do it anyways
	return engine.BlockToExecutableData(block, profit, nil), nil
}

func (e *Suavex) Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error) {
	return e.b.Call(ctx, contractAddr, input)
}

func (e *Suavex) NewSession(ctx context.Context) (string, error) {
	return e.builderSessionManager.NewSession()
}

func (e *Suavex) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*sdk.SimulateTransactionResult, error) {
	return e.builderSessionManager.AddTransaction(sessionId, tx)
}
