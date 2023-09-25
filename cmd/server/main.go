package main

import (
	"os"
	"os/signal"

	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/nats-io/nats.go"
)

func main() {
	// config
	cfg, err := config.LoadConfig("./configuration")
	if err != nil {
		logger.L().Fatal("unable to load configuration", helpers.Error(err))
	}
	// nats client
	nc, err := nats.Connect(cfg.Nats.Urls)
	if err != nil {
		logger.L().Fatal("unable to create NATS client", helpers.Error(err), helpers.String("urls", cfg.Nats.Urls))
	}
	defer nc.Close()
	// subscribe to nats subject
	subscription, err := nc.Subscribe(cfg.Nats.Subject, func(m *nats.Msg) {
		logger.L().Info("received message", helpers.String("msg", string(m.Data)))
		err := m.Respond([]byte("OK"))
		if err != nil {
			logger.L().Error("unable to respond to message", helpers.Error(err))
		}
	})
	if err != nil {
		logger.L().Fatal("unable to subscribe to NATS", helpers.Error(err), helpers.String("subject", cfg.Nats.Subject))
	}
	defer func(subscribe *nats.Subscription) {
		_ = subscribe.Unsubscribe()
	}(subscription)
	err = nc.Flush()
	if err != nil {
		logger.L().Fatal("unable to flush NATS", helpers.Error(err))
	}
	// Setup the interrupt handler to drain so we don't miss
	// requests when scaling down.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	logger.L().Info("received interrupt, draining...")
	_ = nc.Drain()
	logger.L().Fatal("Exiting")
}
