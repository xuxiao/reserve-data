package configuration

import (
	"log"
	"time"

	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/data/fetcher"
	"github.com/KyberNetwork/reserve-data/data/storage"
	"github.com/KyberNetwork/reserve-data/http"
	"github.com/KyberNetwork/reserve-data/signer"
	ethereum "github.com/ethereum/go-ethereum/common"
)

func GetConfigForDev() *Config {
	// settingPath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/dev_setting.json"
	settingPath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/staging_setting.json"
	addressConfig, err := common.GetAddressConfigFromFile(settingPath)
	if err != nil {
		log.Fatalf("Config file %s is not found. Error: %s", settingPath, err)
	}
	feePath := "/go/src/github.com/KyberNetwork/reserve-data/cmd/fee.json"
	feeConfig, err := common.GetFeeFromFile(feePath)
	if err != nil {
		log.Fatalf("Fees file cannot found at: %s", feePath, err)
	}
	wrapperAddr := ethereum.HexToAddress(addressConfig.Wrapper)
	reserveAddr := ethereum.HexToAddress(addressConfig.Reserve)
	pricingAddr := ethereum.HexToAddress(addressConfig.Pricing)
	burnerAddr := ethereum.HexToAddress(addressConfig.FeeBurner)
	networkAddr := ethereum.HexToAddress(addressConfig.Network)

	common.SupportedTokens = map[string]common.Token{}
	tokens := []common.Token{}
	for id, t := range addressConfig.Tokens {
		tok := common.Token{
			id, t.Address, t.Decimals,
		}
		common.SupportedTokens[id] = tok
		tokens = append(tokens, tok)
	}

	storage, err := storage.NewBoltStorage("/go/src/github.com/KyberNetwork/reserve-data/cmd/dev.db")
	if err != nil {
		panic(err)
	}

	fetcherRunner := fetcher.NewTickerRunner(3*time.Second, 2*time.Second, 3*time.Second, 5*time.Second, 5*time.Second)

	fileSigner, depositSigner := signer.NewFileSigner("/go/src/github.com/KyberNetwork/reserve-data/cmd/config.json")

	exchangePool := NewDevExchangePool(
		feeConfig, addressConfig, fileSigner, storage,
	)

	// endpoint := "https://ropsten.infura.io"
	// endpoint := "http://blockchain:8545"
	// endpoint := "https://kovan.infura.io"
	endpoint := "https://mainnet.infura.io"
	bkendpoints := []string{
		"https://mainnet.infura.io",
	}

	hmac512auth := http.KNAuthentication{
		fileSigner.KNSecret,
		fileSigner.KNReadOnly,
		fileSigner.KNConfiguration,
		fileSigner.KNConfirmConf,
	}

	return &Config{
		ActivityStorage:         storage,
		DataStorage:             storage,
		FetcherStorage:          storage,
		MetricStorage:           storage,
		FetcherRunner:           fetcherRunner,
		FetcherExchanges:        exchangePool.FetcherExchanges(),
		Exchanges:               exchangePool.CoreExchanges(),
		BlockchainSigner:        fileSigner,
		EnableAuthentication:    true,
		DepositSigner:           depositSigner,
		AuthEngine:              hmac512auth,
		EthereumEndpoint:        endpoint,
		BackupEthereumEndpoints: bkendpoints,
		SupportedTokens:         tokens,
		WrapperAddress:          wrapperAddr,
		PricingAddress:          pricingAddr,
		ReserveAddress:          reserveAddr,
		FeeBurnerAddress:        burnerAddr,
		NetworkAddress:          networkAddr,
	}
}
