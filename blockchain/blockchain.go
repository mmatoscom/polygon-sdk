package blockchain

import (
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/blockchain/storage/leveldb"
	"github.com/0xPolygon/minimal/blockchain/storage/memory"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"

	"github.com/hashicorp/go-hclog"
	lru "github.com/hashicorp/golang-lru"
)

var (
	errDuplicateUncle  = errors.New("duplicate uncle")
	errUncleIsAncestor = errors.New("uncle is ancestor")
	errDanglingUncle   = errors.New("uncle's parent is not ancestor")
)

// Blockchain is a blockchain reference
type Blockchain struct {
	logger hclog.Logger

	db        storage.Storage
	consensus Verifier
	executor  Executor

	config  *chain.Chain
	genesis types.Hash

	headersCache    *lru.Cache
	difficultyCache *lru.Cache

	// the current last header + difficulty
	currentHeader     atomic.Value
	currentDifficulty atomic.Value

	// event subscriptions
	stream *eventStream

	// Average gas price (rolling average)
	averageGasPrice      *big.Int
	averageGasPriceCount *big.Int

	// Used for making the UpdateGasPriceAvg atomic
	agpMux sync.Mutex
}

type Verifier interface {
	VerifyHeader(parent, header *types.Header) error
}

type Executor interface {
	ProcessBlock(parentRoot types.Hash, block *types.Block) (*state.BlockResult, error)
}

// UpdateGasPriceAvg Updates the rolling average value of the gas price
func (b *Blockchain) UpdateGasPriceAvg(newValue *big.Int) {
	b.agpMux.Lock()

	b.averageGasPriceCount.Add(b.averageGasPriceCount, big.NewInt(1))

	differential := big.NewInt(0)
	differential.Div(newValue.Sub(newValue, b.averageGasPrice), b.averageGasPriceCount)

	b.averageGasPrice.Add(b.averageGasPrice, differential)

	b.agpMux.Unlock()
}

// GetAvgGasPrice returns the average gas price
func (b *Blockchain) GetAvgGasPrice() *big.Int {
	return b.averageGasPrice
}

// NewBlockchain creates a new blockchain object
func NewBlockchain(logger hclog.Logger, dataDir string, config *chain.Chain, consensus Verifier, executor Executor) (*Blockchain, error) {
	b := &Blockchain{
		logger:    logger.Named("blockchain"),
		config:    config,
		consensus: consensus,
		executor:  executor,
		stream:    &eventStream{},
	}

	var storage storage.Storage
	var err error
	if dataDir == "" {
		if storage, err = memory.NewMemoryStorage(nil); err != nil {
			return nil, err
		}
	} else {
		if storage, err = leveldb.NewLevelDBStorage(filepath.Join(dataDir, "blockchain"), logger); err != nil {
			return nil, err
		}
	}
	b.db = storage

	b.headersCache, _ = lru.New(100)
	b.difficultyCache, _ = lru.New(100)

	// push the first event to the stream
	b.stream.push(&Event{})

	if _, ok := consensus.(*MockVerifier); ok {
		// if we are using mock consensus we can compute right away the genesis since
		// this consensus does not change the header hash
		if err := b.ComputeGenesis(); err != nil {
			return nil, err
		}
	}

	b.averageGasPrice = big.NewInt(0)
	b.averageGasPriceCount = big.NewInt(0)

	return b, nil
}

func (b *Blockchain) ComputeGenesis() error {
	// try to write the genesis block
	head, ok := b.db.ReadHeadHash()
	if ok {
		// initialized storage
		b.genesis, ok = b.db.ReadCanonicalHash(0)
		if !ok {
			return fmt.Errorf("failed to load genesis hash")
		}
		// validate that the genesis file in storage matches the chain.Genesis
		if b.genesis != b.config.Genesis.Hash() {
			return fmt.Errorf("genesis file does not match current genesis")
		}
		header, ok := b.GetHeaderByHash(head)
		if !ok {
			return fmt.Errorf("failed to get header with hash %s", head.String())
		}
		diff, ok := b.GetTD(head)
		if !ok {
			return fmt.Errorf("failed to read difficulty")
		}

		b.logger.Info("Current header", "hash", header.Hash.String(), "number", header.Number)
		b.setCurrentHeader(header, diff)
	} else {
		// empty storage, write the genesis
		if err := b.writeGenesis(b.config.Genesis); err != nil {
			return err
		}
	}
	b.logger.Info("genesis", "hash", b.config.Genesis.Hash())
	return nil
}

func (b *Blockchain) SetConsensus(c Verifier) {
	b.consensus = c
}

func (b *Blockchain) setCurrentHeader(h *types.Header, diff *big.Int) {
	hh := h.Copy()
	b.currentHeader.Store(hh)

	dd := new(big.Int).Set(diff)
	b.currentDifficulty.Store(dd)
}

// Header returns the current header
func (b *Blockchain) Header() *types.Header {
	return b.currentHeader.Load().(*types.Header)
}

// CurrentTD returns the current total difficulty
func (b *Blockchain) CurrentTD() *big.Int {
	return b.currentDifficulty.Load().(*big.Int)
}

func (b *Blockchain) Config() *chain.Params {
	return b.config.Params
}

func (b *Blockchain) GetHeader(hash types.Hash, number uint64) (*types.Header, bool) {
	return b.GetHeaderByHash(hash)
}

func (b *Blockchain) GetBlock(hash types.Hash, number uint64, full bool) (*types.Block, bool) {
	return b.GetBlockByHash(hash, full)
}

// GetParent return the parent
func (b *Blockchain) GetParent(header *types.Header) (*types.Header, bool) {
	return b.readHeader(header.ParentHash)
}

// Genesis returns the genesis block
func (b *Blockchain) Genesis() types.Hash {
	return b.genesis
}

func (b *Blockchain) writeGenesis(genesis *chain.Genesis) error {
	header := genesis.ToBlock()
	header.ComputeHash()

	if err := b.writeGenesisImpl(header); err != nil {
		return err
	}
	return nil
}

func (b *Blockchain) writeGenesisImpl(header *types.Header) error {
	b.genesis = header.Hash

	if err := b.db.WriteHeader(header); err != nil {
		return err
	}
	if _, err := b.advanceHead(header); err != nil {
		return err
	}

	// write the value to the stream
	evnt := &Event{}
	evnt.AddNewHeader(header)
	b.stream.push(evnt)

	return nil
}

// Empty checks if the blockchain is empty
func (b *Blockchain) Empty() bool {
	_, ok := b.db.ReadHeadHash()
	return !ok
}

func (b *Blockchain) GetChainTD() (*big.Int, bool) {
	header := b.Header()
	return b.GetTD(header.Hash)
}

func (b *Blockchain) GetTD(hash types.Hash) (*big.Int, bool) {
	return b.readDiff(hash)
}

func (b *Blockchain) writeCanonicalHeader(evnt *Event, h *types.Header) error {
	td, ok := b.readDiff(h.ParentHash)
	if !ok {
		return fmt.Errorf("parent difficulty not found 2")
	}

	diff := big.NewInt(1).Add(td, new(big.Int).SetUint64(h.Difficulty))
	if err := b.db.WriteCanonicalHeader(h, diff); err != nil {
		return err
	}

	evnt.Type = EventHead
	evnt.AddNewHeader(h)
	evnt.SetDifficulty(diff)

	b.setCurrentHeader(h, diff)
	return nil
}

func (b *Blockchain) advanceHead(h *types.Header) (*big.Int, error) {
	if err := b.db.WriteHeadHash(h.Hash); err != nil {
		return nil, err
	}
	if err := b.db.WriteHeadNumber(h.Number); err != nil {
		return nil, err
	}
	if err := b.db.WriteCanonicalHash(h.Number, h.Hash); err != nil {
		return nil, err
	}

	currentDiff := big.NewInt(0)
	if h.ParentHash != types.StringToHash("") {
		td, ok := b.readDiff(h.ParentHash)
		if !ok {
			return nil, fmt.Errorf("parent difficulty not found 1")
		}
		currentDiff = td
	}

	diff := big.NewInt(1).Add(currentDiff, new(big.Int).SetUint64(h.Difficulty))
	if err := b.db.WriteDiff(h.Hash, diff); err != nil {
		return nil, err
	}

	b.setCurrentHeader(h, diff)
	return diff, nil
}

// GetReceiptsByHash returns the receipts by their hash
func (b *Blockchain) GetReceiptsByHash(hash types.Hash) ([]*types.Receipt, error) {
	return b.db.ReadReceipts(hash)
}

// GetBodyByHash returns the body by their hash
func (b *Blockchain) GetBodyByHash(hash types.Hash) (*types.Body, bool) {
	return b.readBody(hash)
}

// GetHeaderByHash returns the header by his hash
func (b *Blockchain) GetHeaderByHash(hash types.Hash) (*types.Header, bool) {
	return b.readHeader(hash)
}

func (b *Blockchain) readHeader(hash types.Hash) (*types.Header, bool) {
	h, ok := b.headersCache.Get(hash)
	if ok {
		return h.(*types.Header), true
	}
	hh, err := b.db.ReadHeader(hash)
	if err != nil {
		return nil, false
	}
	hh.ComputeHash()
	b.headersCache.Add(hash, hh)
	return hh, true
}

func (b *Blockchain) readBody(hash types.Hash) (*types.Body, bool) {
	bb, err := b.db.ReadBody(hash)
	if err != nil {
		b.logger.Error("failed to read body", "err", err)
		return nil, false
	}
	return bb, true
}

func (b *Blockchain) readDiff(hash types.Hash) (*big.Int, bool) {
	d, ok := b.difficultyCache.Get(hash)
	if ok {
		return d.(*big.Int), true
	}
	dd, ok := b.db.ReadDiff(hash)
	if !ok {
		return nil, false
	}
	b.difficultyCache.Add(hash, dd)
	return dd, true
}

// GetHeaderByNumber returns the header by his number
func (b *Blockchain) GetHeaderByNumber(n uint64) (*types.Header, bool) {
	hash, ok := b.db.ReadCanonicalHash(n)
	if !ok {
		return nil, false
	}
	h, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}
	return h, true
}

func (b *Blockchain) WriteHeaders(headers []*types.Header) error {
	return b.WriteHeadersWithBodies(headers)
}

// WriteHeadersWithBodies writes a batch of headers
func (b *Blockchain) WriteHeadersWithBodies(headers []*types.Header) error {
	// validate chain
	for i := 1; i < len(headers); i++ {
		if headers[i].Number-1 != headers[i-1].Number {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, headers[i].Number, headers[i-1].Number)
		}
		if headers[i].ParentHash != headers[i-1].Hash {
			return fmt.Errorf("parent hash not correct")
		}
	}

	for _, h := range headers {
		evnt := &Event{}
		if err := b.writeHeaderImpl(evnt, h); err != nil {
			return err
		}
		b.dispatchEvent(evnt)
	}
	return nil
}

// WriteBlocks writes a batch of blocks
func (b *Blockchain) WriteBlocks(blocks []*types.Block) error {
	size := len(blocks)
	if size == 0 {
		return fmt.Errorf("no headers found to insert")
	}

	if size == 1 {
		b.logger.Info("write block", "num", blocks[0].Number(), "parent", blocks[0].ParentHash())
	} else {
		b.logger.Info("write blocks", "num", size, "from", blocks[0].Number(), "to", blocks[size-1].Number(), "parent", blocks[0].ParentHash())
	}

	parent, ok := b.readHeader(blocks[0].ParentHash())
	if !ok {
		return fmt.Errorf("parent of %s (%d) not found: %s", blocks[0].Hash().String(), blocks[0].Number(), blocks[0].ParentHash())
	}
	if parent.Hash == types.ZeroHash {
		return fmt.Errorf("parent not found")
	}

	// validate chain
	for i := 0; i < size; i++ {
		block := blocks[i]
		if block.Number()-1 != parent.Number {
			return fmt.Errorf("number sequence not correct at %d, %d and %d", i, block.Number(), parent.Number)
		}
		if block.ParentHash() != parent.Hash {
			return fmt.Errorf("parent hash not correct")
		}
		if err := b.consensus.VerifyHeader(parent, block.Header); err != nil {
			return fmt.Errorf("failed to verify the header: %v", err)
		}

		// verify body data
		if hash := buildroot.CalculateUncleRoot(block.Uncles); hash != block.Header.Sha3Uncles {
			return fmt.Errorf("uncle root hash mismatch: have %s, want %s", hash, block.Header.Sha3Uncles)
		}
		// TODO, the wrapper around transactions
		if hash := buildroot.CalculateTransactionsRoot(block.Transactions); hash != block.Header.TxRoot {
			return fmt.Errorf("transaction root hash mismatch: have %s, want %s", hash, block.Header.TxRoot)
		}
		parent = block.Header
	}

	// Write chain
	for indx, block := range blocks {
		header := block.Header

		if err := b.writeBody(block); err != nil {
			return err
		}
		// Process and validate the block
		res, err := b.processBlock(blocks[indx])
		if err != nil {
			return err
		}

		// Write the header to the chain
		evnt := &Event{}
		if err := b.writeHeaderImpl(evnt, header); err != nil {
			return err
		}
		b.dispatchEvent(evnt)

		// write the receipts, do it only after the header has been written.
		// Otherwise, a client might ask for a header once the receipt is valid
		// but before it is written into the storage
		if err := b.db.WriteReceipts(block.Hash(), res.Receipts); err != nil {
			return err
		}

		// Update the average gas price
		b.UpdateGasPriceAvg(new(big.Int).SetUint64(header.GasUsed))
	}

	b.logger.Info("new head", "hash", b.Header().Hash, "number", b.Header().Number)
	return nil
}

func (b *Blockchain) writeBody(block *types.Block) error {
	body := block.Body()

	// write the full body (txns + receipts)
	if err := b.db.WriteBody(block.Header.Hash, body); err != nil {
		return err
	}

	// write txn lookups (txhash -> block)
	for _, txn := range block.Transactions {
		if err := b.db.WriteTxLookup(txn.Hash, block.Hash()); err != nil {
			return err
		}
	}
	return nil
}

func (b *Blockchain) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	v, ok := b.db.ReadTxLookup(hash)
	return v, ok
}

func (b *Blockchain) processBlock(block *types.Block) (*state.BlockResult, error) {
	header := block.Header

	// process the block
	parent, ok := b.readHeader(header.ParentHash)
	if !ok {
		return nil, fmt.Errorf("unknown ancestor 1")
	}
	result, err := b.executor.ProcessBlock(parent.StateRoot, block)
	if err != nil {
		return nil, err
	}

	receipts := result.Receipts
	if len(receipts) != len(block.Transactions) {
		return nil, fmt.Errorf("bad size of receipts and transactions")
	}

	// validate the fields
	if result.Root != header.StateRoot {
		return nil, fmt.Errorf("invalid merkle root")
	}
	if result.TotalGas != header.GasUsed {
		return nil, fmt.Errorf("gas used is different")
	}
	receiptSha := buildroot.CalculateReceiptsRoot(result.Receipts)
	if receiptSha != header.ReceiptsRoot {
		return nil, fmt.Errorf("invalid receipts root")
	}
	return result, nil
}

var emptyFrom = types.Address{}

func (b *Blockchain) GetHashHelper(header *types.Header) func(i uint64) (res types.Hash) {
	return func(i uint64) (res types.Hash) {
		num, hash := header.Number-1, header.ParentHash

		for {
			if num == i {
				res = hash
				return
			}
			h, ok := b.GetHeaderByHash(hash)
			if !ok {
				return
			}
			hash = h.ParentHash
			if num == 0 {
				return
			}
			num--
		}
	}
}

func (b *Blockchain) GetHashByNumber(i uint64) types.Hash {
	block, ok := b.GetBlockByNumber(i, false)
	if !ok {
		return types.Hash{}
	}
	return block.Hash()
}

// WriteBlock writes a block of data
func (b *Blockchain) WriteBlock(block *types.Block) error {
	evnt := &Event{}
	if err := b.writeHeaderImpl(evnt, block.Header); err != nil {
		return err
	}
	b.dispatchEvent(evnt)
	return nil
}

func (b *Blockchain) dispatchEvent(evnt *Event) {
	b.stream.push(evnt)
}

// WriteHeader writes a block and the data, assumes the genesis is already set
func (b *Blockchain) writeHeaderImpl(evnt *Event, header *types.Header) error {
	head := b.Header()

	// Write the data
	if header.ParentHash == head.Hash {
		// Fast path to save the new canonical header
		return b.writeCanonicalHeader(evnt, header)
	}

	if err := b.db.WriteHeader(header); err != nil {
		return err
	}

	headerDiff, ok := b.readDiff(head.Hash)
	if !ok {
		panic("failed to get header difficulty")
	}

	parentDiff, ok := b.readDiff(header.ParentHash)
	if !ok {
		return fmt.Errorf("parent of %s (%d) not found", header.Hash.String(), header.Number)
	}
	if err := b.db.WriteDiff(header.Hash, big.NewInt(1).Add(parentDiff, new(big.Int).SetUint64(header.Difficulty))); err != nil {
		return err
	}

	b.headersCache.Add(header.Hash, header)

	incomingDiff := big.NewInt(1).Add(parentDiff, new(big.Int).SetUint64(header.Difficulty))
	if incomingDiff.Cmp(headerDiff) > 0 {
		// new block has higher difficulty than us, reorg the chain
		if err := b.handleReorg(evnt, head, header); err != nil {
			return err
		}
	} else {
		// new block has lower difficulty than us, create a new fork
		evnt.AddOldHeader(header)
		evnt.Type = EventFork

		if err := b.writeFork(header); err != nil {
			return err
		}
	}

	return nil
}

func (b *Blockchain) writeFork(header *types.Header) error {
	forks, err := b.db.ReadForks()
	if err != nil {
		if err == storage.ErrNotFound {
			forks = []types.Hash{}
		} else {
			return err
		}
	}
	newForks := []types.Hash{}
	for _, fork := range forks {
		if fork != header.ParentHash {
			newForks = append(newForks, fork)
		}
	}
	newForks = append(newForks, header.Hash)
	if err := b.db.WriteForks(newForks); err != nil {
		return err
	}
	return nil
}

func (b *Blockchain) handleReorg(evnt *Event, oldHeader *types.Header, newHeader *types.Header) error {
	newChainHead := newHeader
	oldChainHead := oldHeader

	oldChain := []*types.Header{}
	newChain := []*types.Header{}

	var ok bool

	for oldHeader.Number > newHeader.Number {
		oldHeader, ok = b.readHeader(oldHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", oldHeader.ParentHash.String())
		}
		oldChain = append(oldChain, oldHeader)
	}

	for newHeader.Number > oldHeader.Number {
		newHeader, ok = b.readHeader(newHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", newHeader.ParentHash.String())
		}
		newChain = append(newChain, newHeader)
	}

	for oldHeader.Hash != newHeader.Hash {
		oldHeader, ok = b.readHeader(oldHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", oldHeader.ParentHash.String())
		}
		newHeader, ok = b.readHeader(newHeader.ParentHash)
		if !ok {
			return fmt.Errorf("header '%s' not found", newHeader.ParentHash.String())
		}

		oldChain = append(oldChain, oldHeader)
	}

	for _, b := range oldChain[:len(oldChain)-1] {
		evnt.AddOldHeader(b)
	}
	evnt.AddOldHeader(oldChainHead)

	evnt.AddNewHeader(newChainHead)
	for _, b := range newChain {
		evnt.AddNewHeader(b)
	}

	if err := b.writeFork(oldChainHead); err != nil {
		return fmt.Errorf("failed to write the old header as fork: %v", err)
	}

	// Update canonical chain numbers
	for _, h := range newChain {
		if err := b.db.WriteCanonicalHash(h.Number, h.Hash); err != nil {
			return err
		}
	}

	diff, err := b.advanceHead(newChainHead)
	if err != nil {
		return err
	}

	evnt.Type = EventReorg
	evnt.SetDifficulty(diff)
	return nil
}

// GetForks returns the forks
func (b *Blockchain) GetForks() ([]types.Hash, error) {
	return b.db.ReadForks()
}

// GetBlockByHash returns the block by their hash
func (b *Blockchain) GetBlockByHash(hash types.Hash, full bool) (*types.Block, bool) {
	header, ok := b.readHeader(hash)
	if !ok {
		return nil, false
	}

	block := &types.Block{
		Header: header,
	}
	if !full {
		return block, true
	}
	body, ok := b.readBody(hash)
	if !ok {
		return block, true
	}

	block.Transactions = body.Transactions
	block.Uncles = body.Uncles
	return block, true
}

// GetBlockByNumber returns the block by their number
func (b *Blockchain) GetBlockByNumber(n uint64, full bool) (*types.Block, bool) {
	hash, ok := b.db.ReadCanonicalHash(n)
	if !ok {
		return nil, false
	}
	return b.GetBlockByHash(hash, full)
}

func (b *Blockchain) Close() error {
	return b.db.Close()
}
