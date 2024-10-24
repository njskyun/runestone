package main

import (
	"bytes"
	"regexp"

	// "crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/bxelab/runestone"
)

type Config struct {
	WalletName  string
	PrivateKey  string
	FeePerByte  int64
	UtxoAmount  int64
	IsAutoSpeed int64
	SpeedFee    int64
	Network     string
	RpcUrl      string
	LocalRpcUrl string
	Etching     *struct {
		Rune              string
		Logo              string
		Symbol            *string
		Premine           *uint64
		Amount            *uint64
		Cap               *uint64
		Divisibility      *int
		HeightStart       *int
		HeightEnd         *int
		HeightOffsetStart *int
		HeightOffsetEnd   *int
	}
	Mint *struct {
		RuneId  string
		MintNum int64
	}
}

var wallet_name = "walletname_8888"

// importPrivateKey 导入私钥到指定钱包
func (c Config) importPrivateKey(cksum string) error {
	_, addr, _ := c.GetPrivateKeyAddr()

	isExists, err := c.CheckAddressInWallet(addr)
	if err != nil {
		return err
	}

	if isExists {
		return nil
	}

	addesc := "addr(" + addr + ")"

	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "mm",
		"method":  "importdescriptors",
		"params": [][]map[string]interface{}{
			{
				{
					"desc":      fmt.Sprintf("%s#%s", addesc, cksum), // 确保描述符的正确性
					"timestamp": "now",
					"active":    false,
					"index":     0,
					"internal":  false,
					"label":     "mm",
					"watchonly": true, // 设置为仅观察地址
				},
			},
		},
	})
	if err != nil {
		return err
	}

	localrpc := config.GetLocalRpcUrl() + "/wallet/" + wallet_name

	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return fmt.Errorf("RPC error: %v", errMsg)
	}

	// fmt.Println(result)
	// 提取校验和
	checksum := handleResult(result)
	if checksum != "" {
		fmt.Println("地址导入中")
		err := c.importPrivateKey(checksum)
		if err != nil {
			fmt.Println("地址导入失败")
			return err
		}
	}

	return nil
}

// 提取并比较校验和
func extractAndCompareChecksum(errorMessage string) string {
	re := regexp.MustCompile(`Provided checksum '([a-z0-9]+)' does not match computed checksum '([a-z0-9]+)'`)
	matches := re.FindStringSubmatch(errorMessage)

	if len(matches) == 3 {
		return matches[2]
	}

	return ""
}

func handleResult(result map[string]interface{}) string {
	if resultList, ok := result["result"].([]interface{}); ok && len(resultList) > 0 {
		if errorDetails, ok := resultList[0].(map[string]interface{})["error"].(map[string]interface{}); ok {
			// 提取 message 字段
			message, ok := errorDetails["message"].(string)
			if !ok {
				return ""
			}

			// 提取校验和
			return extractAndCompareChecksum(message)
		}
	}

	return ""
}

func (c Config) walletExists(walletName string) (bool, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "mm",
		"method":  "listwallets",
		"params":  []interface{}{},
	})
	if err != nil {
		return false, err
	}

	localrpc := c.GetLocalRpcUrl()

	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	// 检查是否有错误信息
	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return false, fmt.Errorf("RPC error: %v", errMsg)
	}

	// 获取钱包列表
	wallets, ok := result["result"].([]interface{})
	if !ok {
		return false, fmt.Errorf("unexpected response format")
	}

	// 检查钱包名称是否在列表中
	for _, w := range wallets {
		if name, ok := w.(string); ok && name == walletName {
			return true, nil
		}
	}

	return false, nil
}

func (c Config) CheckAddressInWallet(address string) (bool, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "mm",
		"method":  "getaddressinfo",
		"params":  []interface{}{address},
	})
	if err != nil {
		return false, err
	}

	localrpc := c.GetLocalRpcUrl() + "/wallet/" + wallet_name

	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	// 检查是否有错误信息
	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return false, fmt.Errorf("RPC error: %v", errMsg)
	}

	// fmt.Println("getaddressinfo:  ", result)
	// 检查地址是否有效且在钱包中
	if resultData, ok := result["result"].(map[string]interface{}); ok {
		// 直接使用 ismine 字段
		return resultData["ismine"].(bool), nil
	}

	return false, nil
}

// 新建个观察钱包
func (c Config) createWallet() (string, error) {
	//检测钱包是否存在，没有则新建一个临时使用
	isExists, err := c.walletExists(wallet_name)
	if err != nil {
		return "", err
	}

	if isExists {
		err := c.importPrivateKey("aaaaaaaa")
		if err != nil {
			return "", err
		}

		return wallet_name, nil
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "mm",
		"method":  "createwallet",
		"params":  []interface{}{wallet_name, true, true, nil, true, true},
	})
	if err != nil {
		return "", err
	}

	localrpc := c.GetLocalRpcUrl()

	resp, err := http.Post(localrpc, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		return "", fmt.Errorf("RPC error: %v", errMsg)
	}

	err = c.importPrivateKey("aaaaaaaa")
	if err != nil {
		return "", err
	}

	return wallet_name, nil
}

func DefaultConfig() Config {
	return Config{
		Network: "mainnet",
	}

}
func (c Config) GetFeePerByte() int64 {
	if c.FeePerByte == 0 {
		return 0
	}
	return c.FeePerByte
}

func (c Config) GetLocalRpcUrl() string {
	return c.LocalRpcUrl
}

func (c Config) GetIsAutoSpeed() int64 {
	return c.IsAutoSpeed
}

func (c Config) GetUtxoAmount() int64 {
	if c.UtxoAmount == 0 {
		return 330
	}
	return c.UtxoAmount
}

func (c Config) GetSpeedFee() int64 {
	return c.SpeedFee
}

func (c Config) GetWalletName() string {
	//先新建临时钱包，将提供的私钥导入到钱包中
	var err error
	walletName, err = c.createWallet()
	if err != nil {
		fmt.Println("创建钱包失败: ", err)
		return ""
	}

	return walletName
}

func (c Config) GetMint() (*runestone.RuneId, int64, error) {
	if c.Mint == nil {
		return nil, 0, errors.New("Mint config is required")
	}
	if c.Mint.RuneId == "" {
		return nil, 0, errors.New("RuneId is required")
	}
	if c.Mint.MintNum == 0 {
		return nil, 0, errors.New("MintNum is required")
	}

	runeId, err := runestone.RuneIdFromString(c.Mint.RuneId)
	if err != nil {
		return nil, 0, err
	}
	return runeId, c.Mint.MintNum, nil
}

func (c Config) GetNetwork() *chaincfg.Params {
	if c.Network == "mainnet" {
		return &chaincfg.MainNetParams
	}
	if c.Network == "testnet" {
		return &chaincfg.TestNet3Params
	}
	if c.Network == "regtest" {
		return &chaincfg.RegressionNetParams
	}
	if c.Network == "signet" {
		return &chaincfg.SigNetParams
	}
	panic("unknown network")
}

func (c Config) GetPrivateKeyAddr() (*btcec.PrivateKey, string, error) {
	if c.PrivateKey == "" {
		return nil, "", errors.New("PrivateKey is required")
	}
	pkBytes, err := hex.DecodeString(c.PrivateKey)
	if err != nil {
		return nil, "", err
	}
	privKey, pubKey := btcec.PrivKeyFromBytes(pkBytes)
	if err != nil {
		return nil, "", err
	}
	tapKey := txscript.ComputeTaprootKeyNoScript(pubKey)
	addr, err := btcutil.NewAddressTaproot(
		schnorr.SerializePubKey(tapKey), c.GetNetwork(),
	)
	if err != nil {
		return nil, "", err
	}
	address := addr.EncodeAddress()

	return privKey, address, nil
}
func (c Config) GetRuneLogo() (mime string, data []byte) {
	if c.Etching != nil && c.Etching.Logo != "" {
		mime, err := getContentType(c.Etching.Logo)
		if err != nil {
			return "", nil
		}
		data, err := getFileBytes(c.Etching.Logo)
		if err != nil {
			return "", nil
		}
		return mime, data

	}
	return "", nil
}

func getContentType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil {
		return "", err
	}

	contentType := http.DetectContentType(buffer)
	return contentType, nil
}
func getFileBytes(filePath string) ([]byte, error) {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return fileBytes, nil
}
