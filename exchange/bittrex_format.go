package exchange

// map of token pair to map of asks/bids to array of [rate, amount]
type Bittresp struct {
	Success bool                            `json:"success"`
	Msg     string                          `json:"message"`
	Result  map[string][]map[string]float64 `json:"result"`
}

type Bittinfo struct {
	Success bool `json:"success"`
	Result  []struct {
		Currency      string  `json:"Currency"`
		Balance       float64 `json:"Balance"`
		Available     float64 `json:"Available"`
		Pending       float64 `json:"Pending"`
		CryptoAddress string  `json:"CryptoAddress"`
		Requested     bool    `json:"Requested"`
		Uuid          string  `json:"uuid"`
	} `json:"result"`
	Error string `json:"message"`
}

type BittrexDepositAddress struct {
	Success string `json:"success"`
	Message string `json:"message"`
	Result  struct {
		Currency string `json:"Currency"`
		Address  string `json:"Address"`
	} `json:"result"`
}

type BittPairInfo struct {
	Base      string  `json:"MarketCurrency"`
	Quote     string  `json:"BaseCurrency"`
	MinAmount float64 `json:"MinTradeSize"`
}

type BittExchangeInfo struct {
	Pairs []BittPairInfo `json:"result"`
}

type Bittwithdraw struct {
	Success bool              `json:"success"`
	Result  map[string]string `json:"result"`
	Error   string            `json:"message"`
}

type Bitttraderesult struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
	Result  struct {
		AccountId                  string
		OrderUuid                  string
		Exchange                   string
		Type                       string
		Quantity                   float64
		QuantityRemaining          float64
		Limit                      float64
		Reserved                   float64
		ReserveRemaining           float64
		CommissionReserved         float64
		CommissionReserveRemaining float64
		CommissionPaid             float64
		Price                      float64
		PricePerUnit               float64
		Opened                     string
		Closed                     string
		IsOpen                     bool
		Sentinel                   string
		CancelInitiated            bool
		ImmediateOrCancel          bool
		IsConditional              bool
		Condition                  string
		ConditionTarget            string
	} `json:"result"`
}

type Bitttrade struct {
	Success bool              `json:"success"`
	Error   string            `json:"message"`
	Result  map[string]string `json:"result"`
}

type Bittcancelorder struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
}

type Bittwithdrawhistory struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
	Result  []struct {
		PaymentUuid    string
		Currency       string
		Amount         float64
		Address        string
		Opened         string
		Authorized     bool
		PendingPayment bool
		TxCost         float64
		TxId           string
		Canceled       bool
		InvalidAddress bool
	} `json:"result"`
}

type Bittdeposithistory struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
	Result  []struct {
		Id            uint64
		Currency      string
		Amount        float64
		CryptoAddress string
		TxId          string
		Confirmations int
		LastUpdated   string
	} `json:"result"`
}

type BittTradeHistory struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Result  []struct {
		OrderUuid         string  `json:"OrderUuid"`
		Exchange          string  `json:"Exchange"`
		TimeStamp         string  `json:"TimeStamp"`
		OrderType         string  `json:"OrderType"`
		Limit             float64 `json:"Limit"`
		Quantity          float64 `json:"Quantity"`
		QuantityRemaining float64 `json:"QuantityRemaining"`
		Commission        float64 `json:"Commission"`
		Price             float64 `json:"Price"`
		IsConditional     bool    `json:"IsConditional"`
		ImmediateOrCancel bool    `json:"ImmediateOrCancel"`
	} `json:"result"`
}
