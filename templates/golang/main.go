package main

import (
	"context"
	"log"
	"os"

	"github.com/lightningnetwork/lnd/routing/route"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	lnds, err := getLndNodes(ctx)
	if err != nil {
		log.Fatalf("Could not set up connection: %v", err)
		os.Exit(1)
	}

	target, err := route.NewVertexFromStr(os.Getenv("TARGET"))
	if err != nil {
		log.Fatalf("Could not get target node: %v", err)
		os.Exit(1)
	}

	log.Printf("Starting attack against: %v", target)
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

	cleanup := func(force bool) error {
		log.Println("Cleaning up channels for lnd-0")
		if err := graph.CloseAllChannels(ctx, 0, force); err != nil {
			return err
		}

		log.Println("Cleaning up channels for lnd-1")
		if err := graph.CloseAllChannels(ctx, 1, force); err != nil {
			return err
		}

		log.Println("Cleaning up channels for lnd-2")
		if err := graph.CloseAllChannels(ctx, 2, force); err != nil {
			return err
		}

		return nil
	}

	// Run cleanup on start to get rid of any lingering channels. Force
	// close to clean up any old state from an unsuccessful run.
	log.Println("Cleaning up any attacker channels from previous runs")
	if err := cleanup(true); err != nil {
		log.Fatalf("Could not clean up on start: %v", err)
		os.Exit(1)
	}

	// Always cleanup at the end of our run. Do not force close because
	// we expect all payments to be resolved.
	defer func() {
		if err := cleanup(false); err != nil {
			log.Fatalf("Could not clean up channels: %v", err)
		}

		log.Println("Waiting for threads to shutdown")
		cancel()
		jammer.wg.Wait()
	}()

	// Temporarily fixing to a single target peer.
	if err := runAttack(ctx, graph, jammer, target, "6", false); err != nil {
		log.Fatalf("Attack error: %v", err)
	}
}
