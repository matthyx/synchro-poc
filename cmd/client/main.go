package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"sync"

	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/synchro"
	"github.com/nats-io/nats.go"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func getConfig() (*rest.Config, error) {
	// try in-cluster config first
	clusterConfig, err := rest.InClusterConfig()
	if err == nil {
		return clusterConfig, nil
	}
	// fallback to kubeconfig
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	clusterConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err == nil {
		return clusterConfig, nil
	}
	// nothing works
	return nil, errors.New("unable to find config")
}

func newClient() (dynamic.Interface, error) {
	clusterConfig, err := getConfig()
	if err != nil {
		return nil, err
	}
	dynClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		return nil, err
	}
	return dynClient, nil
}

func main() {
	ctx := context.Background()
	// config
	cfg, err := config.LoadConfig("./configuration")
	if err != nil {
		logger.L().Fatal("unable to load configuration", helpers.Error(err))
	}
	ctx = context.WithValue(ctx, "cfg", cfg)
	// k8s client
	client, err := newClient()
	if err != nil {
		logger.L().Fatal("unable to create k8s client", helpers.Error(err))
	}
	ctx = context.WithValue(ctx, "client", client)
	// nats client
	nc, err := nats.Connect(cfg.Nats.Urls)
	if err != nil {
		logger.L().Fatal("unable to create NATS client", helpers.Error(err), helpers.String("urls", cfg.Nats.Urls))
	}
	defer nc.Close()
	ctx = context.WithValue(ctx, "nc", nc)
	// wait group
	var wg sync.WaitGroup
	ctx = context.WithValue(ctx, "wg", &wg)
	for _, r := range cfg.Resources {
		syncClient := synchro.NewClient(cfg, client, nc, r)
		wg.Add(1)
		go syncClient.Run(&wg)
	}
	wg.Wait()
}
