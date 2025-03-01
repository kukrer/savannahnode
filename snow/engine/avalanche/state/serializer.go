// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package state manages the meta-data required by consensus for an avalanche
// dag.
package state

import (
	"errors"
	"time"

	"github.com/kukrer/savannahnode/cache"
	"github.com/kukrer/savannahnode/database"
	"github.com/kukrer/savannahnode/database/versiondb"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/snow/choices"
	"github.com/kukrer/savannahnode/snow/consensus/avalanche"
	"github.com/kukrer/savannahnode/snow/consensus/snowstorm"
	"github.com/kukrer/savannahnode/snow/engine/avalanche/vertex"
	"github.com/kukrer/savannahnode/utils/logging"
	"github.com/kukrer/savannahnode/utils/math"
)

const (
	dbCacheSize = 10000
	idCacheSize = 1000
)

var (
	errUnknownVertex = errors.New("unknown vertex")
	errWrongChainID  = errors.New("wrong ChainID in vertex")
)

var _ vertex.Manager = &Serializer{}

// Serializer manages the state of multiple vertices
type Serializer struct {
	SerializerConfig
	versionDB *versiondb.Database
	state     *prefixedState
	edge      ids.Set
}

type SerializerConfig struct {
	ChainID             ids.ID
	VM                  vertex.DAGVM
	DB                  database.Database
	Log                 logging.Logger
	XChainMigrationTime time.Time
}

func NewSerializer(config SerializerConfig) vertex.Manager {
	versionDB := versiondb.New(config.DB)
	dbCache := &cache.LRU{Size: dbCacheSize}
	s := Serializer{
		SerializerConfig: config,
		versionDB:        versionDB,
	}

	rawState := &state{
		serializer: &s,
		log:        config.Log,
		dbCache:    dbCache,
		db:         versionDB,
	}

	s.state = newPrefixedState(rawState, idCacheSize)
	s.edge.Add(s.state.Edge()...)

	return &s
}

func (s *Serializer) ParseVtx(b []byte) (avalanche.Vertex, error) {
	return newUniqueVertex(s, b)
}

func (s *Serializer) BuildVtx(parentIDs []ids.ID, txs []snowstorm.Tx) (avalanche.Vertex, error) {
	return s.buildVtx(parentIDs, txs, false)
}

func (s *Serializer) BuildStopVtx(parentIDs []ids.ID) (avalanche.Vertex, error) {
	return s.buildVtx(parentIDs, nil, true)
}

func (s *Serializer) buildVtx(
	parentIDs []ids.ID,
	txs []snowstorm.Tx,
	stopVtx bool,
) (avalanche.Vertex, error) {
	height := uint64(0)
	for _, parentID := range parentIDs {
		parent, err := s.getUniqueVertex(parentID)
		if err != nil {
			return nil, err
		}
		parentHeight := parent.v.vtx.Height()
		childHeight, err := math.Add64(parentHeight, 1)
		if err != nil {
			return nil, err
		}
		height = math.Max64(height, childHeight)
	}

	var (
		vtx vertex.StatelessVertex
		err error
	)
	if !stopVtx {
		txBytes := make([][]byte, len(txs))
		for i, tx := range txs {
			txBytes[i] = tx.Bytes()
		}
		vtx, err = vertex.Build(
			s.ChainID,
			height,
			parentIDs,
			txBytes,
		)
	} else {
		vtx, err = vertex.BuildStopVertex(
			s.ChainID,
			height,
			parentIDs,
		)
	}
	if err != nil {
		return nil, err
	}

	uVtx := &uniqueVertex{
		serializer: s,
		id:         vtx.ID(),
	}
	// setVertex handles the case where this vertex already exists even
	// though we just made it
	return uVtx, uVtx.setVertex(vtx)
}

func (s *Serializer) GetVtx(vtxID ids.ID) (avalanche.Vertex, error) {
	return s.getUniqueVertex(vtxID)
}

func (s *Serializer) Edge() []ids.ID { return s.edge.List() }

func (s *Serializer) parseVertex(b []byte) (vertex.StatelessVertex, error) {
	vtx, err := vertex.Parse(b)
	if err != nil {
		return nil, err
	}
	if vtx.ChainID() != s.ChainID {
		return nil, errWrongChainID
	}
	return vtx, nil
}

func (s *Serializer) getUniqueVertex(vtxID ids.ID) (*uniqueVertex, error) {
	vtx := &uniqueVertex{
		serializer: s,
		id:         vtxID,
	}
	if vtx.Status() == choices.Unknown {
		return nil, errUnknownVertex
	}
	return vtx, nil
}

func (s *Serializer) StopVertexAccepted() (bool, error) {
	edge := s.Edge()
	if len(edge) != 1 {
		return false, nil
	}

	vtx, err := s.getUniqueVertex(edge[0])
	if err != nil {
		return false, err
	}

	return vtx.v.vtx.StopVertex(), nil
}
