package main

import (
	"context"
	"fmt"
	"log"

	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/routing/route"
)

func runAttack(ctx context.Context, graph *GraphHarness,
	jammer *JammingHarness, targetNode route.Vertex,
	targetPeerAlias string) error {

	node, err := graph.LookupByAlias(ctx, targetPeerAlias)
	if err != nil {
		return err
	}

	err = OpenChannels(ctx, graph, targetNode, node.PubKey)
	if err != nil {
		return err
	}

	return nil
}

// OpenChannels opens a set of channels that can be used to build reputation
// with a target node for use over its channel with the target peer provided.
// This function assumes that the target and its peers are directly connected
// with *one* channel.
//
// Given: Target --- Peer
// Note: == represents two channels, -- represents one
//
// This function will open channels as follows:
//
//	   LND0
//		|
//
//	   Target --- Peer === LND2
//
//		|
//	    LND1
func OpenChannels(ctx context.Context, graph *GraphHarness, targetNode,
	targetPeer route.Vertex) error {

	// LND0 -> Target
	chanCap := funding.MaxBtcFundingAmount
	chan1, err := graph.OpenChannel(ctx, OpenChannelReq{
		Source:      0,
		Dest:        targetNode,
		CapacitySat: chanCap,
		PushAmt:     chanCap / 2,
	})
	if err != nil {
		return fmt.Errorf("LND-0 -> target: %v", err)
	}

	log.Printf("Opened channel with target node (%s) from LND-0", targetNode)

	// LND-1 -> Target
	chan2, err := graph.OpenChannel(ctx, OpenChannelReq{
		Source:      1,
		Dest:        targetNode,
		CapacitySat: chanCap,
		PushAmt:     chanCap / 2,
	})
	if err != nil {
		return fmt.Errorf("LND-1 -> target: %v", err)
	}

	log.Printf("Opened channel with target node (%s) from LND-1", targetNode)

	// LND-2 -> Peer x2
	req := OpenChannelReq{
		Source:      2,
		Dest:        targetPeer,
		CapacitySat: chanCap,
		// We still give ourselves some liquidity so that we don't
		// run into fee spike buffer issues.
		PushAmt: chanCap / 2,
	}
	chan3, err := graph.OpenChannel(ctx, req)
	if err != nil {
		return fmt.Errorf("LND-2 -> target peer: %v", err)
	}
	log.Printf("Opened channel 1 with target peer (%s) from LND-2",
		targetPeer)

	chan4, err := graph.OpenChannel(ctx, req)
	if err != nil {
		return fmt.Errorf("LND-2 -> target peer: %v", err)
	}
	log.Printf("Opened channel 2 with target peer (%s) from LND-2",
		targetPeer)

	// Wait for channels to reflect in graphs.
	fmt.Println("Waiting for channels to reflect in graphs")
	graph.WaitForChannel(ctx, 1, 0, chan1)
	graph.WaitForChannel(ctx, 2, 0, chan1)

	graph.WaitForChannel(ctx, 0, 1, chan2)
	graph.WaitForChannel(ctx, 2, 1, chan2)

	graph.WaitForChannel(ctx, 0, 2, chan3)
	graph.WaitForChannel(ctx, 1, 2, chan3)

	graph.WaitForChannel(ctx, 0, 2, chan4)
	graph.WaitForChannel(ctx, 1, 2, chan4)

	return nil
}
