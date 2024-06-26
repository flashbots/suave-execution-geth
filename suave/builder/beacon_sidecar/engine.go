package beacon_sidecar

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

type BeaconSidecar struct {
	latestBeaconBuildBlockArgs BeaconBuildBlockArgs
	latestTimestamp            uint64
	mu                         sync.Mutex
	cancel                     context.CancelFunc
	wg                         sync.WaitGroup
}

func NewBeaconSidecar(beaconRpc string, boostRelayUrl string) *BeaconSidecar {
	log.Info("Starting BeaconSidecar", "beaconRpc", beaconRpc, "boostRelayUrl", boostRelayUrl)
	ctx, cancel := context.WithCancel(context.Background())
	sidecar := &BeaconSidecar{
		cancel: cancel,
	}
	go sidecar.startSyncing(ctx, beaconRpc, boostRelayUrl)
	return sidecar
}

func (bs *BeaconSidecar) GetLatestBeaconBuildBlockArgs() BeaconBuildBlockArgs {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.latestBeaconBuildBlockArgs.Copy()
}

func (bs *BeaconSidecar) GetLatestTimestamp() uint64 {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.latestTimestamp
}

func (bs *BeaconSidecar) Stop() {
	bs.cancel()
	bs.wg.Wait()
}

func (bs *BeaconSidecar) startSyncing(ctx context.Context, beaconRpc string, boostRelayUrl string) {
	defer bs.wg.Done()
	defer bs.cancel()

	bs.wg.Add(1)
	payloadAttrC := make(chan PayloadAttributesEvent)
	go SubscribeToPayloadAttributesEvents(ctx, beaconRpc, payloadAttrC)

	for paEvent := range payloadAttrC {
		log.Debug("New PA event", "data", paEvent.Data)

		validatorData, err := getValidatorForSlot(ctx, boostRelayUrl, paEvent.Data.ProposalSlot)
		if err != nil {
			log.Warn("could not get validator", "slot", paEvent.Data.ProposalSlot, "err", err)
			continue
		}
		res, err := json.Marshal(validatorData)
		if err != nil {
			log.Error("could not marshal validator data", "err", err)
			continue
		}
		log.Debug("New validator data", "data", string(res))

		beaconBuildBlockArgs := NewBeaconBuildBlockArgsFromValAndPAEvent(validatorData, paEvent)
		bs.update(beaconBuildBlockArgs)
	}
}

func (bs *BeaconSidecar) update(beaconBuildBlockArgs BeaconBuildBlockArgs) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.latestBeaconBuildBlockArgs = beaconBuildBlockArgs
	bs.latestTimestamp = uint64(time.Now().Unix())
}
