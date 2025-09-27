package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/kwampek/spreadmaker2/internal/bot"
	"github.com/kwampek/spreadmaker2/internal/exchange/common/exchanges"
)

func main() {
	//Run()
	//TestNewExchange("ASCENDEX")
	//GetAllUnsupported()
	//TestAllChains(10)
	//TestAllCoins()

	Run()
	os.Exit(0) // Завершает весь процесс, т.к. горутины нельзя убить индивидуально
}

// func TBOT() {
// 	bot, err := tbot.NewTelegramBot(bot.BotToken, bot.DBPath)
// 	if err != nil {
// 		fmt.Println("Error: ", err)
// 	}

// 	bot.StartTelegramBot()
// }

func GetAllUnsupported() {
	file, err := os.Open("unchains.txt")
	if err != nil {
		fmt.Println("Ошибка при открытии файла:", err)
		return
	}
	defer file.Close()

	// Создание сканнера для построчного чтения
	scanner := bufio.NewScanner(file)
	all_chains := []string{}

	id := len("ASCENDEX                UnsupportedChain:")

outer:
	for scanner.Scan() {
		line := scanner.Text()
		for _, oldline := range all_chains {
			if line[id:] == oldline {
				continue outer
			}
		}
		all_chains = append(all_chains, line[id:])
		fmt.Println(line[id:])
	}

}

// func TestNewExchange(exchange string) {
// 	var bot = bot.NewBot()

// 	exchange_id := exchanges.RExchangesId[exchange]

// 	fmt.Println(len(bot.OrderBooks))

// 	for _, coin := range bot.Exchanges[exchange_id]. {
// 		if !bot.ChainsTable.AllSymbols[coin].Test(exchange_id) {
// 			fmt.Println("Currency is not supported for ", exchange)
// 			continue
// 		}

// 		info, err := b.RExchanges[exchange_id].FetchOrderBook(coin, "10")
// 		if err != nil {
// 			fmt.Println("Error occured while fetching ", exchange, " orderbook: ", err)
// 		} else {
// 			fmt.Println("OrderBook (", coin, ") : ", info)
// 		}
// 	}
// }

func TestNewExchangeChains(exchange string) {
	var bot = bot.NewBot()

	fmt.Println("\n\n\nTESTCHAINS:")
	id := exchanges.RExchangesId[exchange]

	fmt.Println("EXCHANGFE ID: ", id)

	for symbol, all_exch := range bot.Chains {
		for _, ch := range *all_exch[id].Load() {
			fmt.Println(symbol, ch)
		}
	}
	fmt.Println()
}

func TestAllCoins() {
	all_coins := make(map[string]int)
	var bot = bot.NewBot()
	bot.Sync.StartRoutine()
	for _, ex := range bot.Exchanges {
		ex.FetchAllCoins(all_coins)
	}

	fmt.Println(all_coins)
}

func TestAllChains(count int) {
	var bot = bot.NewBot()
	bot.Sync.StartRoutine()
	bot.CollectChainsInfo()

	cnt := 1
	for symbol, all_exch := range bot.Chains {
		fmt.Println("\nSYMBOL: ", symbol, len(all_exch))
		for id, link := range all_exch {
			fmt.Print(exchanges.RExchanges[id], " : ")
			tmp := link.Load()
			if tmp != nil {
				fmt.Println(*link.Load())
			} else {
				fmt.Println()
			}
		}
		if cnt == count {
			break
		}
		cnt++
	}
}

func Run() {
	var bot = bot.NewBot()
	bot.Start()
}

/*
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"github.com/kwampek/spreadmaker2/internal/bot"
)

const openAIEndpoint = "https://api.openai.com/v1/embeddings"
const openAIModel = "text-embedding-3-large" // или "text-embedding-ada-002"

type OpenAIRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type OpenAIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func getEmbedding(apiKey, input string) ([]float64, error) {
	body, _ := json.Marshal(OpenAIRequest{
		Input: input,
		Model: openAIModel,
	})

	req, _ := http.NewRequest("POST", openAIEndpoint, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	proxyUrl, err := url.Parse("http://127.0.0.1:12334")
	if err != nil {
		log.Fatal("Cant parse proxy parameters: ", err)
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			panic("TODO")
		},
		Jar: nil,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API error: %s\nBody: %s", resp.Status, string(respBody))
	}

	var response OpenAIResponse
	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return nil, err
	}
	fmt.Println(response)

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("empty response from OpenAI")
	}

	return response.Data[0].Embedding, nil
}

func cosineSimilarity(a, b []float64) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	z := x
	for range 20 {
		z -= (z*z - x) / (2 * z)
	}
	return z
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Установи переменную окружения OPENAI_API_KEY")
		return
	}

	networkNames := []string{
		"Ethereum", "ETH", "ethereum-mainnet",
		"Binance Smart Chain", "BSC", "bsc-mainnet",
		"Polygon", "MATIC", "polygon-mainnet",
		"Arbitrum", "ARB", "arbitrum-one",
		"ZkSync", "zkSync Era", "zksync-mainnet",
		"Base", "Base Network", "base-mainnet",
	}

	embeddings := make([][]float64, len(networkNames))
	for i, name := range networkNames {
		embed, err := getEmbedding(apiKey, name)
		if err != nil {
			fmt.Printf("Ошибка при получении embedding для %s: %v\n", name, err)
			return
		}
		embeddings[i] = embed
	}

	// Простая кластеризация: объединение по cosine similarity > threshold
	clusters := make([]int, len(networkNames))
	clusterID := 0
	threshold := 0.92

	for i := range networkNames {
		if clusters[i] != 0 {
			continue
		}
		clusterID++
		clusters[i] = clusterID
		for j := i + 1; j < len(networkNames); j++ {
			if cosineSimilarity(embeddings[i], embeddings[j]) > threshold {
				clusters[j] = clusterID
			}
		}
	}

	// Вывод
	fmt.Println("Сопоставление сетей в ID:")
	namesWithIDs := make([]string, len(networkNames))
	for i := range networkNames {
		namesWithIDs[i] = fmt.Sprintf("%-25s => %d", networkNames[i], clusters[i])
	}
	sort.Strings(namesWithIDs)
	for _, line := range namesWithIDs {
		fmt.Println(line)
	}
}
*/
