// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package message

import (
	"go.uber.org/zap"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/logging"
)

var _ Handler = NoopHandler{}

type Handler interface {
	HandleTx(nodeID ids.NodeID, requestID uint32, msg *Tx) error
}

type NoopHandler struct {
	Log logging.Logger
}

func (h NoopHandler) HandleTx(nodeID ids.NodeID, requestID uint32, _ *Tx) error {
	h.Log.Debug("dropping unexpected Tx message",
		zap.Stringer("nodeID", nodeID),
		zap.Uint32("requestID", requestID),
	)
	return nil
}
