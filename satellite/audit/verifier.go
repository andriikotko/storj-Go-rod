// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package audit

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/spacemonkeygo/monkit/v3"
	"github.com/vivint/infectious"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/errs2"
	"storj.io/common/identity"
	"storj.io/common/memory"
	"storj.io/common/pb"
	"storj.io/common/pkcrypto"
	"storj.io/common/rpc"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/common/storj"
	"storj.io/storj/satellite/metainfo"
	"storj.io/storj/satellite/metainfo/metabase"
	"storj.io/storj/satellite/orders"
	"storj.io/storj/satellite/overlay"
	"storj.io/storj/storage"
	"storj.io/uplink/private/eestream"
	"storj.io/uplink/private/piecestore"
)

var (
	mon = monkit.Package()

	// ErrNotEnoughShares is the errs class for when not enough shares are available to do an audit.
	ErrNotEnoughShares = errs.Class("not enough shares for successful audit")
	// ErrSegmentDeleted is the errs class when the audited segment was deleted during the audit.
	ErrSegmentDeleted = errs.Class("segment deleted during audit")
	// ErrSegmentModified is the errs class used when a segment has been changed in any way.
	ErrSegmentModified = errs.Class("segment has been modified")
)

// Share represents required information about an audited share.
type Share struct {
	Error    error
	PieceNum int
	NodeID   storj.NodeID
	Data     []byte
}

// Verifier helps verify the correctness of a given stripe
//
// architecture: Worker
type Verifier struct {
	log                *zap.Logger
	metainfo           *metainfo.Service
	orders             *orders.Service
	auditor            *identity.PeerIdentity
	dialer             rpc.Dialer
	overlay            *overlay.Service
	containment        Containment
	minBytesPerSecond  memory.Size
	minDownloadTimeout time.Duration

	OnTestingCheckSegmentAlteredHook func()

	// Temporary fields for the verify-piece-hashes command
	OnTestingVerifyMockFunc func() (Report, error)
	UsedToVerifyPieceHashes bool
}

// NewVerifier creates a Verifier.
func NewVerifier(log *zap.Logger, metainfo *metainfo.Service, dialer rpc.Dialer, overlay *overlay.Service, containment Containment, orders *orders.Service, id *identity.FullIdentity, minBytesPerSecond memory.Size, minDownloadTimeout time.Duration) *Verifier {
	return &Verifier{
		log:                log,
		metainfo:           metainfo,
		orders:             orders,
		auditor:            id.PeerIdentity(),
		dialer:             dialer,
		overlay:            overlay,
		containment:        containment,
		minBytesPerSecond:  minBytesPerSecond,
		minDownloadTimeout: minDownloadTimeout,
	}
}

// Verify downloads shares then verifies the data correctness at a random stripe.
func (verifier *Verifier) Verify(ctx context.Context, path storj.Path, skip map[storj.NodeID]bool) (report Report, err error) {
	defer mon.Task()(&ctx)(&err)

	pointerBytes, pointer, err := verifier.metainfo.GetWithBytes(ctx, metabase.SegmentKey(path))
	if err != nil {
		if storj.ErrObjectNotFound.Has(err) {
			verifier.log.Debug("segment deleted before Verify")
			return Report{}, nil
		}
		return Report{}, err
	}
	if pointer.ExpirationDate != (time.Time{}) && pointer.ExpirationDate.Before(time.Now()) {
		errDelete := verifier.metainfo.Delete(ctx, metabase.SegmentKey(path), pointerBytes)
		if errDelete != nil {
			return Report{}, Error.Wrap(errDelete)
		}
		verifier.log.Debug("segment expired before Verify")
		return Report{}, nil
	}

	defer func() {
		if verifier.UsedToVerifyPieceHashes {
			return
		}
		// if piece hashes have not been verified for this segment, do not mark nodes as failing audit
		if !pointer.PieceHashesVerified {
			report.PendingAudits = nil
			report.Fails = nil
		}
	}()

	randomIndex, err := GetRandomStripe(ctx, pointer)
	if err != nil {
		return Report{}, err
	}

	shareSize := pointer.GetRemote().GetRedundancy().GetErasureShareSize()
	segmentLocation, err := metabase.ParseSegmentKey(metabase.SegmentKey(path))
	if err != nil {
		return Report{}, err
	}

	var offlineNodes storj.NodeIDList
	var failedNodes storj.NodeIDList
	var unknownNodes storj.NodeIDList
	containedNodes := make(map[int]storj.NodeID)
	sharesToAudit := make(map[int]Share)

	orderLimits, privateKey, err := verifier.orders.CreateAuditOrderLimits(ctx, segmentLocation.Bucket(), pointer, skip)
	if err != nil {
		return Report{}, err
	}

	// NOTE offlineNodes will include disqualified nodes because they aren't in
	// the skip list
	offlineNodes = getOfflineNodes(pointer, orderLimits, skip)
	if len(offlineNodes) > 0 {
		verifier.log.Debug("Verify: order limits not created for some nodes (offline/disqualified)",
			zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
			zap.Strings("Node IDs", offlineNodes.Strings()))
	}

	shares, err := verifier.DownloadShares(ctx, orderLimits, privateKey, randomIndex, shareSize)
	if err != nil {
		return Report{
			Offlines: offlineNodes,
		}, err
	}

	err = verifier.checkIfSegmentAltered(ctx, path, pointer, pointerBytes)
	if err != nil {
		if ErrSegmentDeleted.Has(err) {
			verifier.log.Debug("segment deleted during Verify")
			return Report{}, nil
		}
		if ErrSegmentModified.Has(err) {
			verifier.log.Debug("segment modified during Verify")
			return Report{}, nil
		}
		return Report{
			Offlines: offlineNodes,
		}, err
	}

	for pieceNum, share := range shares {
		if share.Error == nil {
			// no error -- share downloaded successfully
			sharesToAudit[pieceNum] = share
			continue
		}
		if rpc.Error.Has(share.Error) {
			if errs.Is(share.Error, context.DeadlineExceeded) {
				// dial timeout
				offlineNodes = append(offlineNodes, share.NodeID)
				verifier.log.Debug("Verify: dial timeout (offline)",
					zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
					zap.Stringer("Node ID", share.NodeID),
					zap.Error(share.Error))
				continue
			}
			if errs2.IsRPC(share.Error, rpcstatus.Unknown) {
				// dial failed -- offline node
				offlineNodes = append(offlineNodes, share.NodeID)
				verifier.log.Debug("Verify: dial failed (offline)",
					zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
					zap.Stringer("Node ID", share.NodeID),
					zap.Error(share.Error))
				continue
			}
			// unknown transport error
			unknownNodes = append(unknownNodes, share.NodeID)
			verifier.log.Info("Verify: unknown transport error (skipped)",
				zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
				zap.Stringer("Node ID", share.NodeID),
				zap.Error(share.Error))
			continue
		}

		if errs2.IsRPC(share.Error, rpcstatus.NotFound) {
			// missing share
			failedNodes = append(failedNodes, share.NodeID)
			verifier.log.Info("Verify: piece not found (audit failed)",
				zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
				zap.Stringer("Node ID", share.NodeID),
				zap.Error(share.Error))
			continue
		}

		if errs2.IsRPC(share.Error, rpcstatus.DeadlineExceeded) {
			// dial successful, but download timed out
			containedNodes[pieceNum] = share.NodeID
			verifier.log.Info("Verify: download timeout (contained)",
				zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
				zap.Stringer("Node ID", share.NodeID),
				zap.Error(share.Error))
			continue
		}

		// unknown error
		unknownNodes = append(unknownNodes, share.NodeID)
		verifier.log.Info("Verify: unknown error (skipped)",
			zap.Bool("Piece Hash Verified", pointer.PieceHashesVerified),
			zap.Stringer("Node ID", share.NodeID),
			zap.Error(share.Error))
	}

	mon.IntVal("verify_shares_downloaded_successfully").Observe(int64(len(sharesToAudit))) //mon:locked

	required := int(pointer.Remote.Redundancy.GetMinReq())
	total := int(pointer.Remote.Redundancy.GetTotal())

	if len(sharesToAudit) < required {
		mon.Counter("not_enough_shares_for_audit").Inc(1)
		return Report{
			Fails:    failedNodes,
			Offlines: offlineNodes,
			Unknown:  unknownNodes,
		}, ErrNotEnoughShares.New("got %d, required %d", len(sharesToAudit), required)
	}
	// ensure we get values, even if only zero values, so that redash can have an alert based on this
	mon.Counter("not_enough_shares_for_audit").Inc(0)

	pieceNums, correctedShares, err := auditShares(ctx, required, total, sharesToAudit)
	if err != nil {
		return Report{
			Fails:    failedNodes,
			Offlines: offlineNodes,
			Unknown:  unknownNodes,
		}, err
	}

	for _, pieceNum := range pieceNums {
		failedNodes = append(failedNodes, shares[pieceNum].NodeID)
	}

	successNodes := getSuccessNodes(ctx, shares, failedNodes, offlineNodes, unknownNodes, containedNodes)

	totalInPointer := len(pointer.GetRemote().GetRemotePieces())
	numOffline := len(offlineNodes)
	numSuccessful := len(successNodes)
	numFailed := len(failedNodes)
	numContained := len(containedNodes)
	numUnknown := len(unknownNodes)
	totalAudited := numSuccessful + numFailed + numOffline + numContained
	auditedPercentage := float64(totalAudited) / float64(totalInPointer)
	offlinePercentage := float64(0)
	successfulPercentage := float64(0)
	failedPercentage := float64(0)
	containedPercentage := float64(0)
	unknownPercentage := float64(0)
	if totalAudited > 0 {
		offlinePercentage = float64(numOffline) / float64(totalAudited)
		successfulPercentage = float64(numSuccessful) / float64(totalAudited)
		failedPercentage = float64(numFailed) / float64(totalAudited)
		containedPercentage = float64(numContained) / float64(totalAudited)
		unknownPercentage = float64(numUnknown) / float64(totalAudited)
	}

	mon.Meter("audit_success_nodes_global").Mark(numSuccessful)        //mon:locked
	mon.Meter("audit_fail_nodes_global").Mark(numFailed)               //mon:locked
	mon.Meter("audit_offline_nodes_global").Mark(numOffline)           //mon:locked
	mon.Meter("audit_contained_nodes_global").Mark(numContained)       //mon:locked
	mon.Meter("audit_unknown_nodes_global").Mark(numUnknown)           //mon:locked
	mon.Meter("audit_total_nodes_global").Mark(totalAudited)           //mon:locked
	mon.Meter("audit_total_pointer_nodes_global").Mark(totalInPointer) //mon:locked

	mon.IntVal("audit_success_nodes").Observe(int64(numSuccessful))           //mon:locked
	mon.IntVal("audit_fail_nodes").Observe(int64(numFailed))                  //mon:locked
	mon.IntVal("audit_offline_nodes").Observe(int64(numOffline))              //mon:locked
	mon.IntVal("audit_contained_nodes").Observe(int64(numContained))          //mon:locked
	mon.IntVal("audit_unknown_nodes").Observe(int64(numUnknown))              //mon:locked
	mon.IntVal("audit_total_nodes").Observe(int64(totalAudited))              //mon:locked
	mon.IntVal("audit_total_pointer_nodes").Observe(int64(totalInPointer))    //mon:locked
	mon.FloatVal("audited_percentage").Observe(auditedPercentage)             //mon:locked
	mon.FloatVal("audit_offline_percentage").Observe(offlinePercentage)       //mon:locked
	mon.FloatVal("audit_successful_percentage").Observe(successfulPercentage) //mon:locked
	mon.FloatVal("audit_failed_percentage").Observe(failedPercentage)         //mon:locked
	mon.FloatVal("audit_contained_percentage").Observe(containedPercentage)   //mon:locked
	mon.FloatVal("audit_unknown_percentage").Observe(unknownPercentage)       //mon:locked

	pendingAudits, err := createPendingAudits(ctx, containedNodes, correctedShares, pointer, randomIndex, path)
	if err != nil {
		return Report{
			Successes: successNodes,
			Fails:     failedNodes,
			Offlines:  offlineNodes,
			Unknown:   unknownNodes,
		}, err
	}

	return Report{
		Successes:     successNodes,
		Fails:         failedNodes,
		Offlines:      offlineNodes,
		PendingAudits: pendingAudits,
		Unknown:       unknownNodes,
	}, nil
}

// DownloadShares downloads shares from the nodes where remote pieces are located.
func (verifier *Verifier) DownloadShares(ctx context.Context, limits []*pb.AddressedOrderLimit, piecePrivateKey storj.PiecePrivateKey, stripeIndex int64, shareSize int32) (shares map[int]Share, err error) {
	defer mon.Task()(&ctx)(&err)

	shares = make(map[int]Share, len(limits))
	ch := make(chan *Share, len(limits))

	for i, limit := range limits {
		if limit == nil {
			ch <- nil
			continue
		}

		go func(i int, limit *pb.AddressedOrderLimit) {
			share, err := verifier.GetShare(ctx, limit, piecePrivateKey, stripeIndex, shareSize, i)
			if err != nil {
				share = Share{
					Error:    err,
					PieceNum: i,
					NodeID:   limit.GetLimit().StorageNodeId,
					Data:     nil,
				}
			}
			ch <- &share
		}(i, limit)
	}

	for range limits {
		share := <-ch
		if share != nil {
			shares[share.PieceNum] = *share
		}
	}

	return shares, nil
}

// Reverify reverifies the contained nodes in the stripe.
func (verifier *Verifier) Reverify(ctx context.Context, path storj.Path) (report Report, err error) {
	defer mon.Task()(&ctx)(&err)

	// result status enum
	const (
		skipped = iota
		success
		offline
		failed
		contained
		unknown
		erred
	)

	type result struct {
		nodeID       storj.NodeID
		status       int
		pendingAudit *PendingAudit
		err          error
	}

	pointerBytes, pointer, err := verifier.metainfo.GetWithBytes(ctx, metabase.SegmentKey(path))
	if err != nil {
		if storj.ErrObjectNotFound.Has(err) {
			verifier.log.Debug("segment deleted before Reverify")
			return Report{}, nil
		}
		return Report{}, err
	}
	if pointer.ExpirationDate != (time.Time{}) && pointer.ExpirationDate.Before(time.Now()) {
		errDelete := verifier.metainfo.Delete(ctx, metabase.SegmentKey(path), pointerBytes)
		if errDelete != nil {
			return Report{}, Error.Wrap(errDelete)
		}
		verifier.log.Debug("Segment expired before Reverify")
		return Report{}, nil
	}

	pieceHashesVerified := make(map[storj.NodeID]bool)
	pieceHashesVerifiedMutex := &sync.Mutex{}
	defer func() {
		pieceHashesVerifiedMutex.Lock()

		// for each node in Fails and PendingAudits, remove if piece hashes not verified for that segment
		// if the piece hashes are not verified, remove the "failed" node from containment
		newFails := storj.NodeIDList{}
		newPendingAudits := []*PendingAudit{}

		for _, id := range report.Fails {
			if pieceHashesVerified[id] {
				newFails = append(newFails, id)
			} else {
				_, errDelete := verifier.containment.Delete(ctx, id)
				if errDelete != nil {
					verifier.log.Debug("Error deleting node from containment db", zap.Stringer("Node ID", id), zap.Error(errDelete))
				}
			}
		}
		for _, pending := range report.PendingAudits {
			if pieceHashesVerified[pending.NodeID] {
				newPendingAudits = append(newPendingAudits, pending)
			} else {
				_, errDelete := verifier.containment.Delete(ctx, pending.NodeID)
				if errDelete != nil {
					verifier.log.Debug("Error deleting node from containment db", zap.Stringer("Node ID", pending.NodeID), zap.Error(errDelete))
				}
			}
		}

		report.Fails = newFails
		report.PendingAudits = newPendingAudits

		pieceHashesVerifiedMutex.Unlock()
	}()

	pieces := pointer.GetRemote().GetRemotePieces()
	ch := make(chan result, len(pieces))
	var containedInSegment int64

	for _, piece := range pieces {
		pending, err := verifier.containment.Get(ctx, piece.NodeId)
		if err != nil {
			if ErrContainedNotFound.Has(err) {
				ch <- result{nodeID: piece.NodeId, status: skipped}
				continue
			}
			ch <- result{nodeID: piece.NodeId, status: erred, err: err}
			verifier.log.Debug("Reverify: error getting from containment db", zap.Stringer("Node ID", piece.NodeId), zap.Error(err))
			continue
		}
		containedInSegment++

		go func(pending *PendingAudit) {
			pendingPointerBytes, pendingPointer, err := verifier.metainfo.GetWithBytes(ctx, metabase.SegmentKey(pending.Path))
			if err != nil {
				if storj.ErrObjectNotFound.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: skipped}
					return
				}

				ch <- result{nodeID: pending.NodeID, status: erred, err: err}
				verifier.log.Debug("Reverify: error getting pending pointer from metainfo", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}
			if pendingPointer.ExpirationDate != (time.Time{}) && pendingPointer.ExpirationDate.Before(time.Now().UTC()) {
				errDelete := verifier.metainfo.Delete(ctx, metabase.SegmentKey(pending.Path), pendingPointerBytes)
				if errDelete != nil {
					verifier.log.Debug("Reverify: error deleting expired segment", zap.Stringer("Node ID", pending.NodeID), zap.Error(errDelete))
				}
				verifier.log.Debug("Reverify: segment already expired", zap.Stringer("Node ID", pending.NodeID))
				ch <- result{nodeID: pending.NodeID, status: skipped}
				return
			}

			// set whether piece hashes have been verified for this segment so we know whether to report a failed or pending audit for this node
			pieceHashesVerifiedMutex.Lock()
			pieceHashesVerified[pending.NodeID] = pendingPointer.PieceHashesVerified
			pieceHashesVerifiedMutex.Unlock()

			if pendingPointer.GetRemote().RootPieceId != pending.PieceID {
				ch <- result{nodeID: pending.NodeID, status: skipped}
				return
			}
			var pieceNum int32
			found := false
			for _, piece := range pendingPointer.GetRemote().GetRemotePieces() {
				if piece.NodeId == pending.NodeID {
					pieceNum = piece.PieceNum
					found = true
				}
			}
			if !found {
				ch <- result{nodeID: pending.NodeID, status: skipped}
				return
			}

			segmentLocation, err := metabase.ParseSegmentKey(metabase.SegmentKey(pending.Path)) // TODO: this should be parsed in pending
			if err != nil {
				ch <- result{nodeID: pending.NodeID, status: skipped}
				return
			}

			limit, piecePrivateKey, err := verifier.orders.CreateAuditOrderLimit(ctx, segmentLocation.Bucket(), pending.NodeID, pieceNum, pending.PieceID, pending.ShareSize)
			if err != nil {
				if overlay.ErrNodeDisqualified.Has(err) {
					_, errDelete := verifier.containment.Delete(ctx, pending.NodeID)
					if errDelete != nil {
						verifier.log.Debug("Error deleting disqualified node from containment db", zap.Stringer("Node ID", pending.NodeID), zap.Error(errDelete))
					}
					ch <- result{nodeID: pending.NodeID, status: erred, err: err}
					verifier.log.Debug("Reverify: order limit not created (disqualified)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				if overlay.ErrNodeFinishedGE.Has(err) {
					_, errDelete := verifier.containment.Delete(ctx, pending.NodeID)
					if errDelete != nil {
						verifier.log.Debug("Error deleting gracefully exited node from containment db", zap.Stringer("Node ID", pending.NodeID), zap.Error(errDelete))
					}
					ch <- result{nodeID: pending.NodeID, status: erred, err: err}
					verifier.log.Debug("Reverify: order limit not created (completed graceful exit)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				if overlay.ErrNodeOffline.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: offline}
					verifier.log.Debug("Reverify: order limit not created (offline)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				ch <- result{nodeID: pending.NodeID, status: erred, err: err}
				verifier.log.Debug("Reverify: error creating order limit", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}

			share, err := verifier.GetShare(ctx, limit, piecePrivateKey, pending.StripeIndex, pending.ShareSize, int(pieceNum))

			// check if the pending audit was deleted while downloading the share
			_, getErr := verifier.containment.Get(ctx, pending.NodeID)
			if getErr != nil {
				if ErrContainedNotFound.Has(getErr) {
					ch <- result{nodeID: pending.NodeID, status: skipped}
					verifier.log.Debug("Reverify: pending audit deleted during reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(getErr))
					return
				}
				ch <- result{nodeID: pending.NodeID, status: erred, err: getErr}
				verifier.log.Debug("Reverify: error getting from containment db", zap.Stringer("Node ID", pending.NodeID), zap.Error(getErr))
				return
			}

			// analyze the error from GetShare
			if err != nil {
				if rpc.Error.Has(err) {
					if errs.Is(err, context.DeadlineExceeded) {
						// dial timeout
						ch <- result{nodeID: pending.NodeID, status: offline}
						verifier.log.Debug("Reverify: dial timeout (offline)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						return
					}
					if errs2.IsRPC(err, rpcstatus.Unknown) {
						// dial failed -- offline node
						verifier.log.Debug("Reverify: dial failed (offline)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						ch <- result{nodeID: pending.NodeID, status: offline}
						return
					}
					// unknown transport error
					ch <- result{nodeID: pending.NodeID, status: unknown, pendingAudit: pending}
					verifier.log.Info("Reverify: unknown transport error (skipped)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				if errs2.IsRPC(err, rpcstatus.NotFound) {
					// Get the original segment pointer in the metainfo
					err := verifier.checkIfSegmentAltered(ctx, pending.Path, pendingPointer, pendingPointerBytes)
					if err != nil {
						ch <- result{nodeID: pending.NodeID, status: skipped}
						verifier.log.Debug("Reverify: audit source changed before reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						return
					}
					// missing share
					ch <- result{nodeID: pending.NodeID, status: failed}
					verifier.log.Info("Reverify: piece not found (audit failed)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				if errs2.IsRPC(err, rpcstatus.DeadlineExceeded) {
					// dial successful, but download timed out
					ch <- result{nodeID: pending.NodeID, status: contained, pendingAudit: pending}
					verifier.log.Info("Reverify: download timeout (contained)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				// unknown error
				ch <- result{nodeID: pending.NodeID, status: unknown, pendingAudit: pending}
				verifier.log.Info("Reverify: unknown error (skipped)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}
			downloadedHash := pkcrypto.SHA256Hash(share.Data)
			if bytes.Equal(downloadedHash, pending.ExpectedShareHash) {
				ch <- result{nodeID: pending.NodeID, status: success}
				verifier.log.Info("Reverify: hashes match (audit success)", zap.Stringer("Node ID", pending.NodeID))
			} else {
				err := verifier.checkIfSegmentAltered(ctx, pending.Path, pendingPointer, pendingPointerBytes)
				if err != nil {
					ch <- result{nodeID: pending.NodeID, status: skipped}
					verifier.log.Debug("Reverify: audit source changed before reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				verifier.log.Info("Reverify: hashes mismatch (audit failed)", zap.Stringer("Node ID", pending.NodeID),
					zap.Binary("expected hash", pending.ExpectedShareHash), zap.Binary("downloaded hash", downloadedHash))
				ch <- result{nodeID: pending.NodeID, status: failed}
			}
		}(pending)
	}

	for range pieces {
		result := <-ch
		switch result.status {
		case success:
			report.Successes = append(report.Successes, result.nodeID)
		case offline:
			report.Offlines = append(report.Offlines, result.nodeID)
		case failed:
			report.Fails = append(report.Fails, result.nodeID)
		case contained:
			report.PendingAudits = append(report.PendingAudits, result.pendingAudit)
		case unknown:
			report.Unknown = append(report.Unknown, result.nodeID)
		case skipped:
			_, errDelete := verifier.containment.Delete(ctx, result.nodeID)
			if errDelete != nil {
				verifier.log.Debug("Error deleting node from containment db", zap.Stringer("Node ID", result.nodeID), zap.Error(errDelete))
			}
		case erred:
			err = errs.Combine(err, result.err)
		}
	}

	mon.Meter("reverify_successes_global").Mark(len(report.Successes))     //mon:locked
	mon.Meter("reverify_offlines_global").Mark(len(report.Offlines))       //mon:locked
	mon.Meter("reverify_fails_global").Mark(len(report.Fails))             //mon:locked
	mon.Meter("reverify_contained_global").Mark(len(report.PendingAudits)) //mon:locked
	mon.Meter("reverify_unknown_global").Mark(len(report.Unknown))         //mon:locked

	mon.IntVal("reverify_successes").Observe(int64(len(report.Successes)))     //mon:locked
	mon.IntVal("reverify_offlines").Observe(int64(len(report.Offlines)))       //mon:locked
	mon.IntVal("reverify_fails").Observe(int64(len(report.Fails)))             //mon:locked
	mon.IntVal("reverify_contained").Observe(int64(len(report.PendingAudits))) //mon:locked
	mon.IntVal("reverify_unknown").Observe(int64(len(report.Unknown)))         //mon:locked

	mon.IntVal("reverify_contained_in_segment").Observe(containedInSegment) //mon:locked
	mon.IntVal("reverify_total_in_segment").Observe(int64(len(pieces)))     //mon:locked

	return report, err
}

// GetShare use piece store client to download shares from nodes.
func (verifier *Verifier) GetShare(ctx context.Context, limit *pb.AddressedOrderLimit, piecePrivateKey storj.PiecePrivateKey, stripeIndex int64, shareSize int32, pieceNum int) (share Share, err error) {
	defer mon.Task()(&ctx)(&err)

	bandwidthMsgSize := shareSize

	// determines number of seconds allotted for receiving data from a storage node
	timedCtx := ctx
	if verifier.minBytesPerSecond > 0 {
		maxTransferTime := time.Duration(int64(time.Second) * int64(bandwidthMsgSize) / verifier.minBytesPerSecond.Int64())
		if maxTransferTime < verifier.minDownloadTimeout {
			maxTransferTime = verifier.minDownloadTimeout
		}
		var cancel func()
		timedCtx, cancel = context.WithTimeout(ctx, maxTransferTime)
		defer cancel()
	}

	nodeurl := storj.NodeURL{
		ID:      limit.GetLimit().StorageNodeId,
		Address: limit.GetStorageNodeAddress().Address,
	}
	log := verifier.log.Named(nodeurl.ID.String())
	ps, err := piecestore.DialNodeURL(timedCtx, verifier.dialer, nodeurl, log, piecestore.DefaultConfig)
	if err != nil {
		return Share{}, Error.Wrap(err)
	}
	defer func() {
		err := ps.Close()
		if err != nil {
			verifier.log.Error("audit verifier failed to close conn to node: %+v", zap.Error(err))
		}
	}()

	offset := int64(shareSize) * stripeIndex

	downloader, err := ps.Download(timedCtx, limit.GetLimit(), piecePrivateKey, offset, int64(shareSize))
	if err != nil {
		return Share{}, err
	}
	defer func() { err = errs.Combine(err, downloader.Close()) }()

	buf := make([]byte, shareSize)
	_, err = io.ReadFull(downloader, buf)
	if err != nil {
		return Share{}, err
	}

	return Share{
		Error:    nil,
		PieceNum: pieceNum,
		NodeID:   nodeurl.ID,
		Data:     buf,
	}, nil
}

// checkIfSegmentAltered checks if path's pointer has been altered since path was selected.
func (verifier *Verifier) checkIfSegmentAltered(ctx context.Context, segmentKey string, oldPointer *pb.Pointer, oldPointerBytes []byte) (err error) {
	defer mon.Task()(&ctx)(&err)

	if verifier.OnTestingCheckSegmentAlteredHook != nil {
		verifier.OnTestingCheckSegmentAlteredHook()
	}

	newPointerBytes, newPointer, err := verifier.metainfo.GetWithBytes(ctx, metabase.SegmentKey(segmentKey))
	if err != nil {
		if storj.ErrObjectNotFound.Has(err) {
			return ErrSegmentDeleted.New("%q", segmentKey)
		}
		return err
	}

	if oldPointer != nil && oldPointer.CreationDate != newPointer.CreationDate {
		return ErrSegmentDeleted.New("%q", segmentKey)
	}

	if !bytes.Equal(oldPointerBytes, newPointerBytes) {
		return ErrSegmentModified.New("%q", segmentKey)
	}
	return nil
}

// auditShares takes the downloaded shares and uses infectious's Correct function to check that they
// haven't been altered. auditShares returns a slice containing the piece numbers of altered shares,
// and a slice of the corrected shares.
func auditShares(ctx context.Context, required, total int, originals map[int]Share) (pieceNums []int, corrected []infectious.Share, err error) {
	defer mon.Task()(&ctx)(&err)
	f, err := infectious.NewFEC(required, total)
	if err != nil {
		return nil, nil, err
	}

	copies, err := makeCopies(ctx, originals)
	if err != nil {
		return nil, nil, err
	}

	err = f.Correct(copies)
	if err != nil {
		return nil, nil, err
	}

	for _, share := range copies {
		if !bytes.Equal(originals[share.Number].Data, share.Data) {
			pieceNums = append(pieceNums, share.Number)
		}
	}
	return pieceNums, copies, nil
}

// makeCopies takes in a map of audit Shares and deep copies their data to a slice of infectious Shares.
func makeCopies(ctx context.Context, originals map[int]Share) (copies []infectious.Share, err error) {
	defer mon.Task()(&ctx)(&err)
	copies = make([]infectious.Share, 0, len(originals))
	for _, original := range originals {
		copies = append(copies, infectious.Share{
			Data:   append([]byte{}, original.Data...),
			Number: original.PieceNum})
	}
	return copies, nil
}

// getOfflines nodes returns these storage nodes from pointer which have no
// order limit nor are skipped.
func getOfflineNodes(pointer *pb.Pointer, limits []*pb.AddressedOrderLimit, skip map[storj.NodeID]bool) storj.NodeIDList {
	var offlines storj.NodeIDList

	nodesWithLimit := make(map[storj.NodeID]bool, len(limits))
	for _, limit := range limits {
		if limit != nil {
			nodesWithLimit[limit.GetLimit().StorageNodeId] = true
		}
	}

	for _, piece := range pointer.GetRemote().GetRemotePieces() {
		if !nodesWithLimit[piece.NodeId] && !skip[piece.NodeId] {
			offlines = append(offlines, piece.NodeId)
		}
	}

	return offlines
}

// getSuccessNodes uses the failed nodes, offline nodes and contained nodes arrays to determine which nodes passed the audit.
func getSuccessNodes(ctx context.Context, shares map[int]Share, failedNodes, offlineNodes, unknownNodes storj.NodeIDList, containedNodes map[int]storj.NodeID) (successNodes storj.NodeIDList) {
	defer mon.Task()(&ctx)(nil)
	fails := make(map[storj.NodeID]bool)
	for _, fail := range failedNodes {
		fails[fail] = true
	}
	for _, offline := range offlineNodes {
		fails[offline] = true
	}
	for _, unknown := range unknownNodes {
		fails[unknown] = true
	}
	for _, contained := range containedNodes {
		fails[contained] = true
	}

	for _, share := range shares {
		if !fails[share.NodeID] {
			successNodes = append(successNodes, share.NodeID)
		}
	}

	return successNodes
}

func createPendingAudits(ctx context.Context, containedNodes map[int]storj.NodeID, correctedShares []infectious.Share, pointer *pb.Pointer, randomIndex int64, path storj.Path) (pending []*PendingAudit, err error) {
	defer mon.Task()(&ctx)(&err)

	if len(containedNodes) == 0 {
		return nil, nil
	}

	redundancy := pointer.GetRemote().GetRedundancy()
	required := int(redundancy.GetMinReq())
	total := int(redundancy.GetTotal())
	shareSize := redundancy.GetErasureShareSize()

	fec, err := infectious.NewFEC(required, total)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	stripeData, err := rebuildStripe(ctx, fec, correctedShares, int(shareSize))
	if err != nil {
		return nil, Error.Wrap(err)
	}

	for pieceNum, nodeID := range containedNodes {
		share := make([]byte, shareSize)
		err = fec.EncodeSingle(stripeData, share, pieceNum)
		if err != nil {
			return nil, Error.Wrap(err)
		}
		pending = append(pending, &PendingAudit{
			NodeID:            nodeID,
			PieceID:           pointer.GetRemote().RootPieceId,
			StripeIndex:       randomIndex,
			ShareSize:         shareSize,
			ExpectedShareHash: pkcrypto.SHA256Hash(share),
			Path:              path,
		})
	}

	return pending, nil
}

func rebuildStripe(ctx context.Context, fec *infectious.FEC, corrected []infectious.Share, shareSize int) (_ []byte, err error) {
	defer mon.Task()(&ctx)(&err)
	stripe := make([]byte, fec.Required()*shareSize)
	err = fec.Rebuild(corrected, func(share infectious.Share) {
		copy(stripe[share.Number*shareSize:], share.Data)
	})
	if err != nil {
		return nil, err
	}
	return stripe, nil
}

// GetRandomStripe takes a pointer and returns a random stripe index within that pointer.
func GetRandomStripe(ctx context.Context, pointer *pb.Pointer) (index int64, err error) {
	defer mon.Task()(&ctx)(&err)
	redundancy, err := eestream.NewRedundancyStrategyFromProto(pointer.GetRemote().GetRedundancy())
	if err != nil {
		return 0, err
	}

	// the last segment could be smaller than stripe size
	if pointer.GetSegmentSize() < int64(redundancy.StripeSize()) {
		return 0, nil
	}

	var src cryptoSource
	rnd := rand.New(src)
	numStripes := pointer.GetSegmentSize() / int64(redundancy.StripeSize())
	randomStripeIndex := rnd.Int63n(numStripes)

	return randomStripeIndex, nil
}

// VerifyPieceHashes verifies the piece hashes for segments with piece_hashes_verified = false.
func (verifier *Verifier) VerifyPieceHashes(ctx context.Context, path storj.Path, dryRun bool) (changed bool, err error) {
	defer mon.Task()(&ctx)(&err)

	verifier.log.Info("Verifying piece hashes.", zap.String("Path", path))

	maxAttempts := 3
	for attempts := 0; attempts < maxAttempts; attempts++ {
		attempts++

		_, pointer, err := verifier.metainfo.GetWithBytes(ctx, metabase.SegmentKey(path))
		if err != nil {
			if storj.ErrObjectNotFound.Has(err) {
				verifier.log.Info("Segment not found.")
				return false, nil
			}
			return false, Error.Wrap(err)
		}

		if pointer.PieceHashesVerified {
			verifier.log.Info("Piece hashes already verified.")
			return false, nil
		}

		if pointer.Type != pb.Pointer_REMOTE {
			verifier.log.Info("Not a remote segment.")
			return false, nil
		}

		var report Report
		if verifier.OnTestingVerifyMockFunc != nil {
			report, err = verifier.OnTestingVerifyMockFunc()
		} else {
			report, err = verifier.Verify(ctx, path, nil)
		}
		if err != nil {
			return false, err
		}

		verifier.log.Info("Audit report received.",
			zap.Int("Success", report.Successes.Len()),
			zap.Int("Fails", report.Fails.Len()),
			zap.Int("Offlines", report.Offlines.Len()),
			zap.Int("Pending Audits", len(report.PendingAudits)),
			zap.Int("Unknown", report.Unknown.Len()),
		)

		if report.Successes.Len() == 0 {
			// skip it - this could happen if there was deleted or expired
			verifier.log.Info("Empty success list. Skipping the segment.")
			return false, nil
		}

		if report.Successes.Len() < int(pointer.Remote.Redundancy.MinReq) {
			verifier.log.Warn("Segment would be irreparable. Not fixing it.",
				zap.Int("Successful Nodes", report.Successes.Len()),
				zap.Int32("Minimum Required", pointer.Remote.Redundancy.MinReq))
			return false, nil
		}

		if report.Successes.Len() < int(pointer.Remote.Redundancy.RepairThreshold) {
			verifier.log.Warn("Segment would require repair. Not fixing it.",
				zap.Int("Successful Nodes", report.Successes.Len()),
				zap.Int32("Repair Threshold", pointer.Remote.Redundancy.RepairThreshold))
			return false, nil
		}

		toRemoveCount := report.Fails.Len() + report.Offlines.Len() + len(report.PendingAudits) + report.Unknown.Len()

		toRemove := make([]*pb.RemotePiece, 0, toRemoveCount)
		for _, piece := range pointer.Remote.RemotePieces {
			if !report.Successes.Contains(piece.NodeId) {
				toRemove = append(toRemove, piece)
			}
		}

		// sanity check
		if len(toRemove) != toRemoveCount {
			return false, Error.New("Pieces to remove (%d) do not match unsuccessful nodes (%d)", len(toRemove), toRemoveCount)
		}

		verifier.log.Info("Removing unsuccessful pieces from pointer.", zap.Int("Pieces To Remove", toRemoveCount))

		if dryRun {
			verifier.log.Info("Dry run, skipping the actual fix.", zap.Int("Successful Nodes", report.Successes.Len()))
			return true, nil
		}

		_, err = verifier.metainfo.UpdatePiecesCheckDuplicatesVerifyHashes(ctx, metabase.SegmentKey(path), pointer, nil, toRemove, false, true)
		if err != nil {
			if storage.ErrValueChanged.Has(err) {
				verifier.log.Info("Race detected while modifying segment pointer. Retrying...")
				continue
			}
			if storage.ErrKeyNotFound.Has(err) {
				verifier.log.Info("Object not found.")
				return false, nil
			}
			return false, Error.Wrap(err)
		}

		return true, nil
	}

	return false, Error.New("Failed to modify segment pointer in %d attempts.", maxAttempts)
}
