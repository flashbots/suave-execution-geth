package backends

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/core/types"
	suave "github.com/ethereum/go-ethereum/suave/core"
)

func TestEthBackend_Compatibility(t *testing.T) {
	// This test ensures that the client is able to call to the server.
	// It does not cover the internal logic implemention of the endpoints.
	srv := rpc.NewServer()
	require.NoError(t, srv.RegisterName("suavex", NewEthBackendServer(&mockBackend{})))

	clt := &RemoteEthBackend{client: rpc.DialInProc(srv)}

	_, err := clt.BuildEthBlock(context.Background(), &types.BuildBlockArgs{}, nil)
	require.NoError(t, err)

	_, err = clt.BuildEthBlockFromBundles(context.Background(), &types.BuildBlockArgs{}, nil)
	require.NoError(t, err)

	_, err = clt.Call(context.Background(), common.Address{}, nil)
	require.NoError(t, err)
}

// mockBackend is a backend for the EthBackendServer that returns mock data
type mockBackend struct{}

func (n *mockBackend) CurrentHeader() *types.Header {
	return &types.Header{}
}

func (n *mockBackend) BuildBlockFromTxs(ctx context.Context, buildArgs *suave.BuildBlockArgs, txs types.Transactions) (*types.Block, *big.Int, error) {
	block := types.NewBlock(&types.Header{GasUsed: 1000, BaseFee: big.NewInt(1)}, txs, nil, nil, trie.NewStackTrie(nil))
	return block, big.NewInt(11000), nil
}

func (n *mockBackend) BuildBlockFromBundles(ctx context.Context, buildArgs *suave.BuildBlockArgs, bundles []types.SBundle) (*types.Block, *big.Int, error) {
	var txs types.Transactions
	for _, bundle := range bundles {
		txs = append(txs, bundle.Txs...)
	}
	block := types.NewBlock(&types.Header{GasUsed: 1000, BaseFee: big.NewInt(1)}, txs, nil, nil, trie.NewStackTrie(nil))
	return block, big.NewInt(11000), nil
}

func (n *mockBackend) BuildEthBlockFromBundles(ctx context.Context, buildArgs *suave.BuildBlockArgs, bundles []types.SBundle) (*engine.ExecutionPayloadEnvelope, error) {
	return getDummyExecutionPayloadEnvelope(), nil
}

func (n *mockBackend) BuildEthBlockFromTxs(ctx context.Context, buildArgs *suave.BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error) {
	return getDummyExecutionPayloadEnvelope(), nil
}

func (n *mockBackend) Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error) {
	return []byte{0x1}, nil
}

func getDummyExecutionPayloadEnvelope() *engine.ExecutionPayloadEnvelope {
	dummyExecutableData := &engine.ExecutableData{
		ParentHash:    common.HexToHash("0x01"),
		FeeRecipient:  common.HexToAddress("0x02"),
		StateRoot:     common.HexToHash("0x03"),
		ReceiptsRoot:  common.HexToHash("0x04"),
		LogsBloom:     []byte("dummyLogsBloom"),
		Random:        common.HexToHash("0x05"),
		Number:        1,
		GasLimit:      10000000,
		GasUsed:       500000,
		Timestamp:     1640995200,
		ExtraData:     []byte("dummyExtraData"),
		BaseFeePerGas: big.NewInt(1000000000),
		BlockHash:     common.HexToHash("0x06"),
		Transactions:  [][]byte{[]byte("dummyTransaction")},
	}
	exePayloadEnv := engine.ExecutionPayloadEnvelope{
		ExecutionPayload: dummyExecutableData,
		BlockValue:       big.NewInt(123456789),
	}
	return &exePayloadEnv
}
