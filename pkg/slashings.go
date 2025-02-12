package pkg

import (
	"context"
	"eth2-monitor/prysmgrpc"
	"eth2-monitor/spec"
	"fmt"

	"eth2-monitor/cmd/opts"

	ethpb "github.com/prysmaticlabs/prysm/v2/proto/prysm/v1alpha1"
	"github.com/rs/zerolog/log"
)

func ReportSlashing(ctx context.Context, prefix string, reason string, slot spec.Slot, slasher spec.ValidatorIndex, slashee spec.ValidatorIndex) {
	var epoch spec.Epoch = slot / spec.SLOTS_PER_EPOCH
	var balances map[spec.Epoch]spec.Gwei

	rewardStr := ""

	if opts.Slashings.ShowSlashingReward {
		rewardStr = "; reward is unknown"

		s, err := prysmgrpc.New(ctx, prysmgrpc.WithAddress(opts.BeaconNode))
		if err != nil {
			log.Error().Err(err).Msg("ReportSlashing failed while reporting a slashing")
			return
		}

		Measure(func() {
			balances, err = s.GetValidatorBalances(slasher, []spec.Epoch{epoch, epoch + 1})
		}, "ListValidatorBalance(epoch=%v, slasher=%v)", epoch, slasher)
		if err != nil {
			log.Error().Err(err).Msg("ListValidatorBalance failed while determining slasher's reward")
		} else {
			rewardStr = fmt.Sprintf("; next epoch reward is %.03f ETH", float32(balances[epoch+1]-balances[epoch])*1e-9)
		}
	}

	Report("%s Slashing occurred! Validator %v %s and slashed by %v at slot %v%s",
		prefix, slashee, reason, slasher, slot, rewardStr)
	TweetSlashing(reason, slot, slasher, slashee)
}

func ProcessSlashings(ctx context.Context, blocks map[spec.Slot][]*ChainBlock) (err error) {
	for slot, blockContainers := range blocks {
		for _, block := range blockContainers {
			var slasher spec.ValidatorIndex
			var proposerSlashings []*ethpb.ProposerSlashing
			var attesterSlashings []*ethpb.AttesterSlashing

			switch block.BlockContainer.Block.(type) {
			case *ethpb.BeaconBlockContainer_Phase0Block:
				phase0Block := block.BlockContainer.GetPhase0Block().Block
				slasher = spec.ValidatorIndex(phase0Block.ProposerIndex)
				proposerSlashings = phase0Block.Body.ProposerSlashings
				attesterSlashings = phase0Block.Body.AttesterSlashings
			case *ethpb.BeaconBlockContainer_AltairBlock:
				altairBlock := block.BlockContainer.GetAltairBlock().Block
				slasher = spec.ValidatorIndex(altairBlock.ProposerIndex)
				proposerSlashings = altairBlock.Body.ProposerSlashings
				attesterSlashings = altairBlock.Body.AttesterSlashings
			}

			for _, proposerSlashing := range proposerSlashings {
				slashee := spec.ValidatorIndex(proposerSlashing.Header_1.Header.ProposerIndex)

				ReportSlashing(ctx, "🚫 🧱", "proposed two conflicting blocks",
					slot, slasher, slashee)
			}

			for _, attesterSlashing := range attesterSlashings {
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
	}

	return nil
}
