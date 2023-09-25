package main

import (
	"encoding/json"
	"os"
	"os/signal"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/domain"
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
	// store resources in a map
	resources := map[string][]byte{}
	// subscribe to nats subject
	subscription, err := nc.Subscribe(cfg.Nats.Subject, func(m *nats.Msg) {
		var msg domain.Message
		err := json.Unmarshal(m.Data, &msg)
		if err != nil {
			logger.L().Error("cannot unmarshal message", helpers.Error(err))
		}
		logger.L().Info("received message", helpers.Interface("type", msg.Type), helpers.String("kind", msg.Kind.Resource), helpers.String("key", msg.Key))
		switch msg.Type {
		case domain.Added:
			resources[msg.Key] = msg.Object
		case domain.Modified:
			modified, err := jsonpatch.MergePatch(resources[msg.Key], msg.Patch)
			if err != nil {
				logger.L().Error("cannot merge patch", helpers.Error(err))
				return
			}
			resources[msg.Key] = modified
			// FIXME modify response and generate checksum to validate
		case domain.Deleted:
			delete(resources, msg.Key)
		}
		err = m.Respond([]byte("OK"))
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