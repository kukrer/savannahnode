// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
	"net/http"
	"time"

	"github.com/gorilla/rpc/v2"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/vms/platformvm/blocks"
)

var Noop Metrics = noopMetrics{}

type noopMetrics struct{}

func (noopMetrics) MarkOptionVoteWon() {}

func (noopMetrics) MarkOptionVoteLost() {}

func (noopMetrics) MarkAccepted(blocks.Block) error { return nil }

func (noopMetrics) InterceptRequest(i *rpc.RequestInfo) *http.Request {
	return i.Request
}

func (noopMetrics) AfterRequest(*rpc.RequestInfo) {}

func (noopMetrics) IncValidatorSetsCreated() {}

func (noopMetrics) IncValidatorSetsCached() {}

func (noopMetrics) AddValidatorSetsDuration(time.Duration) {}

func (noopMetrics) AddValidatorSetsHeightDiff(uint64) {}

func (noopMetrics) SetLocalStake(uint64) {}

func (noopMetrics) SetTotalStake(uint64) {}

func (noopMetrics) SetSubnetPercentConnected(ids.ID, float64) {}

func (noopMetrics) SetPercentConnected(float64) {}
