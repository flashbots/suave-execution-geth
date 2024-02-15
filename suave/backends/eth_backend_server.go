package backends

import (
	"context"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	suave "github.com/ethereum/go-ethereum/suave/core"
)

// EthBackend is the set of functions exposed from the SUAVE-enabled node
type EthBackend interface {
	BuildEthBlock(ctx context.Context, buildArgs *types.BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error)
	BuildEthBlockFromBundles(ctx context.Context, buildArgs *types.BuildBlockArgs, bundles []types.SBundle) (*engine.ExecutionPayloadEnvelope, error)
	Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error)
}

var _ EthBackend = &EthBackendServer{}

// EthBackendServerBackend is the interface implemented by the SUAVE-enabled node
// to resolve the EthBackend server queries
type EthBackendServerBackend interface {
	CurrentHeader() *types.Header
	BuildEthBlockFromBundles(ctx context.Context, buildArgs *suave.BuildBlockArgs, bundles []types.SBundle) (*engine.ExecutionPayloadEnvelope, error)
	BuildEthBlockFromTxs(ctx context.Context, buildArgs *suave.BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error)
	Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error)
}

type EthBackendServer struct {
	b EthBackendServerBackend
}

func NewEthBackendServer(b EthBackendServerBackend) *EthBackendServer {
	return &EthBackendServer{b}
}

func (e *EthBackendServer) BuildEthBlock(ctx context.Context, buildArgs *types.BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error) {
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

	return e.b.BuildEthBlockFromTxs(ctx, buildArgs, txs)
}

func (e *EthBackendServer) BuildEthBlockFromBundles(ctx context.Context, buildArgs *types.BuildBlockArgs, bundles []types.SBundle) (*engine.ExecutionPayloadEnvelope, error) {
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

	return e.b.BuildEthBlockFromBundles(ctx, buildArgs, bundles)
}

func (e *EthBackendServer) Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error) {
	return e.b.Call(ctx, contractAddr, input)
}
