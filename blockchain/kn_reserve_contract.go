package blockchain

import (
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethereum "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type KNReserveContract struct {
	*KNContractBase
}

func (self *KNReserveContract) Withdraw(opts *bind.TransactOpts, token ethereum.Address, amount *big.Int, destination ethereum.Address) (*types.Transaction, error) {
	return self.KNContractBase.BuildTx(opts, "withdraw", token, amount, destination)
}

func NewKNReserveContract(address ethereum.Address, client *ethclient.Client) (*KNReserveContract, error) {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	file, err := os.Open(filepath.Join(exPath, "..", "blockchain", "reserve.abi"))
	if err != nil {
		return nil, err
	}
	parsed, err := abi.JSON(file)
	if err != nil {
		return nil, err
	}
	return &KNReserveContract{
		NewKNContractBase(address, parsed, client),
	}, nil
}
