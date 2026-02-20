package logging

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
)

func Init() {
	Info = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	Debug = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
}

func SetupGracefulShutdown(onShutdown func()) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		Info.Printf("Received signal %v, shutting down gracefully...", sig)
		onShutdown()
	}()
}
