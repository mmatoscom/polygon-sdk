package tests

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"strings"
	"testing"

	"github.com/umbracle/fastrlp"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/state/runtime/evm"
	"github.com/0xPolygon/minimal/types"

	"github.com/0xPolygon/minimal/crypto"
)

var mainnetChainConfig = chain.Params{
	Forks: &chain.Forks{
		Homestead: chain.NewFork(1150000),
		EIP150:    chain.NewFork(2463000),
		EIP158:    chain.NewFork(2675000),
		Byzantium: chain.NewFork(4370000),
	},
}

var vmTests = "VMTests"

type VMCase struct {
	Info *info `json:"_info"`
	Env  *env  `json:"env"`
	Exec *exec `json:"exec"`

	Gas  string `json:"gas"`
	Logs string `json:"logs"`
	Out  string `json:"out"`

	Post map[types.Address]*chain.GenesisAccount `json:"post"`
	Pre  map[types.Address]*chain.GenesisAccount `json:"pre"`
}

func testVMCase(t *testing.T, name string, c *VMCase) {
	env := c.Env.ToEnv(t)
	env.GasPrice = types.BytesToHash(c.Exec.GasPrice.Bytes())
	env.Origin = c.Exec.Origin

	s, _, root := buildState(t, c.Pre)

	config := mainnetChainConfig.Forks.At(uint64(env.Number))

	executor := state.NewExecutor(&mainnetChainConfig, s)
	executor.GetHash = func(*types.Header) func(i uint64) types.Hash {
		return vmTestBlockHash
	}

	e, _ := executor.BeginTxn(root, c.Env.ToHeader(t))

	evmR := evm.NewEVM()

	code := e.GetCode(c.Exec.Address)
	contract := runtime.NewContractCall(1, c.Exec.Caller, c.Exec.Caller, c.Exec.Address, c.Exec.Value, c.Exec.GasLimit, code, c.Exec.Data)

	ret, gas, err := evmR.Run(contract, e, &config)

	if c.Gas == "" {
		if err == nil {
			t.Fatalf("gas unspecified (indicating an error), but VM returned no error")
		}
		if gas > 0 {
			t.Fatalf("gas unspecified (indicating an error), but VM returned gas remaining > 0")
		}
		return
	}

	// check return
	if c.Out == "" {
		c.Out = "0x"
	}
	if ret := hex.EncodeToHex(ret); ret != c.Out {
		t.Fatalf("return mismatch: got %s, want %s", ret, c.Out)
	}

	txn := e.Txn()

	// check logs
	if logs := rlpHashLogs(txn.Logs()); logs != types.StringToHash(c.Logs) {
		t.Fatalf("logs hash mismatch: got %x, want %x", logs, c.Logs)
	}

	// check state
	for addr, alloc := range c.Post {
		for key, val := range alloc.Storage {
			if have := txn.GetState(addr, key); have != val {
				t.Fatalf("wrong storage value at %x:\n  got  %x\n  want %x", key, have, val)
			}
		}
	}

	// check remaining gas
	if expected := stringToUint64T(t, c.Gas); gas != expected {
		t.Fatalf("gas remaining mismatch: got %d want %d", gas, expected)
	}
}

func rlpHashLogs(logs []*types.Log) (res types.Hash) {
	r := &types.Receipt{
		Logs: logs,
	}

	ar := &fastrlp.Arena{}
	v := r.MarshalLogsWith(ar)

	keccak.Keccak256Rlp(res[:0], v)
	return
}

func TestEVM(t *testing.T) {
	folders, err := listFolders(vmTests)
	if err != nil {
		t.Fatal(err)
	}

	long := []string{
		"loop-",
		"gasprice",
		"origin",
	}

	for _, folder := range folders {
		files, err := listFiles(folder)
		if err != nil {
			t.Fatal(err)
		}

		for _, file := range files {
			t.Run(file, func(t *testing.T) {
				if !strings.HasSuffix(file, ".json") {
					return
				}

				data, err := ioutil.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var vmcases map[string]*VMCase
				if err := json.Unmarshal(data, &vmcases); err != nil {
					t.Fatal(err)
				}

				for name, cc := range vmcases {
					if contains(long, name) && testing.Short() {
						t.Skip()
						continue
					}
					testVMCase(t, name, cc)
				}
			})
		}
	}
}

func vmTestBlockHash(n uint64) types.Hash {
	return types.BytesToHash(crypto.Keccak256([]byte(big.NewInt(int64(n)).String())))
}
