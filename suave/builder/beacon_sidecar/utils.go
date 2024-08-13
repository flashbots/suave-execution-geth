// Partially based on https://github.com/flashbots/suave-geth/blob/892e2e11ba2735cdbc7d7ef694b8942dadaf0bdd/suave/cmd/suavecli/boost_utils.go

package beacon_sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/rs/zerolog"

	eth2client "github.com/attestantio/go-eth2-client"
	eth2apiv1 "github.com/attestantio/go-eth2-client/api/v1"
	eth2http "github.com/attestantio/go-eth2-client/http"
)

func subscribeToPayloadAttributesEvents(
	ctx context.Context,
	endpoint string,
	payloadAttrC chan eth2apiv1.PayloadAttributesEvent,
) error {
	provider, err := newEth2EventsProvider(ctx, endpoint)
	if err != nil {
		return err
	}
	log.Debug("Subscribing to payload_attributes events")
	return provider.Events(ctx, []string{"payload_attributes"}, func(event *eth2apiv1.Event) {
		if data, ok := event.Data.(*eth2apiv1.PayloadAttributesEvent); ok {
			payloadAttrC <- *data
		} else {
			log.Error("Unexpected data type", "type", fmt.Sprintf("%T", event.Data))
		}
	})
}

func getValidatorForSlot(ctx context.Context, relayUrl string, nextSlot uint64) (ValidatorData, error) {
	endpoint := relayUrl + "/relay/v1/builder/validators"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ValidatorData{}, fmt.Errorf("could not prepare request: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return ValidatorData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return ValidatorData{}, errors.New("nothing returned from the boost relay")
	}

	if resp.StatusCode > 299 {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return ValidatorData{}, fmt.Errorf("could not read error response body for status code %d: %w", resp.StatusCode, err)
		}
		return ValidatorData{}, fmt.Errorf("http error: %d / %s", resp.StatusCode, string(bodyBytes))
	}

	var dst GetValidatorRelayResponse
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ValidatorData{}, fmt.Errorf("could not read response body: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &dst); err != nil {
		return ValidatorData{}, fmt.Errorf("could not unmarshal response %s: %w", string(bodyBytes), err)
	}

	res := make(map[uint64]ValidatorData)
	for _, data := range dst {
		feeRecipient := common.HexToAddress(data.Entry.Message.FeeRecipient)
		pubkeyHex := strings.ToLower(data.Entry.Message.Pubkey)

		res[data.Slot] = ValidatorData{
			Pubkey:       pubkeyHex,
			FeeRecipient: feeRecipient,
			GasLimit:     data.Entry.Message.GasLimit,
		}
	}
	v, found := res[nextSlot]
	if !found {
		return ValidatorData{}, errors.New("validator not found")
	}

	return v, nil
}

func newEth2EventsProvider(ctx context.Context, endpoint string) (eth2client.EventsProvider, error) {
	client, err := newEth2HttpClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if provider, isProvider := client.(eth2client.EventsProvider); isProvider {
		return provider, nil
	}
	return nil, errors.New("client does not support event subscriptions")
}

func newEth2HttpClient(ctx context.Context, endpoint string) (eth2client.Service, error) {
	client, err := eth2http.New(ctx,
		eth2http.WithAddress(endpoint),
		eth2http.WithLogLevel(zerolog.WarnLevel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	return client, nil
}
