package beacon_sidecar

import (
	"encoding/json"

	eth2apiv1 "github.com/attestantio/go-eth2-client/api/v1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/suave/builder/api"
)

type BeaconBuildBlockArgs struct {
	Slot            uint64
	ProposerPubkey  []byte
	Parent          common.Hash
	Timestamp       uint64
	FeeRecipient    common.Address
	GasLimit        uint64
	Random          common.Hash
	Withdrawals     []*types.Withdrawal
	ParentBlockRoot common.Hash
}

func NewBeaconBuildBlockArgsFromValAndPAEvent(valData ValidatorData, paEvent eth2apiv1.PayloadAttributesEvent) BeaconBuildBlockArgs {
	beaconBuildBlockArgs := BeaconBuildBlockArgs{
		Slot:            uint64(paEvent.Data.ProposalSlot),
		ProposerPubkey:  hexutil.MustDecode(valData.Pubkey),
		Parent:          common.Hash(paEvent.Data.ParentBlockHash),
		Timestamp:       paEvent.Data.V3.Timestamp,
		Random:          paEvent.Data.V3.PrevRandao,
		FeeRecipient:    valData.FeeRecipient,
		GasLimit:        valData.GasLimit,
		ParentBlockRoot: common.Hash(paEvent.Data.V3.ParentBeaconBlockRoot),
	}

	for _, w := range paEvent.Data.V3.Withdrawals {
		withdrawal := types.Withdrawal{
			Index:     uint64(w.Index),
			Validator: uint64(w.ValidatorIndex),
			Address:   common.Address(w.Address),
			Amount:    uint64(w.Amount),
		}
		beaconBuildBlockArgs.Withdrawals = append(beaconBuildBlockArgs.Withdrawals, &withdrawal)
	}
	return beaconBuildBlockArgs
}

func (b *BeaconBuildBlockArgs) Copy() BeaconBuildBlockArgs {
	deepCopy := BeaconBuildBlockArgs{
		Slot:            b.Slot,
		ProposerPubkey:  make([]byte, len(b.ProposerPubkey)),
		Parent:          b.Parent,
		Timestamp:       b.Timestamp,
		Random:          b.Random,
		FeeRecipient:    b.FeeRecipient,
		GasLimit:        b.GasLimit,
		ParentBlockRoot: b.ParentBlockRoot,
		Withdrawals:     make([]*types.Withdrawal, len(b.Withdrawals)),
	}

	copy(deepCopy.ProposerPubkey, b.ProposerPubkey)
	for i, w := range b.Withdrawals {
		deepCopy.Withdrawals[i] = &types.Withdrawal{
			Index:     w.Index,
			Validator: w.Validator,
			Address:   w.Address,
			Amount:    w.Amount,
		}
	}

	return deepCopy
}

func (b *BeaconBuildBlockArgs) ToBuildBlockArgs() api.BuildBlockArgs {
	return api.BuildBlockArgs{
		Slot:           b.Slot,
		ProposerPubkey: b.ProposerPubkey,
		Parent:         b.Parent,
		Timestamp:      b.Timestamp,
		FeeRecipient:   b.FeeRecipient,
		GasLimit:       b.GasLimit,
		Random:         b.Random,
		Withdrawals:    b.Withdrawals,
		BeaconRoot:     b.ParentBlockRoot,
	}

}

func (b BeaconBuildBlockArgs) Bytes() ([]byte, error) {
	return json.Marshal(b)
}

type ValidatorData struct {
	Pubkey       string
	FeeRecipient common.Address
	GasLimit     uint64
}

type GetValidatorRelayResponse []struct {
	Slot  uint64 `json:"slot,string"`
	Entry struct {
		Message struct {
			FeeRecipient string `json:"fee_recipient"`
			GasLimit     uint64 `json:"gas_limit,string"`
			Timestamp    uint64 `json:"timestamp,string"`
			Pubkey       string `json:"pubkey"`
		} `json:"message"`
		Signature string `json:"signature"`
	} `json:"entry"`
}
