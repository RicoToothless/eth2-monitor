package main

import (
	"context"
	"eth2-monitor/prysmgrpc"
	"eth2-monitor/spec"
	"fmt"

	"github.com/rs/zerolog/log"
)

func ReportSlashing(ctx context.Context, prefix string, reason string, slot spec.Slot, slasher spec.ValidatorIndex, slashee spec.ValidatorIndex) {
	var epoch spec.Epoch = slot / spec.SLOTS_PER_EPOCH
	var balances map[spec.Epoch]spec.Gwei
	var err error

	s, err := prysmgrpc.New(ctx, prysmgrpc.WithAddress(opts.BeaconNode))
	if err != nil {
		log.Error().Err(err).Msg("ReportSlashing failed while reporting a slashing")
		return
	}

	rewardStr := "reward is unknown"
	Measure(func() {
		balances, err = s.GetValidatorBalances(slasher, []spec.Epoch{epoch, epoch + 1})
	}, "ListValidatorBalance(epoch=%v, slasher=%v)", epoch, slasher)
	if err != nil {
		log.Error().Err(err).Msg("ListValidatorBalance failed while determing slasher's reward")
	} else {
		rewardStr = fmt.Sprintf("next epoch reward is %.03f ETH", float32(balances[epoch+1]-balances[epoch])*1e-9)
	}

	log.Warn().Msgf("%s Slashing occurred! Validator %v %s and slashed by %v at slot %v; %s",
		prefix, slashee, reason, slasher, slot, rewardStr)
}

func ProcessSlashings(ctx context.Context, blocks map[spec.Slot]*ChainBlock) (err error) {
	for slot, block := range blocks {
		body := block.BlockContainer.Block.Block.Body
		slasher := block.BlockContainer.Block.Block.ProposerIndex

		for _, proposerSlashing := range body.ProposerSlashings {
			slashee := proposerSlashing.Header_1.Header.ProposerIndex

			ReportSlashing(ctx, "🚫 🧱", "proposed two conflicting blocks",
				slot, slasher, slashee)
		}

		for _, attesterSlashing := range body.AttesterSlashings {
			var slashee spec.ValidatorIndex
			attestation1Validators := make(map[spec.ValidatorIndex]interface{})
			for _, index := range attesterSlashing.Attestation_1.AttestingIndices {
				attestation1Validators[index] = nil
			}

			for _, index := range attesterSlashing.Attestation_2.AttestingIndices {
				if _, ok := attestation1Validators[index]; ok {
					slashee = index
					break
				}
			}

			ReportSlashing(ctx, "🚫 🧾", "attested two conflicting blocks",
				slot, slasher, slashee)
		}
	}

	return nil
}
