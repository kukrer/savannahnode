// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/stretchr/testify/require"

	"github.com/kukrer/savannahnode/database/leveldb"
	"github.com/kukrer/savannahnode/database/memdb"
	"github.com/kukrer/savannahnode/database/meterdb"
	"github.com/kukrer/savannahnode/database/prefixdb"
	"github.com/kukrer/savannahnode/utils/logging"
	"github.com/kukrer/savannahnode/version"
)

func TestNewSingleLevelDB(t *testing.T) {
	dir := t.TempDir()

	v1 := version.Semantic1_0_0

	dbPath := filepath.Join(dir, v1.String())
	db, err := leveldb.New(dbPath, nil, logging.NoLog{}, "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewLevelDB(dir, nil, logging.NoLog{}, v1, "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	semDB := manager.Current()
	cmp := semDB.Version.Compare(v1)
	require.Equal(t, 0, cmp, "incorrect version on current database")

	_, exists := manager.Previous()
	require.False(t, exists, "there should be no previous database")

	dbs := manager.GetDatabases()
	require.Len(t, dbs, 1)

	err = manager.Close()
	require.NoError(t, err)
}

func TestNewCreatesSingleDB(t *testing.T) {
	dir := t.TempDir()

	v1 := version.Semantic1_0_0

	manager, err := NewLevelDB(dir, nil, logging.NoLog{}, v1, "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	semDB := manager.Current()
	cmp := semDB.Version.Compare(v1)
	require.Equal(t, 0, cmp, "incorrect version on current database")

	_, exists := manager.Previous()
	require.False(t, exists, "there should be no previous database")

	dbs := manager.GetDatabases()
	require.Len(t, dbs, 1)

	err = manager.Close()
	require.NoError(t, err)
}

func TestNewInvalidMemberPresent(t *testing.T) {
	dir := t.TempDir()

	v1 := &version.Semantic{
		Major: 1,
		Minor: 1,
		Patch: 0,
	}
	v2 := &version.Semantic{
		Major: 1,
		Minor: 2,
		Patch: 0,
	}

	dbPath1 := filepath.Join(dir, v1.String())
	db1, err := leveldb.New(dbPath1, nil, logging.NoLog{}, "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	dbPath2 := filepath.Join(dir, v2.String())
	db2, err := leveldb.New(dbPath2, nil, logging.NoLog{}, "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	err = db2.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewLevelDB(dir, nil, logging.NoLog{}, v2, "", prometheus.NewRegistry())
	require.Error(t, err, "expected to error creating the manager due to an open db")

	err = db1.Close()
	require.NoError(t, err)

	f, err := os.Create(filepath.Join(dir, "dummy"))
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)

	db, err := NewLevelDB(dir, nil, logging.NoLog{}, v1, "", prometheus.NewRegistry())
	require.NoError(t, err, "expected not to error with a non-directory file being present")

	err = db.Close()
	require.NoError(t, err)
}

func TestNewSortsDatabases(t *testing.T) {
	dir := t.TempDir()

	vers := []*version.Semantic{
		{
			Major: 2,
			Minor: 1,
			Patch: 2,
		},
		{
			Major: 2,
			Minor: 0,
			Patch: 2,
		},
		{
			Major: 1,
			Minor: 3,
			Patch: 2,
		},
		{
			Major: 1,
			Minor: 0,
			Patch: 2,
		},
		{
			Major: 1,
			Minor: 0,
			Patch: 1,
		},
	}

	for _, version := range vers {
		dbPath := filepath.Join(dir, version.String())
		db, err := leveldb.New(dbPath, nil, logging.NoLog{}, "", prometheus.NewRegistry())
		if err != nil {
			t.Fatal(err)
		}

		err = db.Close()
		if err != nil {
			t.Fatal(err)
		}
	}

	manager, err := NewLevelDB(dir, nil, logging.NoLog{}, vers[0], "", prometheus.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = manager.Close()
		require.NoError(t, err, "problem closing database manager")
	}()

	semDB := manager.Current()
	cmp := semDB.Version.Compare(vers[0])
	require.Equal(t, 0, cmp, "incorrect version on current database")

	prev, exists := manager.Previous()
	if !exists {
		t.Fatal("expected to find a previous database")
	}
	cmp = prev.Version.Compare(vers[1])
	require.Equal(t, 0, cmp, "incorrect version on previous database")

	dbs := manager.GetDatabases()
	if len(dbs) != len(vers) {
		t.Fatalf("Expected to find %d databases, but found %d", len(vers), len(dbs))
	}

	for i, db := range dbs {
		cmp = db.Version.Compare(vers[i])
		require.Equal(t, 0, cmp, "expected to find database version %s, but found %s", vers[i], db.Version.String())
	}
}

func TestPrefixDBManager(t *testing.T) {
	db := memdb.New()

	prefix0 := []byte{0}
	db0 := prefixdb.New(prefix0, db)

	prefix1 := []byte{1}
	db1 := prefixdb.New(prefix1, db0)

	k0 := []byte{'s', 'c', 'h', 'n', 'i'}
	v0 := []byte{'t', 'z', 'e', 'l'}
	k1 := []byte{'c', 'u', 'r', 'r', 'y'}
	v1 := []byte{'w', 'u', 'r', 's', 't'}

	require.NoError(t, db0.Put(k0, v0))
	require.NoError(t, db1.Put(k1, v1))
	require.NoError(t, db0.Close())
	require.NoError(t, db1.Close())

	m := &manager{databases: []*VersionedDatabase{
		{
			Database: db,
			Version:  version.Semantic1_0_0,
		},
	}}

	m0 := m.NewPrefixDBManager(prefix0)
	m1 := m0.NewPrefixDBManager(prefix1)

	val, err := m0.Current().Database.Get(k0)
	require.NoError(t, err)
	require.Equal(t, v0, val)

	val, err = m1.Current().Database.Get(k1)
	require.NoError(t, err)
	require.Equal(t, v1, val)
}

func TestNestedPrefixDBManager(t *testing.T) {
	db := memdb.New()

	prefix0 := []byte{0}
	db0 := prefixdb.NewNested(prefix0, db)

	prefix1 := []byte{1}
	db1 := prefixdb.NewNested(prefix1, db0)

	k0 := []byte{'s', 'c', 'h', 'n', 'i'}
	v0 := []byte{'t', 'z', 'e', 'l'}
	k1 := []byte{'c', 'u', 'r', 'r', 'y'}
	v1 := []byte{'w', 'u', 'r', 's', 't'}

	require.NoError(t, db0.Put(k0, v0))
	require.NoError(t, db1.Put(k1, v1))
	require.NoError(t, db0.Close())
	require.NoError(t, db1.Close())

	m := &manager{databases: []*VersionedDatabase{
		{
			Database: db,
			Version:  version.Semantic1_0_0,
		},
	}}

	m0 := m.NewNestedPrefixDBManager(prefix0)
	m1 := m0.NewNestedPrefixDBManager(prefix1)

	val, err := m0.Current().Database.Get(k0)
	require.NoError(t, err)
	require.Equal(t, v0, val)

	val, err = m1.Current().Database.Get(k1)
	require.NoError(t, err)
	require.Equal(t, v1, val)
}

func TestMeterDBManager(t *testing.T) {
	registry := prometheus.NewRegistry()

	m := &manager{databases: []*VersionedDatabase{
		{
			Database: memdb.New(),
			Version: &version.Semantic{
				Major: 2,
				Minor: 0,
				Patch: 0,
			},
		},
		{
			Database: memdb.New(),
			Version: &version.Semantic{
				Major: 1,
				Minor: 5,
				Patch: 0,
			},
		},
		{
			Database: memdb.New(),
			Version:  version.Semantic1_0_0,
		},
	}}

	// Create meterdb manager with fresh registry and confirm
	// that there are no errors registering metrics for multiple
	// versioned databases.
	manager, err := m.NewMeterDBManager("", registry)
	require.NoError(t, err)

	dbs := manager.GetDatabases()
	require.Len(t, dbs, 3)

	_, ok := dbs[0].Database.(*meterdb.Database)
	require.True(t, ok)
	_, ok = dbs[1].Database.(*meterdb.Database)
	require.False(t, ok)
	_, ok = dbs[2].Database.(*meterdb.Database)
	require.False(t, ok)

	// Confirm that the error from a name conflict is handled correctly
	_, err = m.NewMeterDBManager("", registry)
	require.Error(t, err)
}

func TestCompleteMeterDBManager(t *testing.T) {
	registry := prometheus.NewRegistry()

	m := &manager{databases: []*VersionedDatabase{
		{
			Database: memdb.New(),
			Version: &version.Semantic{
				Major: 2,
				Minor: 0,
				Patch: 0,
			},
		},
		{
			Database: memdb.New(),
			Version: &version.Semantic{
				Major: 1,
				Minor: 5,
				Patch: 0,
			},
		},
		{
			Database: memdb.New(),
			Version:  version.Semantic1_0_0,
		},
	}}

	// Create complete meterdb manager with fresh registry and confirm
	// that there are no errors registering metrics for multiple
	// versioned databases.
	manager, err := m.NewCompleteMeterDBManager("", registry)
	require.NoError(t, err)

	dbs := manager.GetDatabases()
	require.Len(t, dbs, 3)

	_, ok := dbs[0].Database.(*meterdb.Database)
	require.True(t, ok)
	_, ok = dbs[1].Database.(*meterdb.Database)
	require.True(t, ok)
	_, ok = dbs[2].Database.(*meterdb.Database)
	require.True(t, ok)

	// Confirm that the error from a name conflict is handled correctly
	_, err = m.NewCompleteMeterDBManager("", registry)
	require.Error(t, err)
}

func TestNewManagerFromDBs(t *testing.T) {
	versions := []*version.Semantic{
		{
			Major: 3,
			Minor: 2,
			Patch: 0,
		},
		{
			Major: 1,
			Minor: 2,
			Patch: 0,
		},
		{
			Major: 1,
			Minor: 1,
			Patch: 1,
		},
	}
	m, err := NewManagerFromDBs(
		[]*VersionedDatabase{
			{
				Database: memdb.New(),
				Version:  versions[2],
			},
			{
				Database: memdb.New(),
				Version:  versions[1],
			},
			{
				Database: memdb.New(),
				Version:  versions[0],
			},
		})
	require.NoError(t, err)

	dbs := m.GetDatabases()
	require.Len(t, dbs, len(versions))
	for i, db := range dbs {
		require.Equal(t, 0, db.Version.Compare(versions[i]))
	}
}

func TestNewManagerFromNoDBs(t *testing.T) {
	// Should error if no dbs are given
	_, err := NewManagerFromDBs(nil)
	require.Error(t, err)
}

func TestNewManagerFromNonUniqueDBs(t *testing.T) {
	_, err := NewManagerFromDBs(
		[]*VersionedDatabase{
			{
				Database: memdb.New(),
				Version: &version.Semantic{
					Major: 1,
					Minor: 1,
					Patch: 0,
				},
			},
			{
				Database: memdb.New(),
				Version: &version.Semantic{
					Major: 1,
					Minor: 1,
					Patch: 0,
				}, // Duplicate
			},
			{
				Database: memdb.New(),
				Version: &version.Semantic{
					Major: 1,
					Minor: 2,
					Patch: 0,
				},
			},
		})
	require.Error(t, err)
}
