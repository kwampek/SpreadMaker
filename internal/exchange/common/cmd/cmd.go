package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	StatusError   = 2
	StatusOk      = 1
	StatusPaused  = 666
	InitLoadLevel = 0
)

type BotSync struct {
	LoadLevel atomic.Int32
	Status    atomic.Int32
	History   []string
}

func (bs *BotSync) StartRoutine() {
	bs.Status.Store(StatusOk)
	bs.LoadLevel.Store(InitLoadLevel)

	go bs.CmdRoutine()
}

func (bs *BotSync) CmdRoutine() {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading command:", err)
			continue
		}
		bs.handleCommand(strings.TrimSpace(text))
	}
}

// --- Command Handling ---

func (bs *BotSync) handleCommand(cmd string) {
	tokens := strings.Fields(cmd)
	if len(tokens) == 0 {
		return
	}

	switch tokens[0] {
	case "start":
		bs.handleStart()
	case "stop":
		bs.handleStop()
	case "remove":
		bs.handleRemove(tokens)
	case "add_chain":
		bs.handleAddChain(tokens)
	case "coin":
		bs.handleCoin(tokens)
	case "proxy":
		bs.handleProxy()
	case "fetch_info":
		bs.handleFetchInfo(tokens)
	case "update_info":
		bs.handleUpdateInfo(tokens)
	default:
		fmt.Println("Unknown command:", tokens[0])
	}
}

// --- Individual Command Handlers ---

func (bs *BotSync) handleStart() {
	bs.Status.Store(1)
	fmt.Println("Bot started.")
}

func (bs *BotSync) handleStop() {
	fmt.Println("Stopping bot... waiting for LoadLevel to reach 0")
	bs.Status.Store(0)
	for bs.LoadLevel.Load() > 0 {
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Bot stopped.")
}

func (bs *BotSync) handleRemove(tokens []string) {
	if len(tokens) < 2 {
		fmt.Println("Usage: remove [name]")
		return
	}
	fmt.Printf("Removing %s (placeholder)\n", tokens[1])
}

func (bs *BotSync) handleAddChain(tokens []string) {
	if len(tokens) < 2 {
		fmt.Println("Usage: add_chain [name]")
		return
	}
	fmt.Printf("Adding chain %s (placeholder)\n", tokens[1])
}

func (bs *BotSync) handleCoin(tokens []string) {
	if len(tokens) < 2 {
		fmt.Println("Usage: coin [name]")
		return
	}
	fmt.Printf("Handling coin %s (placeholder)\n", tokens[1])
}

func (bs *BotSync) handleProxy() {
	fmt.Println("Handling proxy (placeholder)")
}

func (bs *BotSync) handleFetchInfo(tokens []string) {
	if len(tokens) < 2 {
		fmt.Println("Usage: fetch_info [file_name]")
		return
	}
	fmt.Printf("Fetching info from %s (placeholder)\n", tokens[1])
}

func (bs *BotSync) handleUpdateInfo(tokens []string) {
	if len(tokens) < 2 {
		fmt.Println("Usage: update_info [file_name]")
		return
	}
	fmt.Printf("Updating info from %s (placeholder)\n", tokens[1])
}
