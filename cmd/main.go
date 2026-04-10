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

const (
	timeToWaitForPreStop          = 10 * time.Second
	timeToWaitForGracefulShutdown = 5 * time.Second
)

func main() { //nolint:funlen
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
		logrus.Debug("Start watch signal activity")
		defer logrus.Debug("Stop watch signal activity")

		<-signalChanInterrupt
		logrus.Warn("Received interrupt signal")
		cancel()
		<-signalChanInterrupt
		os.Exit(1)
	}()

	application := internal.NewApplication()

	application.Args = os.Args[1:]

	err := application.Init()
	if err != nil {
		logrus.Fatal(err)
	}

	wait := func(reason string, d time.Duration) {
		logrus.Infof("Waiting during %s for %s...", reason, d.String())
		time.Sleep(d)
	}

	stopApplication := func() {
		logrus.Warn("Stoping application...")

		if ctx.Err() != nil {
			wait("graceful shutdown", timeToWaitForGracefulShutdown)

			return
		}

		wait("prestop", timeToWaitForPreStop)
		logrus.Info("Canceling context")
		cancel()
		wait("graceful shutdown", timeToWaitForGracefulShutdown)
	}
	defer stopApplication()

	logrus.RegisterExitHandler(stopApplication)

	err = application.Run(ctx)
	if err != nil {
		logrus.Fatal(err)
	}
}
