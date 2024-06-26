// Based on https://github.com/flashbots/suave-geth/blob/892e2e11ba2735cdbc7d7ef694b8942dadaf0bdd/suave/cmd/suavecli/boost_utils.go

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
	"github.com/r3labs/sse"
)

func SubscribeToPayloadAttributesEvents(ctx context.Context, endpoint string, payloadAttrC chan<- PayloadAttributesEvent) {
	retries := 100 // todo: make this configurable
	eventsURL := fmt.Sprintf("%s/eth/v1/events?topics=payload_attributes", endpoint)

	client := sse.NewClient(eventsURL)
	for i := 0; i < retries; i++ {
		select {
		case <-ctx.Done():
			log.Info("Stopping subscription to payload_attributes events")
			return
		default:
			err := client.SubscribeRawWithContext(ctx, func(msg *sse.Event) {
				var payloadAttributesResp PayloadAttributesEvent
				if len(msg.Data) == 0 {
					log.Warn("Empty payload_attributes event")
					return
				}
				err := json.Unmarshal(msg.Data, &payloadAttributesResp)
				if err != nil {
					log.Error("Could not unmarshal payload_attributes event", "err", err)
					return
				}

				select {
				case payloadAttrC <- payloadAttributesResp:
				case <-ctx.Done():
					log.Info("Context completed during event processing")
					return
				}
			})

			if err != nil {
				log.Error("Failed to subscribe to payload_attributes events", "err", err)
				if ctx.Err() != nil {
					log.Info("Context error detected, stopping retries")
					return
				}
				// Wait before retrying to avoid hammering the server on immediate reconnects
				time.Sleep(1 * time.Second)
				continue
			}
		}

		log.Warn("SubscribeRawWithContext ended unexpectedly, reconnecting")
	}
	log.Error("Failed to subscribe to payload_attributes events after retries")
}

func getValidatorForSlot(ctx context.Context, relayUrl string, nextSlot uint64) (ValidatorData, error) {
	endpoint := relayUrl + "/relay/v1/builder/validators"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ValidatorData{}, fmt.Errorf("could not prepare request: %w", err)
	}

	// Execute request
	resp, err := http.DefaultClient.Do(req)
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
