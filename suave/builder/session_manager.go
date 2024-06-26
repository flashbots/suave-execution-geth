package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/attestantio/go-eth2-client/spec/phase0"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/suave/builder/api"
	bs "github.com/ethereum/go-ethereum/suave/builder/beacon_sidecar"
	"github.com/google/uuid"
)

type Config struct {
	GasCeil               uint64
	SessionIdleTimeout    time.Duration
	MaxConcurrentSessions int
}

type SessionManager struct {
	sem            chan struct{}
	sessions       map[string]*miner.Builder
	sessionTimers  map[string]*time.Timer
	sessionsLock   sync.RWMutex
	blockchain     *core.BlockChain
	pool           *txpool.TxPool
	config         *Config
	beacon_sidecar *bs.BeaconSidecar
}

func NewSessionManager(blockchain *core.BlockChain, pool *txpool.TxPool, config *Config, bs *bs.BeaconSidecar) *SessionManager {
	if config.GasCeil == 0 {
		config.GasCeil = 1000000000000000000
	}
	if config.SessionIdleTimeout == 0 {
		config.SessionIdleTimeout = 5 * time.Second
	}
	if config.MaxConcurrentSessions <= 0 {
		config.MaxConcurrentSessions = 16 // chosen arbitrarily
	}

	sem := make(chan struct{}, config.MaxConcurrentSessions)
	for len(sem) < cap(sem) {
		sem <- struct{}{} // fill 'er up
	}

	s := &SessionManager{
		sem:            sem,
		sessions:       make(map[string]*miner.Builder),
		sessionTimers:  make(map[string]*time.Timer),
		blockchain:     blockchain,
		config:         config,
		pool:           pool,
		beacon_sidecar: bs,
	}
	return s
}

func (s *SessionManager) BlockChain() *core.BlockChain {
	return s.blockchain
}

func (s *SessionManager) TxPool() *txpool.TxPool {
	return s.pool
}

func (s *SessionManager) newBuilder(args *api.BuildBlockArgs) (*miner.Builder, error) {
	builderCfg := &miner.BuilderConfig{
		ChainConfig: s.blockchain.Config(),
		Engine:      s.blockchain.Engine(),
		Chain:       s.blockchain,
		EthBackend:  s,
		GasCeil:     s.config.GasCeil,
	}
	args = s.builderArgsFromBeaconSidecar(args)

	builderArgs := &miner.BuilderArgs{
		ParentHash:      args.Parent,
		FeeRecipient:    args.FeeRecipient,
		ProposerPubkey:  args.ProposerPubkey,
		Extra:           args.Extra,
		Slot:            args.Slot,
		Timestamp:       args.Timestamp,
		GasLimit:        args.GasLimit,
		Random:          args.Random,
		Withdrawals:     args.Withdrawals,
		ParentBlockRoot: args.BeaconRoot,
	}
	jsonArgs, _ := json.Marshal(builderArgs)
	log.Info("Creating new builder session", "args", string(jsonArgs))

	session, err := miner.NewBuilder(builderCfg, builderArgs)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// todo: avoid unnecessary and inefficient copying of args among four different BuilderArgs structs
func (s *SessionManager) builderArgsFromBeaconSidecar(overrides *api.BuildBlockArgs) *api.BuildBlockArgs {
	if s.beacon_sidecar == nil {
		return overrides
	}

	args := s.beacon_sidecar.GetLatestBeaconBuildBlockArgs()
	if overrides == nil {
		args := args.ToBuildBlockArgs()
		return &args
	}

	if overrides.Slot == 0 {
		overrides.Slot = args.Slot
	}

	if len(overrides.ProposerPubkey) == 0 {
		overrides.ProposerPubkey = args.ProposerPubkey
	}
	if overrides.Parent == (common.Hash{}) {
		overrides.Parent = args.Parent
	}
	if overrides.Timestamp == 0 {
		overrides.Timestamp = args.Timestamp
	}
	if overrides.FeeRecipient == (common.Address{}) {
		overrides.FeeRecipient = args.FeeRecipient
	}
	if overrides.GasLimit == 0 {
		overrides.GasLimit = args.GasLimit
	}
	if overrides.Random == (common.Hash{}) {
		overrides.Random = args.Random
	}
	if len(overrides.Withdrawals) == 0 {
		overrides.Withdrawals = args.Withdrawals
	}
	if overrides.BeaconRoot == (common.Hash{}) {
		overrides.BeaconRoot = args.ParentBlockRoot
	}

	return overrides
}

// NewSession creates a new builder session and returns the session id
func (s *SessionManager) NewSession(ctx context.Context, args *api.BuildBlockArgs) (string, error) {
	if args == nil {
		return "", fmt.Errorf("args cannot be nil")
	}
	// Wait for session to become available
	select {
	case <-s.sem:
		s.sessionsLock.Lock()
		defer s.sessionsLock.Unlock()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	session, err := s.newBuilder(args)
	if err != nil {
		return "", err
	}

	id := uuid.New().String()[:7]
	s.sessions[id] = session

	// start session timer
	s.sessionTimers[id] = time.AfterFunc(s.config.SessionIdleTimeout, func() {
		s.sessionsLock.Lock()
		defer s.sessionsLock.Unlock()

		delete(s.sessions, id)
		delete(s.sessionTimers, id)
	})

	// Technically, we are certain that there is an open slot in the semaphore
	// channel, but let's be defensive and panic if the invariant is violated.
	select {
	case s.sem <- struct{}{}:
	default:
		panic("released more sessions than are open") // unreachable
	}

	return id, nil
}

func (s *SessionManager) getSession(sessionId string, allowOnTheFlySession bool) (*miner.Builder, error) {
	if sessionId == "" && allowOnTheFlySession {
		return s.newBuilder(&api.BuildBlockArgs{})
	}

	s.sessionsLock.RLock()
	defer s.sessionsLock.RUnlock()

	session, ok := s.sessions[sessionId]
	if !ok {
		return nil, fmt.Errorf("session %s not found", sessionId)
	}

	// reset session timer
	s.sessionTimers[sessionId].Reset(s.config.SessionIdleTimeout)

	return session, nil
}

func (s *SessionManager) AddTransaction(sessionId string, tx *types.Transaction) (*api.SimulateTransactionResult, error) {
	builder, err := s.getSession(sessionId, true)
	if err != nil {
		return nil, err
	}
	return builder.AddTransaction(tx)
}

func (s *SessionManager) AddTransactions(sessionId string, txs types.Transactions) ([]*api.SimulateTransactionResult, error) {
	builder, err := s.getSession(sessionId, true)
	if err != nil {
		return nil, err
	}
	return builder.AddTransactions(txs)
}

func (s *SessionManager) AddBundles(sessionId string, bundles []*api.Bundle) ([]*api.SimulateBundleResult, error) {
	builder, err := s.getSession(sessionId, true)
	if err != nil {
		return nil, err
	}
	return builder.AddBundles(bundles)
}

func (s *SessionManager) BuildBlock(sessionId string) error {
	builder, err := s.getSession(sessionId, false)
	if err != nil {
		return err
	}
	_, err = builder.BuildBlock() // TODO: Return more info
	return err
}

func (s *SessionManager) Bid(sessionId string, blsPubKey phase0.BLSPubKey) (*api.SubmitBlockRequest, error) {
	builder, err := s.getSession(sessionId, false)
	if err != nil {
		return nil, err
	}
	return builder.Bid(blsPubKey)
}

func (s *SessionManager) GetBalance(sessionId string, addr common.Address) (*big.Int, error) {
	builder, err := s.getSession(sessionId, false)
	if err != nil {
		return nil, err
	}
	return builder.GetBalance(addr), nil
}

// CalcBaseFee calculates the basefee of the header.
func CalcBaseFee(config *params.ChainConfig, parent *types.Header) *big.Int {
	// If the current block is the first EIP-1559 block, return the InitialBaseFee.
	if !config.IsLondon(parent.Number) {
		return new(big.Int).SetUint64(params.InitialBaseFee)
	}

	parentGasTarget := parent.GasLimit / config.ElasticityMultiplier()
	// If the parent gasUsed is the same as the target, the baseFee remains unchanged.
	if parent.GasUsed == parentGasTarget {
		return new(big.Int).Set(parent.BaseFee)
	}

	var (
		num   = new(big.Int)
		denom = new(big.Int)
	)

	if parent.GasUsed > parentGasTarget {
		// If the parent block used more gas than its target, the baseFee should increase.
		// max(1, parentBaseFee * gasUsedDelta / parentGasTarget / baseFeeChangeDenominator)
		num.SetUint64(parent.GasUsed - parentGasTarget)
		num.Mul(num, parent.BaseFee)
		num.Div(num, denom.SetUint64(parentGasTarget))
		num.Div(num, denom.SetUint64(config.BaseFeeChangeDenominator()))
		baseFeeDelta := math.BigMax(num, common.Big1)

		return num.Add(parent.BaseFee, baseFeeDelta)
	} else {
		// Otherwise if the parent block used less gas than its target, the baseFee should decrease.
		// max(0, parentBaseFee * gasUsedDelta / parentGasTarget / baseFeeChangeDenominator)
		num.SetUint64(parentGasTarget - parent.GasUsed)
		num.Mul(num, parent.BaseFee)
		num.Div(num, denom.SetUint64(parentGasTarget))
		num.Div(num, denom.SetUint64(config.BaseFeeChangeDenominator()))
		baseFee := num.Sub(parent.BaseFee, num)

		return math.BigMax(baseFee, common.Big0)
	}
}

func (s *SessionManager) Call(sessionId string, tx_args *ethapi.TransactionArgs) ([]byte, error) {
	builder, err := s.getSession(sessionId, false)
	if err != nil {
		return nil, err
	}
	result, err := builder.Call(tx_args)

	return result, err
}
