package sdk

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type API interface {
	// session builder
	NewSession(ctx context.Context) (string, error)
	AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error)

	// block builder precompiles
	BuildEthBlock(ctx context.Context, args *BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error)
	BuildEthBlockFromBundles(ctx context.Context, args *BuildBlockArgs, bundles []SBundle) (*engine.ExecutionPayloadEnvelope, error)
	Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error)
}

type SimulateTransactionResult struct {
	Egp     uint64
	Logs    []*SimulatedLog
	Success bool
	Error   string
}

type SimulatedLog struct {
	Data   []byte
	Addr   common.Address
	Topics []common.Hash
}

type BuildBlockArgs struct {
	Slot           uint64
	ProposerPubkey []byte
	Parent         common.Hash
	Timestamp      uint64
	FeeRecipient   common.Address
	GasLimit       uint64
	Random         common.Hash
	Withdrawals    []*types.Withdrawal
	Extra          []byte
	FillPending    bool
}

type SBundle struct {
	BlockNumber     *big.Int           `json:"blockNumber,omitempty"` // if BlockNumber is set it must match DecryptionCondition!
	MaxBlock        *big.Int           `json:"maxBlock,omitempty"`
	Txs             types.Transactions `json:"txs"`
	RevertingHashes []common.Hash      `json:"revertingHashes,omitempty"`
	RefundPercent   *int               `json:"percent,omitempty"`
}
