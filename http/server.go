package http

import (
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/KyberNetwork/reserve-data"
	"github.com/KyberNetwork/reserve-data/common"
	"github.com/KyberNetwork/reserve-data/metric"
	"github.com/ethereum/go-ethereum/common/hexutil"
	raven "github.com/getsentry/raven-go"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sentry"
	"github.com/gin-gonic/gin"
)

type HTTPServer struct {
	app         reserve.ReserveData
	core        reserve.ReserveCore
	metric      metric.MetricStorage
	host        string
	authEnabled bool
	auth        Authentication
	r           *gin.Engine
}

const MAX_TIMESPOT uint64 = 18446744073709551615

func getTimePoint(c *gin.Context, useDefault bool) uint64 {
	timestamp := c.DefaultQuery("timestamp", "")
	if timestamp == "" {
		if useDefault {
			log.Printf("Interpreted timestamp to default - %d\n", MAX_TIMESPOT)
			return MAX_TIMESPOT
		} else {
			timepoint := common.GetTimepoint()
			log.Printf("Interpreted timestamp to current time - %d\n", timepoint)
			return uint64(timepoint)
		}
	} else {
		timepoint, err := strconv.ParseUint(timestamp, 10, 64)
		if err != nil {
			log.Printf("Interpreted timestamp(%s) to default - %s\n", timestamp, MAX_TIMESPOT)
			return MAX_TIMESPOT
		} else {
			log.Printf("Interpreted timestamp(%s) to %s\n", timestamp, timepoint)
			return timepoint
		}
	}
}

func IsIntime(nonce string) bool {
	serverTime := common.GetTimepoint()
	log.Printf("Server time: %d, None: %d", serverTime, nonce)
	nonceInt, err := strconv.ParseInt(nonce, 10, 64)
	if err != nil {
		log.Printf("IsIntime returns false, err: %v", err)
		return false
	}
	difference := nonceInt - int64(serverTime)
	if difference < -30000 || difference > 30000 {
		log.Printf("IsIntime returns false, nonce: %d, serverTime: %d, difference: %d", nonceInt, int64(serverTime), difference)
		return false
	}
	return true
}

func eligible(ups, allowedPerms []Permission) bool {
	for _, up := range ups {
		for _, ap := range allowedPerms {
			if up == ap {
				return true
			}
		}
	}
	return false
}

// signed message (message = url encoded both query params and post params, keys are sorted) in "signed" header
// using HMAC512
// params must contain "nonce" which is the unixtime in millisecond. The nonce will be invalid
// if it differs from server time more than 10s
func (self *HTTPServer) Authenticated(c *gin.Context, requiredParams []string, perms []Permission) (url.Values, bool) {
	err := c.Request.ParseForm()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": false,
				"reason":  "Malformed request package",
			},
		)
		return c.Request.Form, false
	}

	if !self.authEnabled {
		return c.Request.Form, true
	}

	params := c.Request.Form
	log.Printf("Form params: %s\n", params)
	if !IsIntime(params.Get("nonce")) {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": false,
				"reason":  "Your nonce is invalid",
			},
		)
		return c.Request.Form, false
	}

	for _, p := range requiredParams {
		if params.Get(p) == "" {
			c.JSON(
				http.StatusOK,
				gin.H{
					"success": false,
					"reason":  fmt.Sprintf("Required param (%s) is missing. Param name is case sensitive", p),
				},
			)
			return c.Request.Form, false
		}
	}

	signed := c.GetHeader("signed")
	message := c.Request.Form.Encode()
	userPerms := self.auth.GetPermission(signed, message)
	if eligible(userPerms, perms) {
		return params, true
	} else {
		if len(userPerms) == 0 {
			c.JSON(
				http.StatusOK,
				gin.H{
					"success": false,
					"reason":  "Invalid signed token",
				},
			)
		} else {
			c.JSON(
				http.StatusOK,
				gin.H{
					"success": false,
					"reason":  "You don't have permission to proceed",
				},
			)
		}
		return params, false
	}
}

func (self *HTTPServer) AllPricesVersion(c *gin.Context) {
	log.Printf("Getting all prices version")
	data, err := self.app.CurrentPriceVersion(getTimePoint(c, true))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"version": data,
			},
		)
	}
}

func (self *HTTPServer) AllPrices(c *gin.Context) {
	log.Printf("Getting all prices \n")
	data, err := self.app.GetAllPrices(getTimePoint(c, true))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success":   true,
				"version":   data.Version,
				"timestamp": data.Timestamp,
				"data":      data.Data,
				"block":     data.Block,
			},
		)
	}
}

func (self *HTTPServer) Price(c *gin.Context) {
	base := c.Param("base")
	quote := c.Param("quote")
	log.Printf("Getting price for %s - %s \n", base, quote)
	pair, err := common.NewTokenPair(base, quote)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": "Token pair is not supported"},
		)
	} else {
		data, err := self.app.GetOnePrice(pair.PairID(), getTimePoint(c, true))
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
		} else {
			c.JSON(
				http.StatusOK,
				gin.H{
					"success":   true,
					"version":   data.Version,
					"timestamp": data.Timestamp,
					"exchanges": data.Data,
				},
			)
		}
	}
}

func (self *HTTPServer) AuthDataVersion(c *gin.Context) {
	log.Printf("Getting current auth data snapshot version")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}

	data, err := self.app.CurrentAuthDataVersion(getTimePoint(c, true))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"version": data,
			},
		)
	}
}

func (self *HTTPServer) AuthData(c *gin.Context) {
	log.Printf("Getting current auth data snapshot \n")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}

	data, err := self.app.GetAuthData(getTimePoint(c, true))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success":   true,
				"version":   data.Version,
				"timestamp": data.Timestamp,
				"data":      data.Data,
			},
		)
	}
}

func (self *HTTPServer) GetRates(c *gin.Context) {
	log.Printf("Getting all rates \n")
	fromTime, _ := strconv.ParseUint(c.Query("fromTime"), 10, 64)
	toTime, _ := strconv.ParseUint(c.Query("toTime"), 10, 64)
	if toTime == 0 {
		toTime = MAX_TIMESPOT
	}
	data, err := self.app.GetRates(fromTime, toTime)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    data,
			},
		)
	}
}

func (self *HTTPServer) GetRate(c *gin.Context) {
	log.Printf("Getting all rates \n")
	data, err := self.app.GetRate(getTimePoint(c, true))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success":   true,
				"version":   data.Version,
				"timestamp": data.Timestamp,
				"data":      data.Data,
			},
		)
	}
}

func (self *HTTPServer) SetRate(c *gin.Context) {
	postForm, ok := self.Authenticated(c, []string{"tokens", "buys", "sells", "block", "afp_mid"}, []Permission{RebalancePermission})
	if !ok {
		return
	}
	tokenAddrs := postForm.Get("tokens")
	buys := postForm.Get("buys")
	sells := postForm.Get("sells")
	block := postForm.Get("block")
	afpMid := postForm.Get("afp_mid")
	tokens := []common.Token{}
	for _, tok := range strings.Split(tokenAddrs, "-") {
		token, err := common.GetToken(tok)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		} else {
			tokens = append(tokens, token)
		}
	}
	bigBuys := []*big.Int{}
	for _, rate := range strings.Split(buys, "-") {
		r, err := hexutil.DecodeBig(rate)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		} else {
			bigBuys = append(bigBuys, r)
		}
	}
	bigSells := []*big.Int{}
	for _, rate := range strings.Split(sells, "-") {
		r, err := hexutil.DecodeBig(rate)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		} else {
			bigSells = append(bigSells, r)
		}
	}
	intBlock, err := strconv.ParseInt(block, 10, 64)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	bigAfpMid := []*big.Int{}
	for _, rate := range strings.Split(afpMid, "-") {
		r, err := hexutil.DecodeBig(rate)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		} else {
			bigAfpMid = append(bigAfpMid, r)
		}
	}
	id, err := self.core.SetRates(tokens, bigBuys, bigSells, big.NewInt(intBlock), bigAfpMid)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"id":      id,
			},
		)
	}
}

func (self *HTTPServer) Trade(c *gin.Context) {
	postForm, ok := self.Authenticated(c, []string{"base", "quote", "amount", "rate", "type"}, []Permission{RebalancePermission})
	if !ok {
		return
	}

	exchangeParam := c.Param("exchangeid")
	baseTokenParam := postForm.Get("base")
	quoteTokenParam := postForm.Get("quote")
	amountParam := postForm.Get("amount")
	rateParam := postForm.Get("rate")
	typeParam := postForm.Get("type")

	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	base, err := common.GetToken(baseTokenParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	quote, err := common.GetToken(quoteTokenParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	amount, err := strconv.ParseFloat(amountParam, 64)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	rate, err := strconv.ParseFloat(rateParam, 64)
	log.Printf("http server: Trade: rate: %f, raw rate: %s", rate, rateParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	if typeParam != "sell" && typeParam != "buy" {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": fmt.Sprintf("Trade type of %s is not supported.", typeParam)},
		)
		return
	}
	id, done, remaining, finished, err := self.core.Trade(
		exchange, typeParam, base, quote, rate, amount, getTimePoint(c, false))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success":   true,
			"id":        id,
			"done":      done,
			"remaining": remaining,
			"finished":  finished,
		},
	)
}

func (self *HTTPServer) CancelOrder(c *gin.Context) {
	postForm, ok := self.Authenticated(c, []string{"order_id"}, []Permission{RebalancePermission})
	if !ok {
		return
	}

	exchangeParam := c.Param("exchangeid")
	id := postForm.Get("order_id")

	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	log.Printf("Cancel order id: %s from %s\n", id, exchange.ID())
	activityID, err := common.StringToActivityID(id)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	err = self.core.CancelOrder(activityID, exchange)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
}

func (self *HTTPServer) Withdraw(c *gin.Context) {
	postForm, ok := self.Authenticated(c, []string{"token", "amount"}, []Permission{RebalancePermission})
	if !ok {
		return
	}

	exchangeParam := c.Param("exchangeid")
	tokenParam := postForm.Get("token")
	amountParam := postForm.Get("amount")

	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	token, err := common.GetToken(tokenParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	amount, err := hexutil.DecodeBig(amountParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	log.Printf("Withdraw %s %s from %s\n", amount.Text(10), token.ID, exchange.ID())
	id, err := self.core.Withdraw(exchange, token, amount, getTimePoint(c, false))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"id":      id,
		},
	)
}

func (self *HTTPServer) Deposit(c *gin.Context) {
	postForm, ok := self.Authenticated(c, []string{"amount", "token"}, []Permission{RebalancePermission})
	if !ok {
		return
	}

	exchangeParam := c.Param("exchangeid")
	amountParam := postForm.Get("amount")
	tokenParam := postForm.Get("token")

	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	token, err := common.GetToken(tokenParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	amount, err := hexutil.DecodeBig(amountParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	log.Printf("Depositing %s %s to %s\n", amount.Text(10), token.ID, exchange.ID())
	id, err := self.core.Deposit(exchange, token, amount, getTimePoint(c, false))
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"id":      id,
		},
	)
}

func (self *HTTPServer) GetActivities(c *gin.Context) {
	log.Printf("Getting all activity records \n")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	fromTime, _ := strconv.ParseUint(c.Query("fromTime"), 10, 64)
	toTime, _ := strconv.ParseUint(c.Query("toTime"), 10, 64)

	data, err := self.app.GetRecords(fromTime*1000000, toTime*1000000)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    data,
			},
		)
	}
}

func (self *HTTPServer) TradeLogs(c *gin.Context) {
	log.Printf("Getting trade logs")
	fromTime, err := strconv.ParseUint(c.Query("fromTime"), 10, 64)
	if err != nil {
		fromTime = 0
	}
	toTime, err := strconv.ParseUint(c.Query("toTime"), 10, 64)
	if err != nil {
		toTime = uint64(time.Now().UnixNano())
	}

	data, err := self.app.GetTradeLogs(fromTime, toTime)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    data,
			},
		)
	}
}

func (self *HTTPServer) StopFetcher(c *gin.Context) {
	err := self.app.Stop()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
			},
		)
	}
}

func (self *HTTPServer) ImmediatePendingActivities(c *gin.Context) {
	log.Printf("Getting all immediate pending activity records \n")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}

	data, err := self.app.GetPendingActivities()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    data,
			},
		)
	}
}

func (self *HTTPServer) Metrics(c *gin.Context) {
	response := metric.MetricResponse{
		Timestamp: common.GetTimepoint(),
	}
	log.Printf("Getting metrics")
	postForm, ok := self.Authenticated(c, []string{"tokens", "from", "to"}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	tokenParam := postForm.Get("tokens")
	fromParam := postForm.Get("from")
	toParam := postForm.Get("to")
	tokens := []common.Token{}
	for _, tok := range strings.Split(tokenParam, "-") {
		token, err := common.GetToken(tok)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		} else {
			tokens = append(tokens, token)
		}
	}
	from, err := strconv.ParseUint(fromParam, 10, 64)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	}
	to, err := strconv.ParseUint(toParam, 10, 64)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	}
	data, err := self.metric.GetMetric(tokens, from, to)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	}
	response.ReturnTime = common.GetTimepoint()
	response.Data = data
	c.JSON(
		http.StatusOK,
		gin.H{
			"success":    true,
			"timestamp":  response.Timestamp,
			"returnTime": response.ReturnTime,
			"data":       response.Data,
		},
	)
}

func (self *HTTPServer) StoreMetrics(c *gin.Context) {
	log.Printf("Storing metrics")
	postForm, ok := self.Authenticated(c, []string{"timestamp", "data"}, []Permission{RebalancePermission})
	if !ok {
		return
	}
	timestampParam := postForm.Get("timestamp")
	dataParam := postForm.Get("data")

	timestamp, err := strconv.ParseUint(timestampParam, 10, 64)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	}
	metricEntry := metric.MetricEntry{}
	metricEntry.Timestamp = timestamp
	metricEntry.Data = map[string]metric.TokenMetric{}
	// data must be in form of <token>_afpmid_spread|<token>_afpmid_spread|...
	for _, tokenData := range strings.Split(dataParam, "|") {
		parts := strings.Split(tokenData, "_")
		if len(parts) != 3 {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": "submitted data is not in correct format"},
			)
			return
		}
		token := parts[0]
		afpmidStr := parts[1]
		spreadStr := parts[2]

		afpmid, err := strconv.ParseFloat(afpmidStr, 64)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": "Afp mid " + afpmidStr + " is not float64"},
			)
			return
		}
		spread, err := strconv.ParseFloat(spreadStr, 64)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": "Spread " + spreadStr + " is not float64"},
			)
			return
		}
		metricEntry.Data[token] = metric.TokenMetric{
			AfpMid: afpmid,
			Spread: spread,
		}
	}

	err = self.metric.StoreMetric(&metricEntry, common.GetTimepoint())
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
			},
		)
	}
}

func (self *HTTPServer) GetExchangeInfo(c *gin.Context) {
	exchangeParam := c.Query("exchangeid")
	if exchangeParam == "" {
		data := map[string]map[common.TokenPairID]common.ExchangePrecisionLimit{}
		for _, ex := range common.SupportedExchanges {
			exchangeInfo, err := ex.GetInfo()
			if err != nil {
				c.JSON(
					http.StatusOK,
					gin.H{"success": false, "reason": err.Error()},
				)
				return
			}
			data[string(ex.ID())] = exchangeInfo.GetData()
		}
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    data,
			},
		)
	} else {
		exchange, err := common.GetExchange(exchangeParam)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		}
		exchangeInfo, err := exchange.GetInfo()
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		}
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": true,
				"data":    exchangeInfo.GetData(),
			},
		)
	}
}

func (self *HTTPServer) GetPairInfo(c *gin.Context) {
	exchangeParam := c.Param("exchangeid")
	base := c.Param("base")
	quote := c.Param("quote")
	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	pair, err := common.NewTokenPair(base, quote)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	pairInfo, err := exchange.GetExchangeInfo(pair.PairID())
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": pairInfo},
	)
	return
}

func (self *HTTPServer) GetExchangeFee(c *gin.Context) {
	exchangeParam := c.Param("exchangeid")
	exchange, err := common.GetExchange(exchangeParam)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	fee := exchange.GetFee()
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": fee},
	)
	return
}

func (self *HTTPServer) GetFee(c *gin.Context) {
	data := map[string]common.ExchangeFees{}
	for _, exchange := range common.SupportedExchanges {
		fee := exchange.GetFee()
		data[string(exchange.ID())] = fee
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": data},
	)
	return
}

func (self *HTTPServer) GetTargetQty(c *gin.Context) {
	log.Println("Getting target quantity")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	data, err := self.metric.GetTokenTargetQty()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
	} else {
		c.JSON(
			http.StatusOK,
			gin.H{"success": true, "data": data},
		)
	}
}

func (self *HTTPServer) GetPendingTargetQty(c *gin.Context) {
	log.Println("Getting pending target qty")
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	data, err := self.metric.GetPendingTargetQty()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": data},
	)
	return
}

func targetQtySanityCheck(total, reserve, rebalanceThresold, transferThresold float64) error {
	if total <= reserve {
		return errors.New("Total quantity must bigger than reserver quantity")
	}
	if rebalanceThresold < 0 || rebalanceThresold > 1 || transferThresold < 0 || transferThresold > 1 {
		return errors.New("Rebalance and transfer thresold must bigger than 0 and smaller than 1")
	}
	return nil
}

func (self *HTTPServer) ConfirmTargetQty(c *gin.Context) {
	log.Println("Confirm target quantity")
	postForm, ok := self.Authenticated(c, []string{"data", "type"}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	data := postForm.Get("data")
	id := postForm.Get("id")
	err := self.metric.StoreTokenTargetQty(id, data)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true},
	)
	return
}

func (self *HTTPServer) CancelTargetQty(c *gin.Context) {
	log.Println("Cancel target quantity")
	_, ok := self.Authenticated(c, []string{}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	err := self.metric.RemovePendingTargetQty()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true},
	)
	return
}

func (self *HTTPServer) SetTargetQty(c *gin.Context) {
	log.Println("Storing target quantity")
	postForm, ok := self.Authenticated(c, []string{"data", "type"}, []Permission{ConfigurePermission})
	if !ok {
		return
	}
	data := postForm.Get("data")
	dataType := postForm.Get("type")
	log.Println("Setting target qty")
	var err error
	for _, dataConfig := range strings.Split(data, "|") {
		dataParts := strings.Split(dataConfig, "_")
		if dataType == "" || (dataType == "1" && len(dataParts) != 5) {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": "Data submitted not enough information"},
			)
			return
		}
		token := dataParts[0]
		total, _ := strconv.ParseFloat(dataParts[1], 64)
		reserve, _ := strconv.ParseFloat(dataParts[2], 64)
		rebalanceThresold, _ := strconv.ParseFloat(dataParts[3], 64)
		transferThresold, _ := strconv.ParseFloat(dataParts[4], 64)
		_, err = common.GetToken(token)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		}
		err = targetQtySanityCheck(total, reserve, rebalanceThresold, transferThresold)
		if err != nil {
			c.JSON(
				http.StatusOK,
				gin.H{"success": false, "reason": err.Error()},
			)
			return
		}
	}
	err = self.metric.StorePendingTargetQty(data, dataType)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}

	pendingData, err := self.metric.GetPendingTargetQty()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{"success": false, "reason": err.Error()},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": pendingData},
	)
	return
}

func (self *HTTPServer) GetAddress(c *gin.Context) {
	c.JSON(
		http.StatusOK,
		gin.H{"success": true, "data": self.core.GetAddresses()},
	)
	return
}

func (self *HTTPServer) GetTradeHistory(c *gin.Context) {
	timepoint := common.GetTimepoint()
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}

	data, err := self.app.GetTradeHistory(timepoint)
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": false,
				"data":    err.Error(),
			},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func (self *HTTPServer) GetTimeServer(c *gin.Context) {
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    common.GetTimestamp(),
		},
	)
}

func (self *HTTPServer) GetRebalanceStatus(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	data, err := self.metric.GetRebalanceControl()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": false,
				"reason":  err.Error(),
			},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    data.Status,
		},
	)
}

func (self *HTTPServer) HoldRebalance(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	self.metric.StoreRebalanceControl(false)
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
	return
}

func (self *HTTPServer) EnableRebalance(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	self.metric.StoreRebalanceControl(true)
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
	return
}

func (self *HTTPServer) GetSetrateStatus(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ReadOnlyPermission, RebalancePermission, ConfigurePermission, ConfirmConfPermission})
	if !ok {
		return
	}
	data, err := self.metric.GetSetrateControl()
	if err != nil {
		c.JSON(
			http.StatusOK,
			gin.H{
				"success": false,
			},
		)
		return
	}
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    data.Status,
		},
	)
}

func (self *HTTPServer) HoldSetrate(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	self.metric.StoreSetrateControl(false)
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
	return
}

func (self *HTTPServer) EnableSetrate(c *gin.Context) {
	_, ok := self.Authenticated(c, []string{}, []Permission{ConfirmConfPermission})
	if !ok {
		return
	}
	self.metric.StoreSetrateControl(true)
	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
		},
	)
	return
}

func (self *HTTPServer) Run() {
	self.r.GET("/prices-version", self.AllPricesVersion)
	self.r.GET("/prices", self.AllPrices)
	self.r.GET("/prices/:base/:quote", self.Price)
	self.r.GET("/getrates", self.GetRate)
	self.r.GET("/get-all-rates", self.GetRates)

	self.r.GET("/authdata-version", self.AuthDataVersion)
	self.r.GET("/authdata", self.AuthData)
	self.r.GET("/activities", self.GetActivities)
	self.r.GET("/immediate-pending-activities", self.ImmediatePendingActivities)
	self.r.GET("/tradelogs", self.TradeLogs)
	self.r.GET("/metrics", self.Metrics)
	self.r.POST("/metrics", self.StoreMetrics)

	self.r.POST("/cancelorder/:exchangeid", self.CancelOrder)
	self.r.POST("/deposit/:exchangeid", self.Deposit)
	self.r.POST("/withdraw/:exchangeid", self.Withdraw)
	self.r.POST("/trade/:exchangeid", self.Trade)
	self.r.POST("/setrates", self.SetRate)
	self.r.GET("/exchangeinfo", self.GetExchangeInfo)
	self.r.GET("/exchangeinfo/:exchangeid/:base/:quote", self.GetPairInfo)
	self.r.GET("/exchangefees", self.GetFee)
	self.r.GET("/exchangefees/:exchangeid", self.GetExchangeFee)
	self.r.GET("/core/addresses", self.GetAddress)
	self.r.GET("/tradehistory", self.GetTradeHistory)

	self.r.GET("/targetqty", self.GetTargetQty)
	self.r.GET("/pendingtargetqty", self.GetPendingTargetQty)
	self.r.POST("/settargetqty", self.SetTargetQty)
	self.r.POST("/confirmtargetqty", self.ConfirmTargetQty)
	self.r.POST("/canceltargetqty", self.CancelTargetQty)

	self.r.GET("/timeserver", self.GetTimeServer)

	self.r.GET("/rebalancestatus", self.GetRebalanceStatus)
	self.r.POST("/holdrebalance", self.HoldRebalance)
	self.r.POST("/enablerebalance", self.EnableRebalance)

	self.r.GET("/setratestatus", self.GetSetrateStatus)
	self.r.POST("/holdsetrate", self.HoldSetrate)
	self.r.POST("/enablesetrate", self.EnableSetrate)

	self.r.Run(self.host)
}

func NewHTTPServer(
	app reserve.ReserveData,
	core reserve.ReserveCore,
	metric metric.MetricStorage,
	host string,
	enableAuth bool,
	authEngine Authentication,
	env string) *HTTPServer {

	r := gin.Default()
	sentryCli, err := raven.NewWithTags(
		"https://bf15053001464a5195a81bc41b644751:eff41ac715114b20b940010208271b13@sentry.io/228067",
		map[string]string{
			"env": env,
		},
	)
	if err != nil {
		panic(err)
	}
	r.Use(sentry.Recovery(
		sentryCli,
		false,
	))
	corsConfig := cors.DefaultConfig()
	corsConfig.AddAllowHeaders("signed")
	corsConfig.AllowAllOrigins = true
	corsConfig.MaxAge = 5 * time.Minute
	r.Use(cors.New(corsConfig))

	return &HTTPServer{
		app, core, metric, host, enableAuth, authEngine, r,
	}
}
