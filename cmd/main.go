package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maksim-paskal/helm-watch/internal"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logrus.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		ForceColors:      true,
	})

	if os.Getenv("DEBUG") == "true" {
		logrus.SetLevel(logrus.DebugLevel)
	}

	signalChanInterrupt := make(chan os.Signal, 1)
	signal.Notify(signalChanInterrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalChanInterrupt
		logrus.Warn("Received interrupt signal")
		cancel()
		<-signalChanInterrupt
		os.Exit(1)
	}()

	application := internal.NewApplication()

	application.Args = os.Args[1:]

	if err := application.Init(); err != nil {
		logrus.Fatal(err)
	}

	logrus.RegisterExitHandler(func() {
		if ctx.Err() != nil {
			return
		}

		cancel()
		logrus.Warn("Waiting for graceful shutdown...")
		time.Sleep(10 * time.Second) //nolint:mnd
	})

	if err := application.Run(ctx); err != nil {
		logrus.Fatal(err)
	}
}
