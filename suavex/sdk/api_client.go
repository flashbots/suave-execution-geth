package sdk

import (
	"context"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

var _ API = (*APIClient)(nil)

type APIClient struct {
	rpc rpcClient
}

func NewClient(endpoint string) (*APIClient, error) {
	clt, err := rpc.Dial(endpoint)
	if err != nil {
		return nil, err
	}
	return NewClientFromRPC(clt), nil
}

type rpcClient interface {
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
}

func NewClientFromRPC(rpc rpcClient) *APIClient {
	return &APIClient{rpc: rpc}
}

func (a *APIClient) NewSession(ctx context.Context) (string, error) {
	var id string
	err := a.rpc.CallContext(ctx, &id, "suavex_newSession")
	return id, err
}

func (a *APIClient) AddTransaction(ctx context.Context, sessionId string, tx *types.Transaction) (*SimulateTransactionResult, error) {
	var receipt *SimulateTransactionResult
	err := a.rpc.CallContext(ctx, &receipt, "suavex_addTransaction", sessionId, tx)
	return receipt, err
}

func (a *APIClient) BuildEthBlock(ctx context.Context, args *BuildBlockArgs, txs types.Transactions) (*engine.ExecutionPayloadEnvelope, error) {
	var result engine.ExecutionPayloadEnvelope
	err := a.rpc.CallContext(ctx, &result, "suavex_buildEthBlock", args, txs)
	return &result, err
}

func (a *APIClient) BuildEthBlockFromBundles(ctx context.Context, args *BuildBlockArgs, bundles []SBundle) (*engine.ExecutionPayloadEnvelope, error) {
	var result engine.ExecutionPayloadEnvelope
	err := a.rpc.CallContext(ctx, &result, "suavex_buildEthBlockFromBundles", args, bundles)
	return &result, err
}

func (a *APIClient) Call(ctx context.Context, contractAddr common.Address, input []byte) ([]byte, error) {
	var result []byte
	err := a.rpc.CallContext(ctx, &result, "suavex_call", contractAddr, input)
	return result, err
}
