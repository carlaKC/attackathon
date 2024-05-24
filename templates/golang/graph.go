package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/routing/route"
)

// GraphHarness is responsible for all graph related operations (P2P, channels,
// gossip etc).
type GraphHarness struct {
	LndNodes Nodes
}

type OpenChannelReq struct {
	Source int
	Dest   route.Vertex
	// Host is an optional host address for the node, we'll lookup in our
	// graph if this value is empty.
	Host        string
	CapacitySat btcutil.Amount
	PushAmt     btcutil.Amount
	Private     bool
}

func (c *GraphHarness) WaitForSync(ctx context.Context, node int) error {
	for i := 0; i < 3; i++ {
		info, err := c.LndNodes.GetNode(node).Client.GetInfo(ctx)
		if err != nil {
			return err
		}

		if info.SyncedToChain && info.SyncedToGraph {
			return nil
		}

		select {
		case <-time.After(time.Second * 5):
			continue

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("Node: %v not synced in time", node)
}

// OpenChannel is a blocking call that opens a channel from the source node
// provided to the target:
// - looks up a node in the graph
// - connects to it if we are not currently connected
// - opens a channel with the parameters provided
// - opens a channel and mines block to confirm it
// - waits for the channel to be active
func (c *GraphHarness) OpenChannel(ctx context.Context,
	req OpenChannelReq) (*wire.OutPoint, error) {

	if err := c.WaitForSync(ctx, req.Source); err != nil {
		return nil, err
	}

	connected, err := c.PeerConnected(ctx, req.Source, req.Dest)
	if err != nil {
		return nil, err
	}

	if !connected {
		host := req.Host
		if host == "" {
			node, err := c.LookupNode(
				ctx, req.Source, req.Dest, false,
			)
			if err != nil {
				return nil, err
			}

			if len(node.Addresses) == 0 {
				return nil, fmt.Errorf("no public address "+
					"for: %v", req.Dest)
			}

			host = node.Addresses[0]
		}

		err = c.ConnectPeer(ctx, req.Source, req.Dest, host)
		if err != nil {
			return nil, err
		}
	}

	sourceNode := c.LndNodes.GetNode(req.Source)
	streamChan, errChan, err := sourceNode.Client.OpenChannelStream(
		ctx, req.Dest, req.CapacitySat, req.PushAmt, req.Private,
	)
	if err != nil {
		return nil, err
	}
	for {
		select {
		case update := <-streamChan:
			// Wait for the channel to be pending before we mine.
			if update.ChanPending != nil {
				if err := Mine(6); err != nil {
					return nil, fmt.Errorf("could not "+
						"mine: %v", err)
				}
			}

			if update.ChanOpen != nil {
				return outpointFromRPC(
					update.ChanOpen.ChannelPoint,
				), nil
			}

		case e := <-errChan:
			return nil, e

		case <-ctx.Done():
			return nil, ctx.Err()
		}

	}
}

// WaitForChannel waits for the channel provided (created by the channelNode)
// to be in the lookupNode's graph.
func (c *GraphHarness) WaitForChannel(ctx context.Context, lookupNode,
	channelNode int, channel *wire.OutPoint) error {

	info, err := c.LndNodes.GetNode(channelNode).Client.GetInfo(ctx)
	if err != nil {
		return err
	}

	pubkey, err := route.NewVertexFromBytes(info.IdentityPubkey[:])
	if err != nil {
		return err
	}

	for i := 0; i < 5; i++ {
		graphInfo, err := c.LookupNode(ctx, lookupNode, pubkey, true)
		if err == nil {

			for _, c := range graphInfo.Channels {
				if c.ChannelPoint == channel.String() {
					return nil
				}
			}
		}

		log.Printf("Lookup node %v with %v failed: %v\n",
			pubkey, lookupNode, err)

		select {
		case <-time.After(time.Second * 30):

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("Channel: %v not found in %v's graph", channel,
		lookupNode)
}

func (c *GraphHarness) LookupNode(ctx context.Context, source int,
	dest route.Vertex, includeChannels bool) (*lndclient.NodeInfo, error) {

	sourceNode := c.LndNodes.GetNode(source)
	destNode, err := sourceNode.Client.GetNodeInfo(
		ctx, dest, includeChannels,
	)
	if err != nil {
		return nil, err
	}

	return destNode, nil
}

func (c *GraphHarness) ConnectPeer(ctx context.Context, source int,
	dest route.Vertex, addr string) error {

	sourceNode := c.LndNodes.GetNode(source)
	if err := sourceNode.Client.Connect(ctx, dest, addr, true); err != nil {
		return err
	}

	for i := 0; i < 5; i++ {
		connected, err := c.PeerConnected(ctx, source, dest)
		if err != nil || connected {
			return err
		}

		select {
		case <-time.After(time.Second):

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("timeout waiting for peer: %v to connect", dest)
}

func (c *GraphHarness) PeerConnected(ctx context.Context, source int,
	target route.Vertex) (bool, error) {

	sourceNode := c.LndNodes.GetNode(source)

	peers, err := sourceNode.Client.ListPeers(ctx)
	if err != nil {
		return false, nil
	}

	for _, peer := range peers {
		if peer.Pubkey == target {
			return true, nil
		}
	}

	return false, nil
}

// CloseAllChannels cooperatively closes all the channels we currently have
// open and mines blocks to confirm them. Note that it *does not* wait for
// the channels to reflect as closed in our internal state.
func (c *GraphHarness) CloseAllChannels(ctx context.Context, node int) error {
	sourceNode := c.LndNodes.GetNode(node)
	channels, err := sourceNode.Client.ListChannels(ctx, false, false)
	if err != nil {
		return err
	}

	// Close all our channels, then do a once off mine and wait for them
	// to confirm.
	for _, channel := range channels {
		closeChan, errChan, err := sourceNode.Client.CloseChannel(
			ctx, outpointFromString(channel.ChannelPoint),
			true, 0, nil,
		)
		if err != nil {
			return err
		}

	waitForPending:
		for {
			// Just wait for our channel to get to a pending close
			// and then we can proceed to closing the next one.
			// We want to wait for this so that we know all
			// channels will be mined when we mine below.
			select {
			case c := <-closeChan:
				_, ok := c.(*lndclient.PendingCloseUpdate)
				if ok {
					break waitForPending
				}

			case e := <-errChan:
				log.Printf("Error closing channel: %v", e)
				return e

			case <-ctx.Done():
				return ctx.Err()
			}
		}

	}

	if len(channels) != 0 {
		Mine(6)
	}

	return nil

}
