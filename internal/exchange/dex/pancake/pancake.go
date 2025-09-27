package pancake

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	bscRPCURL         = "https://bsc-dataseed.binance.org/"
	pancakeRouterAddr = "0x10ED43C718714eb63d5aA57B78B54704E256024E"

	routerABI = `[{"inputs":[{"internalType":"uint256","name":"amountIn","type":"uint256"},{"internalType":"address[]","name":"path","type":"address[]"}],"name":"getAmountsOut","outputs":[{"internalType":"uint256[]","name":"","type":"uint256[]"}],"stateMutability":"view","type":"function"}]`

	// WBNB and USDT on BSC mainnet
	WBNB = "0xBB4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
	USDT = "0x55d398326f99059fF775485246999027B3197955"
)

func Try() {
	// Слиппейдж: 1% = 0.01
	slippage := 0.01

	// Подключаемся к RPC
	client, err := ethclient.Dial(bscRPCURL)
	if err != nil {
		log.Fatalf("Ошибка подключения к BSC: %v", err)
	}

	// Сколько токенов хотим обменять — 1 BNB
	amountIn := big.NewInt(1e18) // 1 BNB = 1 * 10^18 wei

	// Задаём маршрут обмена: WBNB -> USDT
	path := []common.Address{
		common.HexToAddress(WBNB),
		common.HexToAddress(USDT),
	}

	// Подготавливаем вызов getAmountsOut
	parsedABI, err := abi.JSON(strings.NewReader(routerABI))
	if err != nil {
		log.Fatalf("Ошибка парсинга ABI: %v", err)
	}

	data, err := parsedABI.Pack("getAmountsOut", amountIn, path)
	if err != nil {
		log.Fatalf("Ошибка упаковки данных: %v", err)
	}

	// Выполняем вызов контракта
	toAddress := common.HexToAddress(pancakeRouterAddr)
	msg := ethereum.CallMsg{
		To:   &toAddress, // ← используем ссылку на переменную
		Data: data,
	}

	result, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Fatalf("Ошибка вызова контракта: %v", err)
	}

	// Распаковываем результат
	var amounts []*big.Int
	if err := parsedABI.UnpackIntoInterface(&amounts, "getAmountsOut", result); err != nil {
		log.Fatalf("Ошибка распаковки результата: %v", err)
	}

	amountOut := amounts[len(amounts)-1]
	fmt.Printf("🔁 За 1 BNB вы получите примерно: %s USDT (в wei)\n", amountOut.String())

	// Рассчитываем минимальный выход с учётом слиппейджа
	slippageFactor := big.NewFloat(1.0 - slippage)
	amountOutFloat := new(big.Float).SetInt(amountOut)
	minAmountOutFloat := new(big.Float).Mul(amountOutFloat, slippageFactor)

	minAmountOut := new(big.Int)
	minAmountOutFloat.Int(minAmountOut)

	fmt.Printf("✅ С учётом слиппейджа %.2f%% минимальный выход: %s USDT (в wei)\n", slippage*100, minAmountOut.String())
}
