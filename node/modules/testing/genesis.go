package testing

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/xerrors"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-car"
	"github.com/ipfs/go-cid"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-merkledag"
	peer "github.com/libp2p/go-libp2p-peer"

	"github.com/filecoin-project/lotus/chain/address"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
)

var glog = logging.Logger("genesis")

func MakeGenesisMem(out io.Writer, minerPid peer.ID) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			// TODO: make an address allocation
			w, err := w.GenerateKey(types.KTBLS)
			if err != nil {
				return nil, err
			}

			gmc := &gen.GenMinerCfg{
				PeerIDs: []peer.ID{minerPid},
				// TODO: Call seal.PreSeal
			}
			alloc := map[address.Address]types.BigInt{
				w: types.FromFil(10000),
			}

			b, err := gen.MakeGenesisBlock(bs, alloc, gmc, 100000)
			if err != nil {
				return nil, err
			}
			offl := offline.Exchange(bs)
			blkserv := blockservice.New(bs, offl)
			dserv := merkledag.NewDAGService(blkserv)

			if err := car.WriteCar(context.TODO(), dserv, []cid.Cid{b.Genesis.Cid()}, out); err != nil {
				return nil, err
			}

			return b.Genesis, nil
		}
	}
}

func MakeGenesis(outFile, preseal string) func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
	return func(bs dtypes.ChainBlockstore, w *wallet.Wallet) modules.Genesis {
		return func() (*types.BlockHeader, error) {
			glog.Warn("Generating new random genesis block, note that this SHOULD NOT happen unless you are setting up new network")
			fdata, err := ioutil.ReadFile(preseal)
			if err != nil {
				return nil, err
			}

			var preseal map[string]genesis.GenesisMiner
			if err := json.Unmarshal(fdata, &preseal); err != nil {
				return nil, err
			}

			minerAddresses := make([]address.Address, 0, len(preseal))
			for s := range preseal {
				a, err := address.NewFromString(s)
				if err != nil {
					return nil, err
				}
				if a.Protocol() != address.ID {
					return nil, xerrors.New("expected ID address")
				}
				minerAddresses = append(minerAddresses, a)
			}

			gmc := &gen.GenMinerCfg{
				PeerIDs:    []peer.ID{"peer ID 1"},
				PreSeals:   preseal,
				MinerAddrs: minerAddresses,
			}

			addrs := map[address.Address]types.BigInt{}

			for _, miner := range preseal {
				if _, err := w.Import(&miner.Key); err != nil {
					return nil, xerrors.Errorf("importing miner key: %w", err)
				}

				_ = w.SetDefault(miner.Worker)

				addrs[miner.Worker] = types.FromFil(100000)
			}

			b, err := gen.MakeGenesisBlock(bs, addrs, gmc, uint64(time.Now().Unix()))
			if err != nil {
				return nil, err
			}

			fmt.Println("GENESIS MINER ADDRESS: ", gmc.MinerAddrs[0].String())

			f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, err
			}

			offl := offline.Exchange(bs)
			blkserv := blockservice.New(bs, offl)
			dserv := merkledag.NewDAGService(blkserv)

			if err := car.WriteCar(context.TODO(), dserv, []cid.Cid{b.Genesis.Cid()}, f); err != nil {
				return nil, err
			}

			glog.Warnf("WRITING GENESIS FILE AT %s", f.Name())

			if err := f.Close(); err != nil {
				return nil, err
			}

			return b.Genesis, nil
		}
	}
}
