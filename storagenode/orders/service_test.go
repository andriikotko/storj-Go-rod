// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package orders_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"storj.io/common/memory"
	"storj.io/common/pb"
	"storj.io/common/signing"
	"storj.io/common/testcontext"
	"storj.io/common/testrand"
	"storj.io/storj/private/testplanet"
	"storj.io/storj/satellite/metainfo/metabase"
	"storj.io/storj/satellite/overlay"
	"storj.io/storj/storagenode"
	"storj.io/storj/storagenode/orders"
	"storj.io/storj/storagenode/orders/ordersfile"
)

// TODO remove when db is removed.
func TestOrderDBSettle(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 1, UplinkCount: 1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		satellite := planet.Satellites[0]
		satellite.Audit.Worker.Loop.Pause()
		node := planet.StorageNodes[0]
		service := node.Storage2.Orders
		service.Sender.Pause()
		service.Cleanup.Pause()

		_, orderLimits, piecePrivateKey, err := satellite.Orders.Service.CreatePutOrderLimits(
			ctx,
			metabase.BucketLocation{ProjectID: planet.Uplinks[0].Projects[0].ID, BucketName: "testbucket"},
			[]*overlay.SelectedNode{
				{ID: node.ID(), LastIPPort: "fake", Address: new(pb.NodeAddress)},
			},
			time.Now().Add(2*time.Hour),
			2000,
		)
		require.NoError(t, err)
		require.Len(t, orderLimits, 1)

		orderLimit := orderLimits[0].Limit
		order := &pb.Order{
			SerialNumber: orderLimit.SerialNumber,
			Amount:       1000,
		}
		signedOrder, err := signing.SignUplinkOrder(ctx, piecePrivateKey, order)
		require.NoError(t, err)
		order0 := &ordersfile.Info{
			Limit: orderLimit,
			Order: signedOrder,
		}

		// enter orders into unsent_orders
		err = node.DB.Orders().Enqueue(ctx, order0)
		require.NoError(t, err)

		toSend, err := node.DB.Orders().ListUnsent(ctx, 10)
		require.NoError(t, err)
		require.Len(t, toSend, 1)

		// trigger order send
		service.Sender.TriggerWait()

		toSend, err = node.DB.Orders().ListUnsent(ctx, 10)
		require.NoError(t, err)
		require.Len(t, toSend, 0)

		archived, err := node.DB.Orders().ListArchived(ctx, 10)
		require.NoError(t, err)
		require.Len(t, archived, 1)
	})
}

func TestOrderFileStoreSettle(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 1, UplinkCount: 1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		satellite := planet.Satellites[0]
		uplinkPeer := planet.Uplinks[0]
		satellite.Audit.Worker.Loop.Pause()
		node := planet.StorageNodes[0]
		service := node.Storage2.Orders
		service.Sender.Pause()
		service.Cleanup.Pause()
		tomorrow := time.Now().Add(24 * time.Hour)

		// upload a file to generate an order on the storagenode
		testData := testrand.Bytes(8 * memory.KiB)
		err := uplinkPeer.Upload(ctx, satellite, "testbucket", "test/path", testData)
		require.NoError(t, err)

		toSend, err := node.OrdersStore.ListUnsentBySatellite(tomorrow)
		require.NoError(t, err)
		require.Len(t, toSend, 1)
		ordersForSat := toSend[satellite.ID()]
		require.Len(t, ordersForSat.InfoList, 1)

		// trigger order send
		service.SendOrders(ctx, tomorrow)

		toSend, err = node.OrdersStore.ListUnsentBySatellite(tomorrow)
		require.NoError(t, err)
		require.Len(t, toSend, 0)

		archived, err := node.OrdersStore.ListArchived()
		require.NoError(t, err)
		require.Len(t, archived, 1)
	})
}

// TODO remove when db is removed.
// TestOrderFileStoreAndDBSettle ensures that if orders exist in both DB and filestore, that the DB orders are settled first.
func TestOrderFileStoreAndDBSettle(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 1, UplinkCount: 1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		satellite := planet.Satellites[0]
		uplinkPeer := planet.Uplinks[0]
		satellite.Audit.Worker.Loop.Pause()
		node := planet.StorageNodes[0]
		service := node.Storage2.Orders
		service.Sender.Pause()
		service.Cleanup.Pause()
		tomorrow := time.Now().Add(24 * time.Hour)

		// add orders to orders DB
		_, orderLimits, piecePrivateKey, err := satellite.Orders.Service.CreatePutOrderLimits(
			ctx,
			metabase.BucketLocation{ProjectID: uplinkPeer.Projects[0].ID, BucketName: "testbucket"},
			[]*overlay.SelectedNode{
				{ID: node.ID(), LastIPPort: "fake", Address: new(pb.NodeAddress)},
			},
			time.Now().Add(2*time.Hour),
			2000,
		)
		require.NoError(t, err)
		require.Len(t, orderLimits, 1)

		orderLimit := orderLimits[0].Limit
		order := &pb.Order{
			SerialNumber: orderLimit.SerialNumber,
			Amount:       1000,
		}
		signedOrder, err := signing.SignUplinkOrder(ctx, piecePrivateKey, order)
		require.NoError(t, err)
		order0 := &ordersfile.Info{
			Limit: orderLimit,
			Order: signedOrder,
		}

		// enter orders into unsent_orders
		err = node.DB.Orders().Enqueue(ctx, order0)
		require.NoError(t, err)

		toSendDB, err := node.DB.Orders().ListUnsent(ctx, 10)
		require.NoError(t, err)
		require.Len(t, toSendDB, 1)

		// upload a file to add orders to filestore
		testData := testrand.Bytes(8 * memory.KiB)
		err = uplinkPeer.Upload(ctx, satellite, "testbucket", "test/path", testData)
		require.NoError(t, err)

		toSendFileStore, err := node.OrdersStore.ListUnsentBySatellite(tomorrow)
		require.NoError(t, err)
		require.Len(t, toSendFileStore, 1)
		ordersForSat := toSendFileStore[satellite.ID()]
		require.Len(t, ordersForSat.InfoList, 1)

		// trigger order send
		service.SendOrders(ctx, tomorrow)

		// DB orders should be archived, but filestore orders should still be unsent.
		toSendDB, err = node.DB.Orders().ListUnsent(ctx, 10)
		require.NoError(t, err)
		require.Len(t, toSendDB, 0)

		archived, err := node.DB.Orders().ListArchived(ctx, 10)
		require.NoError(t, err)
		require.Len(t, archived, 1)

		toSendFileStore, err = node.OrdersStore.ListUnsentBySatellite(tomorrow)
		require.NoError(t, err)
		require.Len(t, toSendFileStore, 1)
		ordersForSat = toSendFileStore[satellite.ID()]
		require.Len(t, ordersForSat.InfoList, 1)

		// trigger order send again
		service.SendOrders(ctx, tomorrow)

		// now FileStore orders should be archived too.
		toSendFileStore, err = node.OrdersStore.ListUnsentBySatellite(tomorrow)
		require.NoError(t, err)
		require.Len(t, toSendFileStore, 0)

		archived, err = node.OrdersStore.ListArchived()
		require.NoError(t, err)
		require.Len(t, archived, 1)
	})
}

// TODO remove when db is removed.
func TestCleanArchiveDB(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 1, UplinkCount: 0,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		planet.Satellites[0].Audit.Worker.Loop.Pause()
		satellite := planet.Satellites[0].ID()
		node := planet.StorageNodes[0]
		service := node.Storage2.Orders
		service.Sender.Pause()
		service.Cleanup.Pause()

		serialNumber0 := testrand.SerialNumber()
		serialNumber1 := testrand.SerialNumber()

		order0 := &ordersfile.Info{
			Limit: &pb.OrderLimit{
				SatelliteId:  satellite,
				SerialNumber: serialNumber0,
			},
			Order: &pb.Order{},
		}
		order1 := &ordersfile.Info{
			Limit: &pb.OrderLimit{
				SatelliteId:  satellite,
				SerialNumber: serialNumber1,
			},
			Order: &pb.Order{},
		}

		// enter orders into unsent_orders
		err := node.DB.Orders().Enqueue(ctx, order0)
		require.NoError(t, err)
		err = node.DB.Orders().Enqueue(ctx, order1)
		require.NoError(t, err)

		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)

		// archive one order yesterday, one today
		err = node.DB.Orders().Archive(ctx, yesterday, orders.ArchiveRequest{
			Satellite: satellite,
			Serial:    serialNumber0,
			Status:    orders.StatusAccepted,
		})
		require.NoError(t, err)

		err = node.DB.Orders().Archive(ctx, now, orders.ArchiveRequest{
			Satellite: satellite,
			Serial:    serialNumber1,
			Status:    orders.StatusAccepted,
		})
		require.NoError(t, err)

		// trigger cleanup of archived orders older than 12 hours
		require.NoError(t, service.CleanArchive(ctx, now.Add(-12*time.Hour)))

		archived, err := node.DB.Orders().ListArchived(ctx, 10)
		require.NoError(t, err)

		require.Len(t, archived, 1)
		require.Equal(t, archived[0].Limit.SerialNumber, serialNumber1)
	})
}

func TestCleanArchiveFileStore(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 1, UplinkCount: 0,
		Reconfigure: testplanet.Reconfigure{
			StorageNode: func(_ int, config *storagenode.Config) {
				// A large grace period so we can write to multiple buckets at once
				config.Storage2.OrderLimitGracePeriod = 48 * time.Hour
			},
		},
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		planet.Satellites[0].Audit.Worker.Loop.Pause()
		satellite := planet.Satellites[0].ID()
		node := planet.StorageNodes[0]
		service := node.Storage2.Orders
		service.Sender.Pause()
		service.Cleanup.Pause()
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)

		serialNumber0 := testrand.SerialNumber()
		createdAt0 := now
		serialNumber1 := testrand.SerialNumber()
		createdAt1 := now.Add(-24 * time.Hour)

		order0 := &ordersfile.Info{
			Limit: &pb.OrderLimit{
				SatelliteId:   satellite,
				SerialNumber:  serialNumber0,
				OrderCreation: createdAt0,
			},
			Order: &pb.Order{},
		}
		order1 := &ordersfile.Info{
			Limit: &pb.OrderLimit{
				SatelliteId:   satellite,
				SerialNumber:  serialNumber1,
				OrderCreation: createdAt1,
			},
			Order: &pb.Order{},
		}

		// enqueue both orders; they will be placed in separate buckets because they have different creation hours
		err := node.OrdersStore.Enqueue(order0)
		require.NoError(t, err)
		err = node.OrdersStore.Enqueue(order1)
		require.NoError(t, err)

		// archive one order yesterday, one today
		unsentInfo := orders.UnsentInfo{Version: ordersfile.V1}
		unsentInfo.CreatedAtHour = createdAt0.Truncate(time.Hour)
		err = node.OrdersStore.Archive(satellite, unsentInfo, yesterday, pb.SettlementWithWindowResponse_ACCEPTED)
		require.NoError(t, err)
		unsentInfo.CreatedAtHour = createdAt1.Truncate(time.Hour)
		err = node.OrdersStore.Archive(satellite, unsentInfo, now, pb.SettlementWithWindowResponse_ACCEPTED)
		require.NoError(t, err)

		archived, err := node.OrdersStore.ListArchived()
		require.NoError(t, err)
		require.Len(t, archived, 2)

		// trigger cleanup of archived orders older than 12 hours
		require.NoError(t, service.CleanArchive(ctx, now.Add(-12*time.Hour)))

		archived, err = node.OrdersStore.ListArchived()
		require.NoError(t, err)

		require.Len(t, archived, 1)
		require.Equal(t, archived[0].Limit.SerialNumber, serialNumber1)
	})
}
