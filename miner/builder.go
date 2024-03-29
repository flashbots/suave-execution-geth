package miner

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	denebBuilder "github.com/attestantio/go-builder-client/api/deneb"
	builderV1 "github.com/attestantio/go-builder-client/api/v1"
	"github.com/attestantio/go-eth2-client/spec/bellatrix"
	"github.com/attestantio/go-eth2-client/spec/capella"
	"github.com/attestantio/go-eth2-client/spec/deneb"
	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	suavextypes "github.com/ethereum/go-ethereum/suave/builder/api"
	"github.com/flashbots/go-boost-utils/ssz"
	"github.com/holiman/uint256"
)

var (
	ErrInvalidInclusionRange = errors.New("invalid inclusion range")
	ErrInvalidBlockNumber    = errors.New("invalid block number")
	ErrExceedsMaxBlock       = errors.New("block number exceeds max block")
	ErrEmptyTxs              = errors.New("empty transactions")
	ErrInvalidRefundPercent  = errors.New("refund percent should be between 0 and 99 inclusive")
)

type BuilderConfig struct {
	ChainConfig *params.ChainConfig
	Engine      consensus.Engine
	EthBackend  Backend
	Chain       *core.BlockChain
	GasCeil     uint64
}

type BuilderArgs struct {
	ParentHash     common.Hash
	FeeRecipient   common.Address
	ProposerPubkey []byte
	Extra          []byte
	Slot           uint64
}

type Builder struct {
	env              *environment
	wrk              *worker
	args             *BuilderArgs
	block            *types.Block
	ephemeralPrivKey *ecdsa.PrivateKey
	ephemeralAddr    common.Address
}

func NewBuilder(config *BuilderConfig, args *BuilderArgs) (*Builder, error) {
	b := &Builder{
		args: args,
	}

	b.wrk = &worker{
		config: &Config{
			GasCeil: config.GasCeil,
		},
		eth:         config.EthBackend,
		chainConfig: config.ChainConfig,
		engine:      config.Engine,
		chain:       config.Chain,
	}

	workerParams := &generateParams{
		parentHash: args.ParentHash,
		forceTime:  false,
		coinbase:   args.FeeRecipient,
		extra:      args.Extra,
	}
	env, err := b.wrk.prepareWork(workerParams)
	if err != nil {
		return nil, err
	}

	env.gasPool = new(core.GasPool).AddGas(env.header.GasLimit)
	b.env = env
	b.ephemeralPrivKey, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	b.ephemeralAddr = crypto.PubkeyToAddress(b.ephemeralPrivKey.PublicKey)

	return b, nil
}

func (b *Builder) addTransaction(txn *types.Transaction, env *environment) (*suavextypes.SimulateTransactionResult, error) {
	// If the context is not set, the logs will not be recorded
	b.env.state.SetTxContext(txn.Hash(), b.env.tcount)

	prevGas := env.header.GasUsed
	logs, err := b.wrk.commitTransaction(env, txn)
	if err != nil {
		return &suavextypes.SimulateTransactionResult{
			Error:   err.Error(),
			Success: false,
		}, err
	}
	egp := env.header.GasUsed - prevGas
	return receiptToSimResult(&types.Receipt{Logs: logs}, egp), nil
}

func (b *Builder) AddTransaction(txn *types.Transaction) (*suavextypes.SimulateTransactionResult, error) {
	res, _ := b.addTransaction(txn, b.env)
	return res, nil
}

func (b *Builder) AddTransactions(txns types.Transactions) ([]*suavextypes.SimulateTransactionResult, error) {
	results := make([]*suavextypes.SimulateTransactionResult, 0)
	snap := b.env.copy()

	for _, txn := range txns {
		res, err := b.addTransaction(txn, snap)
		results = append(results, res)
		if err != nil {
			return results, nil
		}
	}
	b.env = snap
	return results, nil
}

func (b *Builder) addBundle(bundle *suavextypes.Bundle, env *environment) (*suavextypes.SimulateBundleResult, error) {
	if err := checkBundleParams(b.env.header.Number, bundle); err != nil {
		return &suavextypes.SimulateBundleResult{
			Error:   err.Error(),
			Success: false,
		}, err
	}

	revertingHashes := bundle.RevertingHashesMap()
	egp := uint64(0)
	feeRecipient := env.coinbase

	if bundle.HasRefund() {
		// set coinbase to the ephemeral address to collect bundle profit
		env.coinbase = b.ephemeralAddr
	}

	var results []*suavextypes.SimulateTransactionResult

	profitPreBundle := env.state.GetBalance(env.coinbase)
	for _, txn := range bundle.Txs {
		result, err := b.addTransaction(txn, env)
		results = append(results, result)
		if err != nil {
			if _, ok := revertingHashes[txn.Hash()]; ok {
				// continue if the transaction is in the reverting hashes
				continue
			}
			return makeBundleSimResult(results, egp, err), err
		}
		egp += result.Egp
	}
	profitPostBundle := env.state.GetBalance(env.coinbase)
	// reset coinbase to the original fee recipient
	env.coinbase = feeRecipient

	if bundle.HasRefund() {
		tx, err := b.getRefundTx(bundle, env, profitPreBundle, profitPostBundle, feeRecipient)
		if err != nil {
			return makeBundleSimResult(results, egp, err), err
		}

		_, err = b.addTransaction(tx, env)
		if err != nil {
			return makeBundleSimResult(results, egp, err), err
		}
	}

	return makeBundleSimResult(results, egp, nil), nil
}

func (b *Builder) getRefundTx(bundle *suavextypes.Bundle, env *environment, profitPreBundle *big.Int, profitPostBundle *big.Int, feeRecipient common.Address) (*types.Transaction, error) {
	if !(len(bundle.Txs) > 1 && bundle.RefundPercent != nil) {
		return nil, errors.New("refund is not possible with the given bundle")
	}

	refundTransferCost := new(big.Int).Mul(big.NewInt(28000), env.header.BaseFee)
	refundPrct := *bundle.RefundPercent
	if refundPrct == 0 {
		refundPrct = 10
	}

	bundleProfit := new(big.Int).Sub(profitPostBundle, profitPreBundle)
	refundAmt := new(big.Int).Mul(bundleProfit, big.NewInt(int64(refundPrct)))
	refundAmt = new(big.Int).Div(refundAmt, big.NewInt(100))
	refundAmt = new(big.Int).Sub(refundAmt, refundTransferCost)
	userTx := bundle.Txs[0]
	refundAddr, err := types.Sender(types.LatestSignerForChainID(userTx.ChainId()), userTx)
	if err != nil {
		return nil, err
	}

	currNonce := env.state.GetNonce(b.ephemeralAddr)
	paymentTx, err := types.SignTx(types.NewTx(&types.LegacyTx{
		Nonce:    currNonce,
		To:       &refundAddr,
		Value:    refundAmt,
		Gas:      28000,
		GasPrice: env.header.BaseFee,
	}), env.signer, b.ephemeralPrivKey)

	if err != nil {
		return nil, err
	}

	return paymentTx, nil
}

func (b *Builder) AddBundles(bundles []*suavextypes.Bundle) ([]*suavextypes.SimulateBundleResult, error) {
	var results []*suavextypes.SimulateBundleResult
	snap := b.env.copy()

	for _, bundle := range bundles {
		result, err := b.addBundle(bundle, snap)
		results = append(results, result)
		if err != nil {
			return results, nil
		}
	}

	b.env = snap
	return results, nil
}

func (b *Builder) GetBalance(addr common.Address) *big.Int {
	return b.env.state.GetBalance(addr)
}

func (b *Builder) FillPending() error {
	if err := b.wrk.commitPendingTxs(b.env); err != nil {
		return err
	}
	return nil
}

func (b *Builder) BuildBlock() (*types.Block, error) {
	work := b.env

	// check if ephemeral address has profit and transfer it to the fee recipient
	refundTransferCost := new(big.Int).Mul(big.NewInt(28000), work.header.BaseFee)
	profit := work.state.GetBalance(b.ephemeralAddr)
	profit = new(big.Int).Sub(profit, refundTransferCost)
	if profit.Cmp(big.NewInt(0)) > 0 {
		currNonce := work.state.GetNonce(b.ephemeralAddr)
		paymentTx, err := types.SignTx(types.NewTx(&types.LegacyTx{
			Nonce:    currNonce,
			To:       &work.coinbase,
			Value:    profit,
			Gas:      28000,
			GasPrice: work.header.BaseFee,
		}), work.signer, b.ephemeralPrivKey)

		if err != nil {
			return nil, err
		}
		_, err = b.wrk.commitTransaction(work, paymentTx)
		if err != nil {
			return nil, err
		}
	}

	block, err := b.wrk.engine.FinalizeAndAssemble(b.wrk.chain, work.header, work.state, work.txs, nil, work.receipts, nil)
	if err != nil {
		return nil, err
	}
	b.block = block
	return block, nil
}

func (b *Builder) Bid(builderPubKey phase0.BLSPubKey) (*suavextypes.SubmitBlockRequest, error) {
	work := b.env

	if b.block == nil {
		return nil, fmt.Errorf("block not built")
	}

	envelope := engine.BlockToExecutableData(b.block, totalFees(b.block, work.receipts), work.sidecars)
	payload, err := executableDataToDenebExecutionPayload(envelope.ExecutionPayload)
	if err != nil {
		return nil, err
	}

	value, overflow := uint256.FromBig(envelope.BlockValue)
	if overflow {
		return nil, fmt.Errorf("block value %v overflows", *envelope.BlockValue)
	}
	var proposerPubkey [48]byte
	copy(proposerPubkey[:], b.args.ProposerPubkey)

	blockBidMsg := builderV1.BidTrace{
		Slot:                 b.args.Slot,
		ParentHash:           payload.ParentHash,
		BlockHash:            payload.BlockHash,
		BuilderPubkey:        builderPubKey,
		ProposerPubkey:       phase0.BLSPubKey(proposerPubkey),
		ProposerFeeRecipient: bellatrix.ExecutionAddress(b.args.FeeRecipient),
		GasLimit:             envelope.ExecutionPayload.GasLimit,
		GasUsed:              envelope.ExecutionPayload.GasUsed,
		Value:                value,
	}

	genesisForkVersion := phase0.Version{0x00, 0x00, 0x10, 0x20}
	builderSigningDomain := ssz.ComputeDomain(ssz.DomainTypeAppBuilder, genesisForkVersion, phase0.Root{})

	root, err := ssz.ComputeSigningRoot(&blockBidMsg, builderSigningDomain)
	if err != nil {
		return nil, err
	}

	bidRequest := suavextypes.SubmitBlockRequest{
		Root: phase0.Root(root),
		SubmitBlockRequest: denebBuilder.SubmitBlockRequest{
			Message:          &blockBidMsg,
			ExecutionPayload: payload,
			Signature:        phase0.BLSSignature{},
			BlobsBundle:      &denebBuilder.BlobsBundle{},
		},
	}
	return &bidRequest, nil
}

func receiptToSimResult(receipt *types.Receipt, egp uint64) *suavextypes.SimulateTransactionResult {
	result := &suavextypes.SimulateTransactionResult{
		Egp:     egp,
		Success: true,
		Logs:    []*suavextypes.SimulatedLog{},
	}
	for _, log := range receipt.Logs {
		result.Logs = append(result.Logs, &suavextypes.SimulatedLog{
			Addr:   log.Address,
			Topics: log.Topics,
			Data:   log.Data,
		})
	}
	return result
}

func makeBundleSimResult(txSimResults []*suavextypes.SimulateTransactionResult, egp uint64, err error) *suavextypes.SimulateBundleResult {
	if err == nil {
		return &suavextypes.SimulateBundleResult{
			Egp:                        egp,
			SimulateTransactionResults: txSimResults,
			Success:                    true,
		}
	}

	return &suavextypes.SimulateBundleResult{
		Error:                      err.Error(),
		SimulateTransactionResults: txSimResults,
		Success:                    false,
	}
}

func executableDataToDenebExecutionPayload(data *engine.ExecutableData) (*deneb.ExecutionPayload, error) {
	transactionData := make([]bellatrix.Transaction, len(data.Transactions))
	for i, tx := range data.Transactions {
		transactionData[i] = bellatrix.Transaction(tx)
	}

	withdrawalData := make([]*capella.Withdrawal, len(data.Withdrawals))
	for i, wd := range data.Withdrawals {
		withdrawalData[i] = &capella.Withdrawal{
			Index:          capella.WithdrawalIndex(wd.Index),
			ValidatorIndex: phase0.ValidatorIndex(wd.Validator),
			Address:        bellatrix.ExecutionAddress(wd.Address),
			Amount:         phase0.Gwei(wd.Amount),
		}
	}

	baseFeePerGas := new(uint256.Int)
	if baseFeePerGas.SetFromBig(data.BaseFeePerGas) {
		return nil, errors.New("base fee per gas: overflow")
	}

	return &deneb.ExecutionPayload{
		ParentHash:    [32]byte(data.ParentHash),
		FeeRecipient:  [20]byte(data.FeeRecipient),
		StateRoot:     [32]byte(data.StateRoot),
		ReceiptsRoot:  [32]byte(data.ReceiptsRoot),
		LogsBloom:     types.BytesToBloom(data.LogsBloom),
		PrevRandao:    [32]byte(data.Random),
		BlockNumber:   data.Number,
		GasLimit:      data.GasLimit,
		GasUsed:       data.GasUsed,
		Timestamp:     data.Timestamp,
		ExtraData:     data.ExtraData,
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     [32]byte(data.BlockHash),
		Transactions:  transactionData,
		Withdrawals:   withdrawalData,
	}, nil
}

func checkBundleParams(currentBlockNumber *big.Int, bundle *suavextypes.Bundle) error {
	if bundle.BlockNumber != nil && bundle.MaxBlock != nil && bundle.BlockNumber.Cmp(bundle.MaxBlock) > 0 {
		return ErrInvalidInclusionRange
	}

	// check inclusion target if BlockNumber is set
	if bundle.BlockNumber != nil {
		if bundle.MaxBlock == nil && currentBlockNumber.Cmp(bundle.BlockNumber) != 0 {
			return ErrInvalidBlockNumber
		}

		if bundle.MaxBlock != nil {
			if currentBlockNumber.Cmp(bundle.MaxBlock) > 0 {
				return ErrExceedsMaxBlock
			}

			if currentBlockNumber.Cmp(bundle.BlockNumber) < 0 {
				return ErrInvalidBlockNumber
			}
		}
	}

	// check if the bundle has transactions
	if bundle.Txs == nil || bundle.Txs.Len() == 0 {
		return ErrEmptyTxs
	}

	// check if refund percent is valid
	// https: //github.com/flashbots/mev-share/blob/main/specs/bundles/refund-recipient.md#refundpercent
	if bundle.RefundPercent != nil && (*bundle.RefundPercent < 0 || *bundle.RefundPercent > 99) {
		return ErrInvalidRefundPercent
	}

	return nil
}
