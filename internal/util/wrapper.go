package util

import (
	"os"
	"os/signal"
	"syscall"
)

func GoWrapper(fn func()) {
	go func() {
		fn()
	}()
}

func GoSafe(fn func(stop <-chan struct{})) {
	stop := make(chan struct{})

	go func() {
		fn(stop)
	}()

	// Ловим Ctrl+C / SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		close(stop) // сигнал для остановки всех функций
	}()
}
