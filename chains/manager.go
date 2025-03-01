// Copyright (C) 2019-2021, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chains

import (
	"crypto"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"go.uber.org/zap"

	"github.com/kukrer/savannahnode/api/health"
	"github.com/kukrer/savannahnode/api/keystore"
	"github.com/kukrer/savannahnode/api/metrics"
	"github.com/kukrer/savannahnode/api/server"
	"github.com/kukrer/savannahnode/chains/atomic"
	"github.com/kukrer/savannahnode/database/prefixdb"
	"github.com/kukrer/savannahnode/ids"
	"github.com/kukrer/savannahnode/message"
	"github.com/kukrer/savannahnode/network"
	"github.com/kukrer/savannahnode/snow"
	"github.com/kukrer/savannahnode/snow/consensus/snowball"
	"github.com/kukrer/savannahnode/snow/engine/avalanche/state"
	"github.com/kukrer/savannahnode/snow/engine/avalanche/vertex"
	"github.com/kukrer/savannahnode/snow/engine/common"
	"github.com/kukrer/savannahnode/snow/engine/common/queue"
	"github.com/kukrer/savannahnode/snow/engine/common/tracker"
	"github.com/kukrer/savannahnode/snow/engine/snowman/block"
	"github.com/kukrer/savannahnode/snow/engine/snowman/syncer"
	"github.com/kukrer/savannahnode/snow/networking/handler"
	"github.com/kukrer/savannahnode/snow/networking/router"
	"github.com/kukrer/savannahnode/snow/networking/sender"
	"github.com/kukrer/savannahnode/snow/networking/timeout"
	"github.com/kukrer/savannahnode/snow/validators"
	"github.com/kukrer/savannahnode/utils/constants"
	"github.com/kukrer/savannahnode/utils/logging"
	"github.com/kukrer/savannahnode/version"
	"github.com/kukrer/savannahnode/vms"
	"github.com/kukrer/savannahnode/vms/metervm"
	"github.com/kukrer/savannahnode/vms/proposervm"

	dbManager "github.com/kukrer/savannahnode/database/manager"
	timetracker "github.com/kukrer/savannahnode/snow/networking/tracker"

	avcon "github.com/kukrer/savannahnode/snow/consensus/avalanche"
	aveng "github.com/kukrer/savannahnode/snow/engine/avalanche"
	avbootstrap "github.com/kukrer/savannahnode/snow/engine/avalanche/bootstrap"
	avagetter "github.com/kukrer/savannahnode/snow/engine/avalanche/getter"

	smcon "github.com/kukrer/savannahnode/snow/consensus/snowman"
	smeng "github.com/kukrer/savannahnode/snow/engine/snowman"
	smbootstrap "github.com/kukrer/savannahnode/snow/engine/snowman/bootstrap"
	snowgetter "github.com/kukrer/savannahnode/snow/engine/snowman/getter"
)

const defaultChannelSize = 1

var (
	errUnknownChainID   = errors.New("unknown chain ID")
	errUnknownVMType    = errors.New("the vm should have type avalanche.DAGVM or snowman.ChainVM")
	errCreatePlatformVM = errors.New("attempted to create a chain running the PlatformVM")
	errNotBootstrapped  = errors.New("chains not bootstrapped")

	_ Manager = &manager{}
)

// Manager manages the chains running on this node.
// It can:
//   * Create a chain
//   * Add a registrant. When a chain is created, each registrant calls
//     RegisterChain with the new chain as the argument.
//   * Manage the aliases of chains
type Manager interface {
	ids.Aliaser

	// Return the router this Manager is using to route consensus messages to chains
	Router() router.Router

	// Create a chain in the future
	CreateChain(ChainParameters)

	// Create a chain now
	ForceCreateChain(ChainParameters)

	// Add a registrant [r]. Every time a chain is
	// created, [r].RegisterChain([new chain]) is called.
	AddRegistrant(Registrant)

	// Given an alias, return the ID of the chain associated with that alias
	Lookup(string) (ids.ID, error)

	// Given an alias, return the ID of the VM associated with that alias
	LookupVM(string) (ids.ID, error)

	// Returns the ID of the subnet that is validating the provided chain
	SubnetID(chainID ids.ID) (ids.ID, error)

	// Returns true iff the chain with the given ID exists and is finished bootstrapping
	IsBootstrapped(ids.ID) bool

	Shutdown()
}

// ChainParameters defines the chain being created
type ChainParameters struct {
	// The ID of the chain being created.
	ID ids.ID
	// ID of the subnet that validates this chain.
	SubnetID ids.ID
	// The genesis data of this chain's ledger.
	GenesisData []byte
	// The ID of the vm this chain is running.
	VMID ids.ID
	// The IDs of the feature extensions this chain is running.
	FxIDs []ids.ID
	// Should only be set if the default beacons can't be used.
	CustomBeacons validators.Set
}

type chain struct {
	Name    string
	Engine  common.Engine
	Handler handler.Handler
	Beacons validators.Set
}

// ChainConfig is configuration settings for the current execution.
// [Config] is the user-provided config blob for the chain.
// [Upgrade] is a chain-specific blob for coordinating upgrades.
type ChainConfig struct {
	Config  []byte
	Upgrade []byte
}

type ManagerConfig struct {
	StakingEnabled              bool            // True iff the network has staking enabled
	StakingCert                 tls.Certificate // needed to sign snowman++ blocks
	Log                         logging.Logger
	LogFactory                  logging.Factory
	VMManager                   vms.Manager // Manage mappings from vm ID --> vm
	DecisionAcceptorGroup       snow.AcceptorGroup
	ConsensusAcceptorGroup      snow.AcceptorGroup
	DBManager                   dbManager.Manager
	MsgCreator                  message.Creator    // message creator, shared with network
	Router                      router.Router      // Routes incoming messages to the appropriate chain
	Net                         network.Network    // Sends consensus messages to other validators
	ConsensusParams             avcon.Parameters   // The consensus parameters (alpha, beta, etc.) for new chains
	Validators                  validators.Manager // Validators validating on this chain
	NodeID                      ids.NodeID         // The ID of this node
	NetworkID                   uint32             // ID of the network this node is connected to
	Server                      server.Server      // Handles HTTP API calls
	Keystore                    keystore.Keystore
	AtomicMemory                *atomic.Memory
	AVAXAssetID                 ids.ID
	XChainID                    ids.ID
	CriticalChains              ids.Set         // Chains that can't exit gracefully
	WhitelistedSubnets          ids.Set         // Subnets to validate
	TimeoutManager              timeout.Manager // Manages request timeouts when sending messages to other validators
	Health                      health.Registerer
	RetryBootstrap              bool                    // Should Bootstrap be retried
	RetryBootstrapWarnFrequency int                     // Max number of times to retry bootstrap before warning the node operator
	SubnetConfigs               map[ids.ID]SubnetConfig // ID -> SubnetConfig
	ChainConfigs                map[string]ChainConfig  // alias -> ChainConfig
	// ShutdownNodeFunc allows the chain manager to issue a request to shutdown the node
	ShutdownNodeFunc func(exitCode int)
	MeterVMEnabled   bool // Should each VM be wrapped with a MeterVM
	Metrics          metrics.MultiGatherer

	ConsensusGossipFrequency time.Duration

	GossipConfig sender.GossipConfig

	// Max Time to spend fetching a container and its
	// ancestors when responding to a GetAncestors
	BootstrapMaxTimeGetAncestors time.Duration
	// Max number of containers in an ancestors message sent by this node.
	BootstrapAncestorsMaxContainersSent int
	// This node will only consider the first [AncestorsMaxContainersReceived]
	// containers in an ancestors message it receives.
	BootstrapAncestorsMaxContainersReceived int

	ApricotPhase4Time            time.Time
	ApricotPhase4MinPChainHeight uint64

	// Tracks CPU/disk usage caused by each peer.
	ResourceTracker timetracker.ResourceTracker

	StateSyncBeacons []ids.NodeID
}

type manager struct {
	// Note: The string representation of a chain's ID is also considered to be an alias of the chain
	// That is, [chainID].String() is an alias for the chain, too
	ids.Aliaser
	ManagerConfig

	// Those notified when a chain is created
	registrants []Registrant

	unblocked     bool
	blockedChains []ChainParameters

	// Key: Subnet's ID
	// Value: Subnet description
	subnets map[ids.ID]Subnet

	chainsLock sync.Mutex
	// Key: Chain's ID
	// Value: The chain
	chains map[ids.ID]handler.Handler

	// snowman++ related interface to allow validators retrival
	validatorState validators.State
}

// New returns a new Manager
func New(config *ManagerConfig) Manager {
	return &manager{
		Aliaser:       ids.NewAliaser(),
		ManagerConfig: *config,
		subnets:       make(map[ids.ID]Subnet),
		chains:        make(map[ids.ID]handler.Handler),
	}
}

// Router that this chain manager is using to route consensus messages to chains
func (m *manager) Router() router.Router { return m.ManagerConfig.Router }

// Create a chain
func (m *manager) CreateChain(chain ChainParameters) {
	if !m.unblocked {
		m.blockedChains = append(m.blockedChains, chain)
	} else {
		m.ForceCreateChain(chain)
	}
}

// Create a chain, this is only called from the P-chain thread, except for
// creating the P-chain.
func (m *manager) ForceCreateChain(chainParams ChainParameters) {
	if m.StakingEnabled && chainParams.SubnetID != constants.PrimaryNetworkID && !m.WhitelistedSubnets.Contains(chainParams.SubnetID) {
		m.Log.Debug("skipped creating non-whitelisted chain",
			zap.Stringer("chainID", chainParams.ID),
			zap.Stringer("vmID", chainParams.VMID),
		)
		return
	}
	// Assert that there isn't already a chain with an alias in [chain].Aliases
	// (Recall that the string representation of a chain's ID is also an alias
	//  for a chain)
	if alias, isRepeat := m.isChainWithAlias(chainParams.ID.String()); isRepeat {
		m.Log.Debug("skipping chain creation",
			zap.String("reason", "there is already a chain with same alias"),
			zap.String("alias", alias),
		)
		return
	}
	m.Log.Info("creating chain",
		zap.Stringer("chainID", chainParams.ID),
		zap.Stringer("vmID", chainParams.VMID),
	)

	sb, exists := m.subnets[chainParams.SubnetID]
	if !exists {
		sb = newSubnet()
		m.subnets[chainParams.SubnetID] = sb
	}

	sb.addChain(chainParams.ID)

	// Note: buildChain builds all chain's relevant objects (notably engine and handler)
	// but does not start their operations. Starting of the handler (which could potentially
	// issue some internal messages), is delayed until chain dispatching is started and
	// the chain is registered in the manager. This ensures that no message generated by handler
	// upon start is dropped.
	chain, err := m.buildChain(chainParams, sb)
	if err != nil {
		sb.removeChain(chainParams.ID)
		if m.CriticalChains.Contains(chainParams.ID) {
			// Shut down if we fail to create a required chain (i.e. X, P or C)
			m.Log.Fatal("error creating required chain",
				zap.Stringer("chainID", chainParams.ID),
				zap.Error(err),
			)
			go m.ShutdownNodeFunc(1)
			return
		}

		chainAlias := m.PrimaryAliasOrDefault(chainParams.ID)
		m.Log.Error("error creating chain",
			zap.String("chainAlias", chainAlias),
			zap.Error(err),
		)

		// Register the health check for this chain regardless of if it was
		// created or not. This attempts to notify the node operator that their
		// node may not be properly validating the subnet they expect to be
		// validating.
		healthCheckErr := fmt.Errorf("failed to create chain on whitelisted subnet: %s", chainParams.SubnetID)
		if err := m.Health.RegisterHealthCheck(chainAlias, health.CheckerFunc(func() (interface{}, error) {
			return nil, healthCheckErr
		})); err != nil {
			m.Log.Error("failed to register failing health check",
				zap.String("chainAlias", chainAlias),
				zap.Error(err),
			)
		}
		return
	}

	m.chainsLock.Lock()
	m.chains[chainParams.ID] = chain.Handler
	m.chainsLock.Unlock()

	// Associate the newly created chain with its default alias
	m.Log.AssertNoError(m.Alias(chainParams.ID, chainParams.ID.String()))

	// Notify those that registered to be notified when a new chain is created
	m.notifyRegistrants(chain.Name, chain.Engine)

	// Allows messages to be routed to the new chain. If the handler hasn't been
	// started and a message is forwarded, then the message will block until the
	// handler is started.
	m.ManagerConfig.Router.AddChain(chain.Handler)

	// Register bootstrapped health checks after P chain has been added to
	// chains.
	//
	// Note: Registering this after the chain has been tracked prevents a race
	//       condition between the health check and adding the first chain to
	//       the manager.
	if chainParams.ID == constants.PlatformChainID {
		if err := m.registerBootstrappedHealthChecks(); err != nil {
			chain.Handler.StopWithError(err)
		}
	}

	// Tell the chain to start processing messages.
	// If the X, P, or C Chain panics, do not attempt to recover
	chain.Handler.Start(!m.CriticalChains.Contains(chainParams.ID))
}

// Create a chain
func (m *manager) buildChain(chainParams ChainParameters, sb Subnet) (*chain, error) {
	if chainParams.ID != constants.PlatformChainID && chainParams.VMID == constants.PlatformVMID {
		return nil, errCreatePlatformVM
	}
	primaryAlias := m.PrimaryAliasOrDefault(chainParams.ID)

	// Create the log and context of the chain
	chainLog, err := m.LogFactory.MakeChain(primaryAlias)
	if err != nil {
		return nil, fmt.Errorf("error while creating chain's log %w", err)
	}

	consensusMetrics := prometheus.NewRegistry()
	chainNamespace := fmt.Sprintf("%s_%s", constants.PlatformName, primaryAlias)
	if err := m.Metrics.Register(chainNamespace, consensusMetrics); err != nil {
		return nil, fmt.Errorf("error while registering chain's metrics %w", err)
	}

	vmMetrics := metrics.NewOptionalGatherer()
	vmNamespace := fmt.Sprintf("%s_vm", chainNamespace)
	if err := m.Metrics.Register(vmNamespace, vmMetrics); err != nil {
		return nil, fmt.Errorf("error while registering vm's metrics %w", err)
	}

	ctx := &snow.ConsensusContext{
		Context: &snow.Context{
			NetworkID: m.NetworkID,
			SubnetID:  chainParams.SubnetID,
			ChainID:   chainParams.ID,
			NodeID:    m.NodeID,

			XChainID:    m.XChainID,
			AVAXAssetID: m.AVAXAssetID,

			Log:          chainLog,
			Keystore:     m.Keystore.NewBlockchainKeyStore(chainParams.ID),
			SharedMemory: m.AtomicMemory.NewSharedMemory(chainParams.ID),
			BCLookup:     m,
			SNLookup:     m,
			Metrics:      vmMetrics,

			ValidatorState:    m.validatorState,
			StakingCertLeaf:   m.StakingCert.Leaf,
			StakingLeafSigner: m.StakingCert.PrivateKey.(crypto.Signer),
		},
		DecisionAcceptor:  m.DecisionAcceptorGroup,
		ConsensusAcceptor: m.ConsensusAcceptorGroup,
		Registerer:        consensusMetrics,
	}
	// We set the state to Initializing here because failing to set the state
	// before it's first access would cause a panic.
	ctx.SetState(snow.Initializing)

	if sbConfigs, ok := m.SubnetConfigs[chainParams.SubnetID]; ok {
		if sbConfigs.ValidatorOnly {
			ctx.SetValidatorOnly()
		}
	}

	// Get a factory for the vm we want to use on our chain
	vmFactory, err := m.VMManager.GetFactory(chainParams.VMID)
	if err != nil {
		return nil, fmt.Errorf("error while getting vmFactory: %w", err)
	}

	// Create the chain
	vm, err := vmFactory.New(ctx.Context)
	if err != nil {
		return nil, fmt.Errorf("error while creating vm: %w", err)
	}
	// TODO: Shutdown VM if an error occurs

	fxs := make([]*common.Fx, len(chainParams.FxIDs))
	for i, fxID := range chainParams.FxIDs {
		// Get a factory for the fx we want to use on our chain
		fxFactory, err := m.VMManager.GetFactory(fxID)
		if err != nil {
			return nil, fmt.Errorf("error while getting fxFactory: %w", err)
		}

		fx, err := fxFactory.New(ctx.Context)
		if err != nil {
			return nil, fmt.Errorf("error while creating fx: %w", err)
		}

		// Create the fx
		fxs[i] = &common.Fx{
			ID: fxID,
			Fx: fx,
		}
	}

	consensusParams := m.ConsensusParams
	if sbConfigs, ok := m.SubnetConfigs[chainParams.SubnetID]; ok && chainParams.SubnetID != constants.PrimaryNetworkID {
		consensusParams = sbConfigs.ConsensusParameters
	}

	// The validators of this blockchain
	var vdrs validators.Set // Validators validating this blockchain
	var ok bool
	if m.StakingEnabled {
		vdrs, ok = m.Validators.GetValidators(chainParams.SubnetID)
	} else { // Staking is disabled. Every peer validates every subnet.
		vdrs, ok = m.Validators.GetValidators(constants.PrimaryNetworkID)
	}
	if !ok {
		return nil, fmt.Errorf("couldn't get validator set of subnet with ID %s. The subnet may not exist", chainParams.SubnetID)
	}

	beacons := vdrs
	if chainParams.CustomBeacons != nil {
		beacons = chainParams.CustomBeacons
	}

	bootstrapWeight := beacons.Weight()

	var chain *chain
	switch vm := vm.(type) {
	case vertex.DAGVM:
		chain, err = m.createAvalancheChain(
			ctx,
			chainParams.GenesisData,
			vdrs,
			beacons,
			vm,
			fxs,
			consensusParams,
			bootstrapWeight,
			sb,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating new avalanche vm %w", err)
		}
	case block.ChainVM:
		chain, err = m.createSnowmanChain(
			ctx,
			chainParams.GenesisData,
			vdrs,
			beacons,
			vm,
			fxs,
			consensusParams.Parameters,
			bootstrapWeight,
			sb,
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating new snowman vm %w", err)
		}
	default:
		return nil, errUnknownVMType
	}

	// Register the chain with the timeout manager
	if err := m.TimeoutManager.RegisterChain(ctx); err != nil {
		return nil, err
	}

	return chain, nil
}

func (m *manager) AddRegistrant(r Registrant) { m.registrants = append(m.registrants, r) }

func (m *manager) unblockChains() {
	m.unblocked = true
	blocked := m.blockedChains
	m.blockedChains = nil
	for _, chainParams := range blocked {
		m.ForceCreateChain(chainParams)
	}
}

// Create a DAG-based blockchain that uses Avalanche
func (m *manager) createAvalancheChain(
	ctx *snow.ConsensusContext,
	genesisData []byte,
	vdrs,
	beacons validators.Set,
	vm vertex.DAGVM,
	fxs []*common.Fx,
	consensusParams avcon.Parameters,
	bootstrapWeight uint64,
	sb Subnet,
) (*chain, error) {
	ctx.Lock.Lock()
	defer ctx.Lock.Unlock()

	meterDBManager, err := m.DBManager.NewMeterDBManager("db", ctx.Registerer)
	if err != nil {
		return nil, err
	}
	prefixDBManager := meterDBManager.NewPrefixDBManager(ctx.ChainID[:])
	vmDBManager := prefixDBManager.NewPrefixDBManager([]byte("vm"))

	db := prefixDBManager.Current()
	vertexDB := prefixdb.New([]byte("vertex"), db.Database)
	vertexBootstrappingDB := prefixdb.New([]byte("vertex_bs"), db.Database)
	txBootstrappingDB := prefixdb.New([]byte("tx_bs"), db.Database)

	vtxBlocker, err := queue.NewWithMissing(vertexBootstrappingDB, "vtx", ctx.Registerer)
	if err != nil {
		return nil, err
	}
	txBlocker, err := queue.New(txBootstrappingDB, "tx", ctx.Registerer)
	if err != nil {
		return nil, err
	}

	// The channel through which a VM may send messages to the consensus engine
	// VM uses this channel to notify engine that a block is ready to be made
	msgChan := make(chan common.Message, defaultChannelSize)

	gossipConfig := m.GossipConfig
	if sbConfigs, ok := m.SubnetConfigs[ctx.SubnetID]; ok && ctx.SubnetID != constants.PrimaryNetworkID {
		gossipConfig = sbConfigs.GossipConfig
	}

	// Passes messages from the consensus engine to the network
	sender, err := sender.New(
		ctx,
		m.MsgCreator,
		m.Net,
		m.ManagerConfig.Router,
		m.TimeoutManager,
		gossipConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize sender: %w", err)
	}

	if err := m.ConsensusAcceptorGroup.RegisterAcceptor(ctx.ChainID, "gossip", sender, false); err != nil { // Set up the event dipatcher
		return nil, fmt.Errorf("problem initializing event dispatcher: %w", err)
	}

	chainConfig, err := m.getChainConfig(ctx.ChainID)
	if err != nil {
		return nil, fmt.Errorf("error while fetching chain config: %w", err)
	}

	if m.MeterVMEnabled {
		vm = metervm.NewVertexVM(vm)
	}

	// Handles serialization/deserialization of vertices and also the
	// persistence of vertices
	vtxManager := state.NewSerializer(
		state.SerializerConfig{
			ChainID:             ctx.ChainID,
			VM:                  vm,
			DB:                  vertexDB,
			Log:                 ctx.Log,
			XChainMigrationTime: version.GetXChainMigrationTime(ctx.NetworkID),
		},
	)
	if err := vm.Initialize(
		ctx.Context,
		vmDBManager,
		genesisData,
		chainConfig.Upgrade,
		chainConfig.Config,
		msgChan,
		fxs,
		sender,
	); err != nil {
		return nil, fmt.Errorf("error during vm's Initialize: %w", err)
	}

	sampleK := consensusParams.K
	if uint64(sampleK) > bootstrapWeight {
		sampleK = int(bootstrapWeight)
	}

	// Asynchronously passes messages from the network to the consensus engine
	handler, err := handler.New(
		m.MsgCreator,
		ctx,
		vdrs,
		msgChan,
		sb.afterBootstrapped(),
		m.ConsensusGossipFrequency,
		m.ResourceTracker,
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing network handler: %w", err)
	}

	connectedPeers := tracker.NewPeers()
	startupTracker := tracker.NewStartup(connectedPeers, (3*bootstrapWeight+3)/4)
	beacons.RegisterCallbackListener(startupTracker)

	commonCfg := common.Config{
		Ctx:                            ctx,
		Validators:                     vdrs,
		Beacons:                        beacons,
		SampleK:                        sampleK,
		StartupTracker:                 startupTracker,
		Alpha:                          bootstrapWeight/2 + 1, // must be > 50%
		Sender:                         sender,
		Subnet:                         sb,
		Timer:                          handler,
		RetryBootstrap:                 m.RetryBootstrap,
		RetryBootstrapWarnFrequency:    m.RetryBootstrapWarnFrequency,
		MaxTimeGetAncestors:            m.BootstrapMaxTimeGetAncestors,
		AncestorsMaxContainersSent:     m.BootstrapAncestorsMaxContainersSent,
		AncestorsMaxContainersReceived: m.BootstrapAncestorsMaxContainersReceived,
		SharedCfg:                      &common.SharedConfig{},
	}

	avaGetHandler, err := avagetter.New(vtxManager, commonCfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize avalanche base message handler: %w", err)
	}

	// create bootstrap gear
	bootstrapperConfig := avbootstrap.Config{
		Config:        commonCfg,
		AllGetsServer: avaGetHandler,
		VtxBlocked:    vtxBlocker,
		TxBlocked:     txBlocker,
		Manager:       vtxManager,
		VM:            vm,
	}
	bootstrapper, err := avbootstrap.New(
		bootstrapperConfig,
		func(lastReqID uint32) error {
			return handler.Consensus().Start(lastReqID + 1)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing avalanche bootstrapper: %w", err)
	}
	handler.SetBootstrapper(bootstrapper)

	// create engine gear
	engineConfig := aveng.Config{
		Ctx:           bootstrapperConfig.Ctx,
		AllGetsServer: avaGetHandler,
		VM:            bootstrapperConfig.VM,
		Manager:       vtxManager,
		Sender:        bootstrapperConfig.Sender,
		Validators:    vdrs,
		Params:        consensusParams,
		Consensus:     &avcon.Topological{},
	}
	engine, err := aveng.New(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("error initializing avalanche engine: %w", err)
	}
	handler.SetConsensus(engine)

	// Register health check for this chain
	chainAlias := m.PrimaryAliasOrDefault(ctx.ChainID)

	if err := m.Health.RegisterHealthCheck(chainAlias, handler); err != nil {
		return nil, fmt.Errorf("couldn't add health check for chain %s: %w", chainAlias, err)
	}

	return &chain{
		Name:    chainAlias,
		Engine:  engine,
		Handler: handler,
	}, nil
}

// Create a linear chain using the Snowman consensus engine
func (m *manager) createSnowmanChain(
	ctx *snow.ConsensusContext,
	genesisData []byte,
	vdrs,
	beacons validators.Set,
	vm block.ChainVM,
	fxs []*common.Fx,
	consensusParams snowball.Parameters,
	bootstrapWeight uint64,
	sb Subnet,
) (*chain, error) {
	ctx.Lock.Lock()
	defer ctx.Lock.Unlock()

	meterDBManager, err := m.DBManager.NewMeterDBManager("db", ctx.Registerer)
	if err != nil {
		return nil, err
	}
	prefixDBManager := meterDBManager.NewPrefixDBManager(ctx.ChainID[:])
	vmDBManager := prefixDBManager.NewPrefixDBManager([]byte("vm"))

	db := prefixDBManager.Current()
	bootstrappingDB := prefixdb.New([]byte("bs"), db.Database)

	blocked, err := queue.NewWithMissing(bootstrappingDB, "block", ctx.Registerer)
	if err != nil {
		return nil, err
	}

	// The channel through which a VM may send messages to the consensus engine
	// VM uses this channel to notify engine that a block is ready to be made
	msgChan := make(chan common.Message, defaultChannelSize)

	gossipConfig := m.GossipConfig
	if sbConfigs, ok := m.SubnetConfigs[ctx.SubnetID]; ok && ctx.SubnetID != constants.PrimaryNetworkID {
		gossipConfig = sbConfigs.GossipConfig
	}

	// Passes messages from the consensus engine to the network
	sender, err := sender.New(
		ctx,
		m.MsgCreator,
		m.Net,
		m.ManagerConfig.Router,
		m.TimeoutManager,
		gossipConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize sender: %w", err)
	}

	if err := m.ConsensusAcceptorGroup.RegisterAcceptor(ctx.ChainID, "gossip", sender, false); err != nil { // Set up the event dipatcher
		return nil, fmt.Errorf("problem initializing event dispatcher: %w", err)
	}

	// first vm to be init is P-Chain once, which provides validator interface to all ProposerVMs
	if m.validatorState == nil {
		valState, ok := vm.(validators.State)
		if !ok {
			return nil, fmt.Errorf("expected validators.State but got %T", vm)
		}

		lockedValState := validators.NewLockedState(&ctx.Lock, valState)

		// Initialize the validator state for future chains.
		m.validatorState = lockedValState

		// Notice that this context is left unlocked. This is because the
		// lock will already be held when accessing these values on the
		// P-chain.
		ctx.ValidatorState = valState

		if !m.ManagerConfig.StakingEnabled {
			m.validatorState = validators.NewNoValidatorsState(m.validatorState)
			ctx.ValidatorState = validators.NewNoValidatorsState(ctx.ValidatorState)
		}
	}

	// Initialize the ProposerVM and the vm wrapped inside it
	chainConfig, err := m.getChainConfig(ctx.ChainID)
	if err != nil {
		return nil, fmt.Errorf("error while fetching chain config: %w", err)
	}

	// enable ProposerVM on this VM
	vm = proposervm.New(vm, m.ApricotPhase4Time, m.ApricotPhase4MinPChainHeight)

	if m.MeterVMEnabled {
		vm = metervm.NewBlockVM(vm)
	}
	if err := vm.Initialize(
		ctx.Context,
		vmDBManager,
		genesisData,
		chainConfig.Upgrade,
		chainConfig.Config,
		msgChan,
		fxs,
		sender,
	); err != nil {
		return nil, err
	}

	sampleK := consensusParams.K
	if uint64(sampleK) > bootstrapWeight {
		sampleK = int(bootstrapWeight)
	}

	// Asynchronously passes messages from the network to the consensus engine
	handler, err := handler.New(
		m.MsgCreator,
		ctx,
		vdrs,
		msgChan,
		sb.afterBootstrapped(),
		m.ConsensusGossipFrequency,
		m.ResourceTracker,
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize message handler: %w", err)
	}

	connectedPeers := tracker.NewPeers()
	startupTracker := tracker.NewStartup(connectedPeers, (3*bootstrapWeight+3)/4)
	beacons.RegisterCallbackListener(startupTracker)

	commonCfg := common.Config{
		Ctx:                            ctx,
		Validators:                     vdrs,
		Beacons:                        beacons,
		SampleK:                        sampleK,
		StartupTracker:                 startupTracker,
		Alpha:                          bootstrapWeight/2 + 1, // must be > 50%
		Sender:                         sender,
		Subnet:                         sb,
		Timer:                          handler,
		RetryBootstrap:                 m.RetryBootstrap,
		RetryBootstrapWarnFrequency:    m.RetryBootstrapWarnFrequency,
		MaxTimeGetAncestors:            m.BootstrapMaxTimeGetAncestors,
		AncestorsMaxContainersSent:     m.BootstrapAncestorsMaxContainersSent,
		AncestorsMaxContainersReceived: m.BootstrapAncestorsMaxContainersReceived,
		SharedCfg:                      &common.SharedConfig{},
	}

	snowGetHandler, err := snowgetter.New(vm, commonCfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize snow base message handler: %w", err)
	}

	// Create engine, bootstrapper and state-syncer in this order,
	// to make sure start callbacks are duly initialized
	engineConfig := smeng.Config{
		Ctx:           commonCfg.Ctx,
		AllGetsServer: snowGetHandler,
		VM:            vm,
		Sender:        commonCfg.Sender,
		Validators:    vdrs,
		Params:        consensusParams,
		Consensus:     &smcon.Topological{},
	}
	engine, err := smeng.New(engineConfig)
	if err != nil {
		return nil, fmt.Errorf("error initializing snowman engine: %w", err)
	}
	handler.SetConsensus(engine)

	// create bootstrap gear
	bootstrapCfg := smbootstrap.Config{
		Config:        commonCfg,
		AllGetsServer: snowGetHandler,
		Blocked:       blocked,
		VM:            vm,
		Bootstrapped:  m.unblockChains,
	}
	bootstrapper, err := smbootstrap.New(
		bootstrapCfg,
		engine.Start,
	)
	if err != nil {
		return nil, fmt.Errorf("error initializing snowman bootstrapper: %w", err)
	}
	handler.SetBootstrapper(bootstrapper)

	// create state sync gear
	stateSyncCfg, err := syncer.NewConfig(
		commonCfg,
		m.StateSyncBeacons,
		snowGetHandler,
		vm,
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't initialize state syncer configuration: %w", err)
	}
	stateSyncer := syncer.New(
		stateSyncCfg,
		bootstrapper.Start,
	)
	handler.SetStateSyncer(stateSyncer)

	// Register health checks
	chainAlias := m.PrimaryAliasOrDefault(ctx.ChainID)

	if err := m.Health.RegisterHealthCheck(chainAlias, handler); err != nil {
		return nil, fmt.Errorf("couldn't add health check for chain %s: %w", chainAlias, err)
	}

	return &chain{
		Name:    chainAlias,
		Engine:  engine,
		Handler: handler,
	}, nil
}

func (m *manager) SubnetID(chainID ids.ID) (ids.ID, error) {
	m.chainsLock.Lock()
	defer m.chainsLock.Unlock()

	chain, exists := m.chains[chainID]
	if !exists {
		return ids.ID{}, errUnknownChainID
	}
	return chain.Context().SubnetID, nil
}

func (m *manager) IsBootstrapped(id ids.ID) bool {
	m.chainsLock.Lock()
	chain, exists := m.chains[id]
	m.chainsLock.Unlock()
	if !exists {
		return false
	}

	return chain.Context().GetState() == snow.NormalOp
}

func (m *manager) chainsNotBootstrapped() []ids.ID {
	m.chainsLock.Lock()
	defer m.chainsLock.Unlock()

	chainsBootstrapping := make([]ids.ID, 0, len(m.chains))
	for chainID, chain := range m.chains {
		if chain.Context().GetState() == snow.NormalOp {
			continue
		}
		chainsBootstrapping = append(chainsBootstrapping, chainID)
	}
	return chainsBootstrapping
}

func (m *manager) registerBootstrappedHealthChecks() error {
	bootstrappedCheck := health.CheckerFunc(func() (interface{}, error) {
		chains := m.chainsNotBootstrapped()
		aliases := make([]string, len(chains))
		for i, chain := range chains {
			aliases[i] = m.PrimaryAliasOrDefault(chain)
		}

		if len(aliases) != 0 {
			return aliases, errNotBootstrapped
		}
		return aliases, nil
	})
	if err := m.Health.RegisterReadinessCheck("bootstrapped", bootstrappedCheck); err != nil {
		return fmt.Errorf("couldn't register bootstrapped readiness check: %w", err)
	}
	if err := m.Health.RegisterHealthCheck("bootstrapped", bootstrappedCheck); err != nil {
		return fmt.Errorf("couldn't register bootstrapped health check: %w", err)
	}
	return nil
}

// Shutdown stops all the chains
func (m *manager) Shutdown() {
	m.Log.Info("shutting down chain manager")
	m.ManagerConfig.Router.Shutdown()
}

// LookupVM returns the ID of the VM associated with an alias
func (m *manager) LookupVM(alias string) (ids.ID, error) { return m.VMManager.Lookup(alias) }

// Notify registrants [those who want to know about the creation of chains]
// that the specified chain has been created
func (m *manager) notifyRegistrants(name string, engine common.Engine) {
	for _, registrant := range m.registrants {
		registrant.RegisterChain(name, engine)
	}
}

// Returns:
// 1) the alias that already exists, or the empty string if there is none
// 2) true iff there exists a chain such that the chain has an alias in [aliases]
func (m *manager) isChainWithAlias(aliases ...string) (string, bool) {
	for _, alias := range aliases {
		if _, err := m.Lookup(alias); err == nil {
			return alias, true
		}
	}
	return "", false
}

// getChainConfig returns value of a entry by looking at ID key and alias key
// it first searches ID key, then falls back to it's corresponding primary alias
func (m *manager) getChainConfig(id ids.ID) (ChainConfig, error) {
	if val, ok := m.ManagerConfig.ChainConfigs[id.String()]; ok {
		return val, nil
	}
	aliases, err := m.Aliases(id)
	if err != nil {
		return ChainConfig{}, err
	}
	for _, alias := range aliases {
		if val, ok := m.ManagerConfig.ChainConfigs[alias]; ok {
			return val, nil
		}
	}

	return ChainConfig{}, nil
}
