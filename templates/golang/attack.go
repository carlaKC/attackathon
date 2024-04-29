package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lightningnetwork/lnd/signal"
)

// RunSlowJammingAttack runs a slow jamming attack against the protected
// bucket:
// - Builds up reputation
// - Probes to determine whether reputation is sufficient to jam protected
// - TODO
func RunSlowJammingAttack(ctxb context.Context, s signal.Interceptor,
	r *ReputationHarness) error {
	var (
		ctx, cancel = context.WithCancel(ctxb)
		wg          sync.WaitGroup
		errChan     = make(chan error, 2)
	)

	// On exit, signal shutdown with cancel and wait for everything to
	// clean up.
	defer func() {
		fmt.Println("Jamming attack exiting")
		cancel()
		wg.Wait()
	}()

	// Start by dispatching payments in the background to build reputation.
	fmt.Println("Starting reputation-building payments")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.BuildReputation(ctx, 0, 2); err != nil {
			errChan <- err
		}

		fmt.Println("Waiting for 0-> 2 reputation to exit")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.BuildReputation(ctx, 2, 0); err != nil {
			errChan <- err
		}

		fmt.Println("Waiting for 2 -> 0 reputation to exit")
	}()

	for i := 0; i < 5; i++ {
		// Sleep every time we try, so that there's a chance to build
		// reputation inbetween attempts.
		fmt.Println("Sleeping 10 seconds before attempting attack")
		select {
		case <-time.After(time.Second * 10):

		case <-ctx.Done():
			return ctx.Err()

		case err := <-errChan:
			return err

		case <-s.ShutdownChannel():
			return errShutdown
		}

		fmt.Println("Dispatching reputation probes")
		outcome, err := r.ProbeEndorsedCount(ctx, 0)
		if err != nil {
			return err
		}

		fmt.Printf("Probe outcome: dispatched %v, received %v "+
			"endorsed: %v, timeout: %v\n", outcome.SenderDispatched,
			outcome.ReceiverResolved, outcome.EndorsedCount,
			outcome.Timeout)

		if outcome.Timeout {
			continue
		}

		// TODO: run actual attack once we get to
		if outcome.SenderDispatched == outcome.EndorsedCount {
			return errors.New("time to write you attack!")
		}
	}

	return nil
}
