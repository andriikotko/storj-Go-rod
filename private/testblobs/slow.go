// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package testblobs

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"storj.io/common/storj"
	"storj.io/storj/storage"
	"storj.io/storj/storagenode"
)

// SlowDB implements slow storage node DB.
type SlowDB struct {
	storagenode.DB
	blobs *SlowBlobs
	log   *zap.Logger
}

// NewSlowDB creates a new slow storage node DB wrapping the provided db.
// Use SetLatency to dynamically configure the latency of all piece operations.
func NewSlowDB(log *zap.Logger, db storagenode.DB) *SlowDB {
	return &SlowDB{
		DB:    db,
		blobs: newSlowBlobs(log, db.Pieces()),
		log:   log,
	}
}

// Pieces returns the blob store.
func (slow *SlowDB) Pieces() storage.Blobs {
	return slow.blobs
}

// SetLatency enables a sleep for delay duration for all piece operations.
// A zero or negative delay means no sleep.
func (slow *SlowDB) SetLatency(delay time.Duration) {
	slow.blobs.SetLatency(delay)
}

// SlowBlobs implements a slow blob store.
type SlowBlobs struct {
	delay int64 // time.Duration
	blobs storage.Blobs
	log   *zap.Logger
}

// newSlowBlobs creates a new slow blob store wrapping the provided blobs.
// Use SetLatency to dynamically configure the latency of all operations.
func newSlowBlobs(log *zap.Logger, blobs storage.Blobs) *SlowBlobs {
	return &SlowBlobs{
		log:   log,
		blobs: blobs,
	}
}

// Create creates a new blob that can be written optionally takes a size
// argument for performance improvements, -1 is unknown size.
func (slow *SlowBlobs) Create(ctx context.Context, ref storage.BlobRef, size int64) (storage.BlobWriter, error) {
	slow.sleep()
	return slow.blobs.Create(ctx, ref, size)
}

// Close closes the blob store and any resources associated with it.
func (slow *SlowBlobs) Close() error {
	return slow.blobs.Close()
}

// Open opens a reader with the specified namespace and key.
func (slow *SlowBlobs) Open(ctx context.Context, ref storage.BlobRef) (storage.BlobReader, error) {
	slow.sleep()
	return slow.blobs.Open(ctx, ref)
}

// OpenWithStorageFormat opens a reader for the already-located blob, avoiding the potential need
// to check multiple storage formats to find the blob.
func (slow *SlowBlobs) OpenWithStorageFormat(ctx context.Context, ref storage.BlobRef, formatVer storage.FormatVersion) (storage.BlobReader, error) {
	slow.sleep()
	return slow.blobs.OpenWithStorageFormat(ctx, ref, formatVer)
}

// Trash deletes the blob with the namespace and key.
func (slow *SlowBlobs) Trash(ctx context.Context, ref storage.BlobRef) error {
	slow.sleep()
	return slow.blobs.Trash(ctx, ref)
}

// RestoreTrash restores all files in the trash.
func (slow *SlowBlobs) RestoreTrash(ctx context.Context, namespace []byte) ([][]byte, error) {
	slow.sleep()
	return slow.blobs.RestoreTrash(ctx, namespace)
}

// EmptyTrash empties the trash.
func (slow *SlowBlobs) EmptyTrash(ctx context.Context, namespace []byte, trashedBefore time.Time) (int64, [][]byte, error) {
	slow.sleep()
	return slow.blobs.EmptyTrash(ctx, namespace, trashedBefore)
}

// Delete deletes the blob with the namespace and key.
func (slow *SlowBlobs) Delete(ctx context.Context, ref storage.BlobRef) error {
	slow.sleep()
	return slow.blobs.Delete(ctx, ref)
}

// DeleteWithStorageFormat deletes the blob with the namespace, key, and format version.
func (slow *SlowBlobs) DeleteWithStorageFormat(ctx context.Context, ref storage.BlobRef, formatVer storage.FormatVersion) error {
	slow.sleep()
	return slow.blobs.DeleteWithStorageFormat(ctx, ref, formatVer)
}

// DeleteNamespace deletes blobs of specific satellite, used after successful GE only.
func (slow *SlowBlobs) DeleteNamespace(ctx context.Context, ref []byte) (err error) {
	slow.sleep()
	return slow.blobs.DeleteNamespace(ctx, ref)
}

// Stat looks up disk metadata on the blob file.
func (slow *SlowBlobs) Stat(ctx context.Context, ref storage.BlobRef) (storage.BlobInfo, error) {
	slow.sleep()
	return slow.blobs.Stat(ctx, ref)
}

// StatWithStorageFormat looks up disk metadata for the blob file with the given storage format
// version. This avoids the potential need to check multiple storage formats for the blob
// when the format is already known.
func (slow *SlowBlobs) StatWithStorageFormat(ctx context.Context, ref storage.BlobRef, formatVer storage.FormatVersion) (storage.BlobInfo, error) {
	slow.sleep()
	return slow.blobs.StatWithStorageFormat(ctx, ref, formatVer)
}

// WalkNamespace executes walkFunc for each locally stored blob in the given namespace.
// If walkFunc returns a non-nil error, WalkNamespace will stop iterating and return the
// error immediately.
func (slow *SlowBlobs) WalkNamespace(ctx context.Context, namespace []byte, walkFunc func(storage.BlobInfo) error) error {
	slow.sleep()
	return slow.blobs.WalkNamespace(ctx, namespace, walkFunc)
}

// ListNamespaces returns all namespaces that might be storing data.
func (slow *SlowBlobs) ListNamespaces(ctx context.Context) ([][]byte, error) {
	return slow.blobs.ListNamespaces(ctx)
}

// FreeSpace return how much free space left for writing.
func (slow *SlowBlobs) FreeSpace() (int64, error) {
	slow.sleep()
	return slow.blobs.FreeSpace()
}

// CheckWritability tests writability of the storage directory by creating and deleting a file.
func (slow *SlowBlobs) CheckWritability() error {
	slow.sleep()
	return slow.blobs.CheckWritability()
}

// SpaceUsedForBlobs adds up how much is used in all namespaces.
func (slow *SlowBlobs) SpaceUsedForBlobs(ctx context.Context) (int64, error) {
	slow.sleep()
	return slow.blobs.SpaceUsedForBlobs(ctx)
}

// SpaceUsedForBlobsInNamespace adds up how much is used in the given namespace.
func (slow *SlowBlobs) SpaceUsedForBlobsInNamespace(ctx context.Context, namespace []byte) (int64, error) {
	slow.sleep()
	return slow.blobs.SpaceUsedForBlobsInNamespace(ctx, namespace)
}

// SpaceUsedForTrash adds up how much is used in all namespaces.
func (slow *SlowBlobs) SpaceUsedForTrash(ctx context.Context) (int64, error) {
	slow.sleep()
	return slow.blobs.SpaceUsedForTrash(ctx)
}

// CreateVerificationFile creates a file to be used for storage directory verification.
func (slow *SlowBlobs) CreateVerificationFile(id storj.NodeID) error {
	slow.sleep()
	return slow.blobs.CreateVerificationFile(id)
}

// VerifyStorageDir verifies that the storage directory is correct by checking for the existence and validity
// of the verification file.
func (slow *SlowBlobs) VerifyStorageDir(id storj.NodeID) error {
	return slow.blobs.VerifyStorageDir(id)
}

// SetLatency configures the blob store to sleep for delay duration for all
// operations. A zero or negative delay means no sleep.
func (slow *SlowBlobs) SetLatency(delay time.Duration) {
	atomic.StoreInt64(&slow.delay, int64(delay))
}

// sleep sleeps for the duration set to slow.delay.
func (slow *SlowBlobs) sleep() {
	delay := time.Duration(atomic.LoadInt64(&slow.delay))
	time.Sleep(delay)
}
