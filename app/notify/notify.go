package notify

import (
	"encoding/json"
	"fmt"
	"github.com/v03413/bepusdt/app/config"
	e "github.com/v03413/bepusdt/app/epay"
	"github.com/v03413/bepusdt/app/help"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"io"
	"net/http"
	"strings"
	"time"
)

func Handle(order model.TradeOrders) {
	if order.ApiType == model.OrderApiTypeEpay {
		epay(order)

		return
	}

	epusdt(order)
}

func epay(order model.TradeOrders) {
	var client = http.Client{Timeout: time.Second * 5}
	var notifyUrl = fmt.Sprintf("%s?%s", order.NotifyUrl, e.BuildNotifyParams(order))

	postReq, err2 := http.NewRequest("GET", notifyUrl, nil)
	if err2 != nil {
		log.Error("Notify NewRequest Error：", err2)
		return
	}

	postReq.Header.Set("Powered-By", "https://github.com/v03413/bepusdt")
	resp, err := client.Do(postReq)
	if err != nil {
		log.Error("Notify Handle Error：", err)
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Warn(fmt.Sprintf("订单回调失败(%v)：resp.StatusCode != 200", order.OrderId), order.OrderSetNotifyState(model.OrderNotifyStateFail))
		return
	}

	all, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn(fmt.Sprintf("订单回调失败(%v)：io.ReadAll(resp.Body) Error:", order.OrderId), err, order.OrderSetNotifyState(model.OrderNotifyStateFail))
		return
	}

	// 解析响应
	var response struct {
		ReturnCode string `json:"return_code"`
		ResultCode string `json:"result_code"`
	}
	if err := json.Unmarshal(all, &response); err != nil {
		// 如果不是JSON格式，检查是否包含success（兼容旧版本）
		if !strings.Contains(strings.ToLower(string(all)), "success") {
			log.Warn(fmt.Sprintf("订单回调失败(%v)：响应不符合要求 (%s)", order.OrderId, string(all)), err, order.OrderSetNotifyState(model.OrderNotifyStateFail))
			return
		}
	} else {
		// 检查JSON响应是否符合要求
		if response.ReturnCode != "SUCCESS" || response.ResultCode != "SUCCESS" {
			log.Warn(fmt.Sprintf("订单回调失败(%v)：响应状态码错误 (%s)", order.OrderId, string(all)), err, order.OrderSetNotifyState(model.OrderNotifyStateFail))
			return
		}
	}

	err = order.OrderSetNotifyState(model.OrderNotifyStateSucc)
	if err != nil {
		log.Error("订单标记通知成功错误：", err, order.OrderId)
	} else {
		log.Info("订单通知成功：", order.OrderId)
	}
}

func epusdt(order model.TradeOrders) {
	var data = make(map[string]interface{})
	var body = struct {
		TradeId            string  `json:"trade_id"`             //  本地订单号
		OrderId            string  `json:"order_id"`             //  客户交易id
		Amount             float64 `json:"amount"`               //  订单金额 CNY
		ActualAmount       string  `json:"actual_amount"`        //  USDT 交易数额
		Token              string  `json:"token"`                //  收款钱包地址
		BlockTransactionId string  `json:"block_transaction_id"` // 区块id
		Signature          string  `json:"signature"`            // 签名
		Status             int     `json:"status"`               //  1：等待支付，2：支付成功，3：订单超时
	}{
		TradeId:            order.TradeId,
		OrderId:            order.OrderId,
		Amount:             order.Money,
		ActualAmount:       order.Amount,
		Token:              order.Address,
		BlockTransactionId: order.TradeHash,
		Status:             order.Status,
	}
	var jsonBody, err = json.Marshal(body)
	if err != nil {
		log.Error("Notify Json Marshal Error：", err)

		return
	}

	if err = json.Unmarshal(jsonBody, &data); err != nil {
		log.Error("Notify JSON Unmarshal Error：", err)

		return
	}

	// 签名
	body.Signature = help.GenerateSignature(data, config.GetAuthToken())

	// 再次序列化
	jsonBody, err = json.Marshal(body)
	var client = http.Client{Timeout: time.Second * 5}
	var postReq, err2 = http.NewRequest("POST", order.NotifyUrl, strings.NewReader(string(jsonBody)))
	if err2 != nil {
		log.Error("Notify NewRequest Error：", err)

		return
	}

	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Powered-By", "https://github.com/v03413/bepusdt")
	resp, err := client.Do(postReq)
	if err != nil {
		log.Error("Notify Handle Error：", err)

		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Warn(fmt.Sprintf("订单回调失败(%v)：resp.StatusCode != 200", order.OrderId), order.OrderSetNotifyState(model.OrderNotifyStateFail))

		return
	}

	all, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn(fmt.Sprintf("订单回调失败(%v)：io.ReadAll(resp.Body) Error:", order.OrderId), err, order.OrderSetNotifyState(model.OrderNotifyStateFail))

		return
	}

	if string(all) != "ok" {
		log.Warn(fmt.Sprintf("订单回调失败(%v)：body != ok (%s)", order.OrderId, string(all)), err, order.OrderSetNotifyState(model.OrderNotifyStateFail))

		return
	}

	err = order.OrderSetNotifyState(model.OrderNotifyStateSucc)
	if err != nil {
		log.Error("订单标记通知成功错误：", err, order.OrderId)
	} else {
		log.Info("订单通知成功：", order.OrderId)
	}
}
