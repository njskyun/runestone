package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"fmt"
	"net/http"

	"github.com/btcsuite/btcd/wire"
	"github.com/bxelab/runestone"
	"github.com/spf13/viper"
	"golang.org/x/text/message"
)

var (
	lastRequestTime time.Time
	sharedAvgFee10  int64
	mu              sync.Mutex
)

type FeeData struct {
	AvgFee10 int64 `json:"avgFee_90"` //avgFee_10 avgFee_25 avgFee_50 avgFee_75 avgFee_90
}

var config = DefaultConfig()
var p *message.Printer
var walletName = ""

func main() {
	p = message.NewPrinter(lang)
	loadConfig()
	checkAndPrintConfig()

	BuildMintTxs()
}

func getrawtransaction(txhash string) (map[string]interface{}, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      "getrawtransaction",
		"method":  "getrawtransaction",
		"params":  []interface{}{txhash, 1},
	})
	if err != nil {
		return nil, err
	}

	localrpc := config.GetLocalRpcUrl()

	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return nil, fmt.Errorf("RPC error: %v", errMsg)
	}

	return result["result"].(map[string]interface{}), nil
}

func returnReplaceTxUtxos(txhash string) ([]*Utxo, error) {
	originTransaction, err := getrawtransaction(txhash)
	if err != nil {
		p.Println("Error:", err)
		return nil, err
	}

	var inputUtxos []*Utxo

	// 处理 vin
	vins := originTransaction["vin"].([]interface{})
	for _, vin := range vins {
		vinMap := vin.(map[string]interface{})
		txID := vinMap["txid"].(string)
		voutIndex := vinMap["vout"].(float64)

		oldTransaction, err := getrawtransaction(txID)
		if err != nil {
			return nil, err
		}

		var script string
		var amount float64
		oldVout := oldTransaction["vout"].([]interface{})
		for _, output := range oldVout {
			outputMap := output.(map[string]interface{})
			if outputMap["n"].(float64) == voutIndex {
				script = outputMap["scriptPubKey"].(map[string]interface{})["hex"].(string)
				amount = outputMap["value"].(float64)
				break
			}
		}

		byteScript, err := hex.DecodeString(script)
		if err != nil {
			return nil, err // 添加错误处理
		}

		newUtxo := &Utxo{
			TxHash:        HexToHash(txID),
			Index:         uint32(voutIndex),
			Value:         int64(amount * 1e8),
			PkScript:      byteScript,
			Ancestorfees:  0,
			Confirmations: 0,
			Ancestorsize:  int64(originTransaction["vsize"].(float64)),
			Ancestorcount: 0,
		}

		inputUtxos = append(inputUtxos, newUtxo)
	}

	return inputUtxos, nil
}

func getUtxos(address string) ([]*Utxo, error) {
	localrpc := config.GetLocalRpcUrl()
	url := fmt.Sprintf(localrpc+"/wallet/%s", config.GetWalletName())
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      "getUtxos",
		"method":  "listunspent",
		"params":  []interface{}{0, 9999999, []string{address}},
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return nil, fmt.Errorf("error: %v", errMsg)
	}

	utxos, ok := result["result"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to retrieve UTXOs")
	}

	var inputUtxos []*Utxo
	for _, utxo := range utxos {
		utxoMap, ok := utxo.(map[string]interface{})
		if !ok {
			p.Println("Failed to assert UTXO as map[string]interface{}")
			continue
		}

		txid, ok := utxoMap["txid"].(string)
		if !ok {
			p.Println("Failed to assert txid as string")
			continue
		}

		vout, ok := utxoMap["vout"].(float64)
		if !ok {
			p.Println("Failed to assert vout as float64")
			continue
		}

		amount, ok := utxoMap["amount"].(float64)
		if !ok {
			p.Println("Failed to assert amount as float64")
			continue
		}

		script, ok := utxoMap["scriptPubKey"].(string)
		if !ok {
			p.Println("Failed to assert scriptPubKey as string")
			continue
		}

		h := HexToHash(txid)

		byteScript, err := hex.DecodeString(script)
		if err != nil {
			fmt.Println("Error decoding hex:", err)
			continue
		}

		var ancestorFees int64
		if dealAncestorFees, exists := utxoMap["ancestorfees"]; exists {
			switch v := dealAncestorFees.(type) {
			case string:
				strFee := strings.ReplaceAll(v, ",", "")
				if parsedFees, err := strconv.ParseInt(strFee, 10, 64); err == nil {
					ancestorFees = parsedFees
				}
			case float64:
				ancestorFees = int64(v)
			}
		}

		var ancestorsize int64
		if dealAncestorsize, exists := utxoMap["ancestorsize"]; exists {
			switch v := dealAncestorsize.(type) {
			case string:
				ssize := strings.ReplaceAll(v, ",", "")
				if parsedSize, err := strconv.ParseInt(ssize, 10, 64); err == nil {
					ancestorsize = parsedSize
				}
			case float64:
				ancestorsize = int64(v)
			}
		}

		confirmations, ok := utxoMap["confirmations"].(float64)
		if !ok {
			p.Println("Failed to assert confirmations as float64")
			continue
		}

		ancestorcount, ok := utxoMap["ancestorcount"].(float64)
		if !ok {
			ancestorcount = 0
		}

		if int64(amount*1e8) > 100000 {
			// p.Println("input Txid: ", h, "; vout:", vout, "; amount: ", amount)

			newUtxo := &Utxo{
				TxHash:        h,
				Index:         uint32(vout),
				Value:         int64(amount * 1e8),
				PkScript:      byteScript,
				Ancestorfees:  ancestorFees,
				Confirmations: int64(confirmations),
				Ancestorsize:  ancestorsize,
				Ancestorcount: int64(ancestorcount),
			}

			inputUtxos = append(inputUtxos, newUtxo)
		}
	}

	return inputUtxos, nil
}

func sendRawTransaction(txHex string) (string, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      txHex,
		"method":  "sendrawtransaction",
		"params":  []interface{}{txHex},
	})
	if err != nil {
		return "", err
	}

	localrpc := config.GetLocalRpcUrl()
	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取并打印响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// log.Printf("Request txHex: %s", txHex) // 打印请求的交易数据
	// log.Printf("Response body: %s", body) // 打印响应体

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return "", fmt.Errorf("error: %v", errMsg)
	}

	txID, ok := result["result"].(string)
	if !ok {
		return "", fmt.Errorf("failed to retrieve transaction ID")
	}

	return txID, nil
}

func fetchAvgFee() (int64, error) {
	mu.Lock()
	defer mu.Unlock()

	currentTime := time.Now()

	if currentTime.Sub(lastRequestTime) >= 90*time.Second {
		url := "https://mempool.fractalbitcoin.io/api/v1/mining/blocks/fee-rates/100m"
		resp, err := http.Get(url)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("failed to fetch data: %s", resp.Status)
		}

		var data []FeeData
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return 0, err
		}

		// 获取最后一个元素
		if len(data) > 0 {
			lastItem := data[len(data)-1]

			// 更新共享变量
			if lastItem.AvgFee10 != 0 {
				lastRequestTime = currentTime
				sharedAvgFee10 = lastItem.AvgFee10
				return sharedAvgFee10, nil
			}
		}
		return 0, nil
	} else {
		if sharedAvgFee10 != 0 {
			return sharedAvgFee10, nil
		}
		return 0, nil
	}
}

func loadConfig() {
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		panic(p.Sprintf("Fatal error config file: %s", err))
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		panic(p.Sprintf("Unable to unmarshal config: %s", err))
	}
}
func checkAndPrintConfig() {
	//check privatekey and print address
	_, addr, err := config.GetPrivateKeyAddr()
	if err != nil {
		p.Println("Private key error:", err.Error())
		return
	}

	p.Println("你的钱包: ", config.GetWalletName())
	p.Println("你的地址 : ", addr)
}

func SendTx(ctx []byte) (string, error) {
	tx := wire.NewMsgTx(wire.TxVersion)
	tx.Deserialize(bytes.NewReader(ctx))

	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", err
	}

	hexStr := hex.EncodeToString(buf.Bytes())
	ctxHash, err := sendRawTransaction(hexStr)

	if err != nil {
		return "", err
	}

	return ctxHash, nil
}

func BuildMintTxs() {
	runeId, mintNum, err := config.GetMint()

	if err != nil {
		p.Println(err.Error())
		return

	}
	r := runestone.Runestone{Mint: runeId}
	runeData, err := r.Encipher()
	if err != nil {
		p.Println(err)
	}
	p.Printf("Mint Rune[%s] data: 0x%x\n", config.Mint.RuneId, runeData)
	//dataString, _ := txscript.DisasmString(data)
	//p.Printf("Mint Script: %s\n", dataString)

	init_gas_fee := config.GetFeePerByte()
	speed_gas_fee := config.GetSpeedFee()
	prvKey, address, _ := config.GetPrivateKeyAddr()

	IsAutoSpeed := config.GetIsAutoSpeed() //是否开启自动加速
	count := int64(0)                      //记录mint了多少张
	speedStatus := int64(0)                //记录mint了多少张

	for {
		if count >= mintNum {
			p.Println("MINT完成, 共: ", count, "张")
			break
		}

		gas_fee := int64(0)

		if init_gas_fee == 0 {
			gas_fee, err = fetchAvgFee()
			if err != nil {
				return
			}
		} else {
			gas_fee = init_gas_fee
		}

		utxos, err := getUtxos(address)

		if len(utxos) == 0 {
			p.Println("utxos: 没有可用余额:")
			break
		}

		if err != nil {
			p.Println("getUtxos error:", err.Error())
			return
		}

		for _, utxo := range utxos {
			var inputUtxos []*Utxo

			if utxo.Ancestorcount == 25 && IsAutoSpeed == 1 { //需要加速快速过快
				time.Sleep(3 * time.Second)
				p.Println("检测是否需要加速......")
				inputUtxos, err = returnReplaceTxUtxos(utxo.TxHash.String())
				if err != nil {
					p.Println("Error:", err)
					return
				}

				//获取当前区块gas
				var linshi_gas_fee = int64(0)
				if speed_gas_fee > 0 {
					linshi_gas_fee = speed_gas_fee
				} else {
					linshi_gas_fee, err = fetchAvgFee()
					if err != nil {
						return
					}
				}

				perfee := utxo.Ancestorfees / utxo.Ancestorsize
				if perfee >= linshi_gas_fee { //判断当前给的gas大于当前区块gas，则退出
					break
				}

				gas_fee = ((utxo.Ancestorsize*linshi_gas_fee - utxo.Ancestorfees) / (utxo.Ancestorsize / 25)) + perfee
				speedStatus = 1
				p.Println("当前平均每笔交易gas为: ", perfee, ";  为了加速到 ", linshi_gas_fee, ";  加速这笔交易给的gas: ", gas_fee)
			} else {
				inputUtxos = append(inputUtxos, utxo)
			}

			tx, err := BuildTransferBTCTx(prvKey, inputUtxos, address, config.GetUtxoAmount(), gas_fee, config.GetNetwork(), runeData, false)
			if err != nil {
				p.Println("BuildMintRuneTx error:", err.Error())
				break
			}

			txid, err := SendTx(tx)
			if err != nil {
				p.Println("广播失败: ", err.Error())
				break
			} else {
				if speedStatus == 0 {
					count++
				} else {
					speedStatus = 0
				}

				p.Println("第", count, "张， txhash是: ", txid, "  ,gas费是:", gas_fee)
			}
		}

		time.Sleep(0 * time.Second)
	}
}
