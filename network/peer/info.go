// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package peer

import (
	"time"

	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/utils/json"
)

type Info struct {
	IP             string     `json:"ip"`
	PublicIP       string     `json:"publicIP,omitempty"`
	ID             ids.NodeID `json:"nodeID"`
	Version        string     `json:"version"`
	LastSent       time.Time  `json:"lastSent"`
	LastReceived   time.Time  `json:"lastReceived"`
	ObservedUptime json.Uint8 `json:"observedUptime"`
	TrackedSubnets []ids.ID   `json:"trackedSubnets"`
}
