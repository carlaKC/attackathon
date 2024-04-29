package main

import (
	"context"
	"fmt"

	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/routing/route"
)

type ReputationHarness struct {
	LndNodes Nodes

	graph  *GraphHarness
	jammer *JammingHarness
}

// OpenChannels opens a set of channels that can be used to build reputation
// with a target node for use over its channel with the target peer provided.
// This function assumes that the target and its peers are directly connected
// with *one* channel.
//
// Given: Target --- Peer
//
// This function will open channels as follows:
//
//	   LND0 (builds and abuses reputation)
//		|
//
//	   Target --- Peer --- LND1 (used to probe endorsement)
//
//		|
//	    LND2 (maintains good reputation)
func (r *ReputationHarness) OpenChannels(ctx context.Context, targetNode,
	targetPeer route.Vertex) error {

	chanCap := funding.MaxBtcFundingAmount
	_, err := r.graph.OpenChannel(ctx, OpenChannelReq{
		Source:      0,
		Dest:        targetNode,
		CapacitySat: chanCap,
		PushAmt:     chanCap / 2,
	})
	if err != nil {
		return fmt.Errorf("LND-0 -> target: %v", err)
	}

	fmt.Printf("Opened channel with target node (%s) from LND-0\n",
		targetNode)

	// With attack node 1, open a channel with the targets peer & push a
	// large amount to them.
	_, err = r.graph.OpenChannel(ctx, OpenChannelReq{
		Source:      1,
		Dest:        targetPeer,
		CapacitySat: chanCap,
		PushAmt:     chanCap / 2,
	})
	if err != nil {
		return fmt.Errorf("LND-1 -> target peer: %v", err)
	}
	fmt.Printf("Opened channel with target peer (%s) from LND-1\n",
		targetPeer)

	// Open chan from a2 to target (a2 is our "good" node). We will build
	// good rep for it and maintain that good rep. If its payments stop
	// going through then we know we have jammed the target channel.
	_, err = r.graph.OpenChannel(ctx, OpenChannelReq{
		Source:      2,
		Dest:        targetNode,
		CapacitySat: chanCap,
		PushAmt:     chanCap / 2,
	})
	if err != nil {
		return fmt.Errorf("LND-2 -> target: %v", err)

	}

	fmt.Printf("Opened channel with target node (%s) from LND-2\n",
		targetNode)

	return nil
}
