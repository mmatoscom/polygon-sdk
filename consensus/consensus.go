package consensus

import (
	"context"
	"log"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

// Consensus is the interface for consensus
type Consensus interface {
	// VerifyHeader verifies the header is correct
	VerifyHeader(parent, header *types.Header) error

	// Start starts the consensus
	Start() error

	// Close closes the connection
	Close() error
}

// Config is the configuration for the consensus
type Config struct {
	// Logger to be used by the backend
	Logger *log.Logger

	// Params are the params of the chain and the consensus
	Params *chain.Params

	// Specific configuration parameters for the backend
	Config map[string]interface{}

	// Path for the consensus protocol tos tore information
	Path string
}

// Factory is the factory function to create a discovery backend
type Factory func(context.Context, bool, *Config, *txpool.TxPool, *network.Server, *blockchain.Blockchain, *state.Executor, *grpc.Server, hclog.Logger) (Consensus, error)
