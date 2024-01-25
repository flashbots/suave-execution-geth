package suave

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/suave/sdk"
)

var AllowedPeekerAny = common.HexToAddress("0xC8df3686b4Afb2BB53e60EAe97EF043FE03Fb829") // "*"

type Bytes = hexutil.Bytes
type DataId = types.DataId

type DataRecord struct {
	Id                  types.DataId
	Salt                types.DataId
	DecryptionCondition uint64
	AllowedPeekers      []common.Address
	AllowedStores       []common.Address
	Version             string
	CreationTx          *types.Transaction
	Signature           []byte
}

func (b *DataRecord) ToInnerRecord() types.DataRecord {
	return types.DataRecord{
		Id:                  b.Id,
		Salt:                b.Salt,
		DecryptionCondition: b.DecryptionCondition,
		AllowedPeekers:      b.AllowedPeekers,
		AllowedStores:       b.AllowedStores,
		Version:             b.Version,
	}
}

type MEVMBid = types.DataRecord

type BuildBlockArgs = types.BuildBlockArgs

type ConfidentialEthBackend = sdk.API
