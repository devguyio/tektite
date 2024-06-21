// Copyright 2024 The Tektite Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sequence

import (
	"fmt"
	"github.com/spirit-labs/tektite/lock"
	"github.com/spirit-labs/tektite/objstore/dev"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

const sequencesBatchSize = 10
const unavailabilityRetryDelay = 1 * time.Millisecond

func TestSingleSequence(t *testing.T) {
	lockMgr := lock.NewInMemLockManager()
	objStore := dev.NewInMemStore(0)
	mgr := NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)
	for i := 0; i < 10*sequencesBatchSize; i++ {
		seq, err := mgr.GetNextID("test_sequence", sequencesBatchSize)
		require.NoError(t, err)
		require.Equal(t, i, seq)
	}

	// Recreate so state gets reloaded
	mgr = NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	for i := 0; i < 10*sequencesBatchSize; i++ {
		seq, err := mgr.GetNextID("test_sequence", sequencesBatchSize)
		require.NoError(t, err)
		require.Equal(t, 10*sequencesBatchSize+i, seq)
	}

}

func TestMultipleSequences(t *testing.T) {
	lockMgr := lock.NewInMemLockManager()
	objStore := dev.NewInMemStore(0)
	mgr := NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	for i := 0; i < 10; i++ {
		sequenceName := fmt.Sprintf("sequence-%d", i)
		for j := 0; j < 10*sequencesBatchSize; j++ {
			seq, err := mgr.GetNextID(sequenceName, sequencesBatchSize)
			require.NoError(t, err)
			require.Equal(t, j, seq)
		}
	}

	mgr = NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	// Reload state
	for i := 0; i < 10; i++ {
		sequenceName := fmt.Sprintf("sequence-%d", i)
		for j := 0; j < 10*sequencesBatchSize; j++ {
			seq, err := mgr.GetNextID(sequenceName, sequencesBatchSize)
			require.NoError(t, err)
			require.Equal(t, 10*sequencesBatchSize+j, seq)
		}
	}

}

func TestSequenceBatchSize(t *testing.T) {
	lockMgr := lock.NewInMemLockManager()
	objStore := dev.NewInMemStore(0)
	mgr := NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	seq, err := mgr.GetNextID("test_sequence", sequencesBatchSize)
	require.NoError(t, err)
	require.Equal(t, 0, seq)

	// Recreate so state gets reloaded
	mgr = NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	seq, err = mgr.GetNextID("test_sequence", sequencesBatchSize)
	require.NoError(t, err)
	require.Equal(t, sequencesBatchSize, seq)
}

func TestConcurrentGets(t *testing.T) {
	lockMgr := lock.NewInMemLockManager()
	objStore := dev.NewInMemStore(0)
	var seqs1 sync.Map
	// Note unavailabilityRetryDelay is set to a low value so the different managers gets coincide more
	mgr1 := NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)
	var seqs2 sync.Map
	mgr2 := NewSequenceManager(objStore, "sequences_obj", lockMgr, unavailabilityRetryDelay)

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for i := 0; i < 1000; i++ {
			seq, err := mgr1.GetNextID("test_sequence", sequencesBatchSize)
			if err != nil {
				panic(err)
			}
			seqs1.Store(seq, struct{}{})
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			seq, err := mgr2.GetNextID("test_sequence", sequencesBatchSize)
			if err != nil {
				panic(err)
			}
			seqs2.Store(seq, struct{}{})
		}
		wg.Done()
	}()

	wg.Wait()
	// Should be no overlap
	overlap := false
	seqs1.Range(func(key, value any) bool {
		_, ok := seqs2.Load(key)
		if ok {
			overlap = true
			return false
		}
		return true
	})
	require.False(t, overlap)

	overlap = false
	seqs2.Range(func(key, value any) bool {
		_, ok := seqs1.Load(key)
		if ok {
			overlap = true
			return false
		}
		return true
	})
	require.False(t, overlap)
}

func TestCloudStoreUnavailable(t *testing.T) {
	lockMgr := lock.NewInMemLockManager()
	store := dev.NewInMemStore(0)
	mgr := NewSequenceManager(store, "sequences_obj", lockMgr, unavailabilityRetryDelay)
	for i := 0; i < 10*sequencesBatchSize; i++ {
		seq, err := mgr.GetNextID("test_sequence", sequencesBatchSize)
		require.NoError(t, err)
		require.Equal(t, i, seq)
	}

	store.SetUnavailable(true)
	time.AfterFunc(1*time.Second, func() {
		store.SetUnavailable(false)
	})

	for i := 0; i < 10*sequencesBatchSize; i++ {
		seq, err := mgr.GetNextID("test_sequence", sequencesBatchSize)
		require.NoError(t, err)
		require.Equal(t, 10*sequencesBatchSize+i, seq)
	}

}
