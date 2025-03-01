// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txheap

import (
	"time"

	"github.com/kukrer/savannahnode/vms/platformvm/txs"
)

var _ TimedHeap = &byStartTime{}

type TimedHeap interface {
	Heap

	Timestamp() time.Time
}

type byStartTime struct {
	txHeap
}

func NewByStartTime() TimedHeap {
	h := &byStartTime{}
	h.initialize(h)
	return h
}

func (h *byStartTime) Less(i, j int) bool {
	iTime := h.txs[i].tx.Unsigned.(txs.StakerTx).StartTime()
	jTime := h.txs[j].tx.Unsigned.(txs.StakerTx).StartTime()
	return iTime.Before(jTime)
}

func (h *byStartTime) Timestamp() time.Time {
	return h.Peek().Unsigned.(txs.StakerTx).StartTime()
}
