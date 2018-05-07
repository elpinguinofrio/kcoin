package features

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kowala-tech/kcoin/accounts"
	"github.com/kowala-tech/kcoin/cluster"
	"github.com/kowala-tech/kcoin/common"
	"github.com/kowala-tech/kcoin/kcoin/genesis"
	"github.com/kowala-tech/kcoin/kcoinclient"
)

var (
	enodeSecretRegexp = regexp.MustCompile(`enode://([a-f0-9]*)@`)
)

func (ctx *Context) PrepareCluster() error {
	nodeRunner, err := cluster.NewDockerNodeRunner()
	if err != nil {
		return err
	}
	ctx.nodeRunner = nodeRunner

	if err := ctx.generateAccounts(); err != nil {
		return err
	}
	if err := ctx.buildGenesis(); err != nil {
		return err
	}
	if err := ctx.buildDockerImages(); err != nil {
		return err
	}
	if err := ctx.runBootnode(); err != nil {
		return err
	}
	if err := ctx.runGenesisValidator(); err != nil {
		return err
	}
	if err := ctx.triggerGenesisValidation(); err != nil {
		return err
	}
	if err := ctx.runRpc(); err != nil {
		return err
	}
	return nil
}

func (ctx *Context) generateAccounts() error {
	seederAccount, err := ctx.newAccount()
	if err != nil {
		return err
	}
	ctx.seederAccount = *seederAccount

	genesisValidatorAccount, err := ctx.newAccount()
	if err != nil {
		return err
	}
	ctx.genesisValidatorAccount = *genesisValidatorAccount

	return nil
}

func (ctx *Context) newAccount() (*accounts.Account, error) {
	acc, err := ctx.AccountsStorage.NewAccount("test")
	if err != nil {
		return nil, err
	}
	if err := ctx.AccountsStorage.Unlock(acc, "test"); err != nil {
		return nil, err
	}
	return &acc, nil
}

func (ctx *Context) buildDockerImages() error {
	var wg sync.WaitGroup
	wg.Add(2)
	errors := make([]error, 0)

	go func() {
		if err := ctx.nodeRunner.BuildDockerImage("kowalatech/bootnode:dev", "bootnode.Dockerfile"); err != nil {
			errors = append(errors, err)
		}
		wg.Done()
	}()

	go func() {
		if err := ctx.nodeRunner.BuildDockerImage("kowalatech/kusd:dev", "kcoin.Dockerfile"); err != nil {
			errors = append(errors, err)
		}
		wg.Done()
	}()
	wg.Wait()

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

func (ctx *Context) runBootnode() error {
	bootnode, err := cluster.BootnodeSpec()
	if err != nil {
		return err
	}
	if err := ctx.nodeRunner.Run(bootnode); err != nil {
		return err
	}
	err = common.WaitFor("fetching bootnode enode", 1*time.Second, 20*time.Second, func() bool {
		bootnodeStdout, err := ctx.nodeRunner.Log(bootnode.ID)
		if err != nil {
			return false
		}
		found := enodeSecretRegexp.FindStringSubmatch(bootnodeStdout)
		if len(found) != 2 {
			return false
		}
		enodeSecret := found[1]
		bootnodeIP, err := ctx.nodeRunner.IP(bootnode.ID)
		if err != nil {
			return false
		}
		ctx.bootnode = fmt.Sprintf("enode://%v@%v:33445", enodeSecret, bootnodeIP)
		return true
	})

	if err != nil {
		return err
	}
	return nil
}

func (ctx *Context) runGenesisValidator() error {
	spec := cluster.NewKcoinNodeBuilder().
		WithBootnode(ctx.bootnode).
		WithLogLevel(3).
		WithID("genesis-validator").
		WithSyncMode("full").
		WithNetworkId(ctx.chainID.String()).
		WithGenesis(ctx.genesis).
		WithAccount(ctx.AccountsStorage, ctx.genesisValidatorAccount).
		WithValidation().
		WithDeposit(big.NewInt(1)).
		NodeSpec()

	if err := ctx.nodeRunner.Run(spec); err != nil {
		return err
	}

	ctx.genesisValidatorNodeID = spec.ID
	return nil
}

func (ctx *Context) runRpc() error {
	spec := cluster.NewKcoinNodeBuilder().
		WithBootnode(ctx.bootnode).
		WithLogLevel(3).
		WithID("rpc").
		WithSyncMode("full").
		WithNetworkId(ctx.chainID.String()).
		WithGenesis(ctx.genesis).
		WithRpc(8080).
		NodeSpec()

	if err := ctx.nodeRunner.Run(spec); err != nil {
		return err
	}

	rpcIP, err := ctx.nodeRunner.IP(spec.ID)
	if err != nil {
		return err
	}

	rpcAddr := fmt.Sprintf("http://%v:%v", rpcIP, 8080)
	client, err := kcoinclient.Dial(rpcAddr)
	if err != nil {
		return err
	}

	ctx.client = client
	return nil
}

func (ctx *Context) triggerGenesisValidation() error {
	command := fmt.Sprintf(`
		personal.unlockAccount(eth.coinbase, "test");
		eth.sendTransaction({from:eth.coinbase,to: "%v",value: 1})
	`, ctx.seederAccount.Address.Hex())
	_, err := ctx.nodeRunner.Exec(ctx.genesisValidatorNodeID, cluster.KcoinExecCommand(command))
	if err != nil {
		return err
	}

	return common.WaitFor("validation starts", 2*time.Second, 20*time.Second, func() bool {
		res, err := ctx.nodeRunner.Exec(ctx.genesisValidatorNodeID, cluster.KcoinExecCommand("eth.blockNumber"))
		if err != nil {
			return false
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(res.StdOut))
		if err != nil {
			return false
		}
		return parsed > 0
	})
}

func (ctx *Context) buildGenesis() error {
	newGenesis, err := genesis.GenerateGenesis(
		genesis.Options{
			Network:                        "test",
			MaxNumValidators:               "5",
			UnbondingPeriod:                "5",
			AccountAddressGenesisValidator: ctx.genesisValidatorAccount.Address.Hex(),
			SmartContractsOwner:            "0x259be75d96876f2ada3d202722523e9cd4dd917d",
			PrefundedAccounts: []genesis.PrefundedAccount{
				{
					AccountAddress: ctx.genesisValidatorAccount.Address.Hex(),
					Balance:        "0x200000000000000000000000000000000000000000000000000000000000000",
				},
				{
					AccountAddress: ctx.seederAccount.Address.Hex(),
					Balance:        "0x200000000000000000000000000000000000000000000000000000000000000",
				},
			},
		},
	)
	if err != nil {
		return err
	}

	rawJson, err := json.Marshal(newGenesis)
	if err != nil {
		return err
	}
	ctx.genesis = rawJson

	return nil
}
