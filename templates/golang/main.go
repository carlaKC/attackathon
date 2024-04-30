package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lightningnetwork/lnd/routing/route"
)

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

	if err := cleanup(); err != nil {
		fmt.Printf("Could not clean up channels: %v\n", err)
	}

	fmt.Println("Waiting for threads to shutdown")
	cancel()
	jammer.wg.Wait()
	graph.wg.Wait()
}
