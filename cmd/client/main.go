package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/nats-io/nats.go"
	"github.com/wI2L/jsondiff"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

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

func watchFor(ctx context.Context, res schema.GroupVersionResource) {
	resources := map[string]unstructured.Unstructured{}
	cfg := ctx.Value("cfg").(config.Config)
	watcher, err := ctx.Value("client").(dynamic.Interface).Resource(res).Namespace("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		logger.L().Fatal("unable to watch for resources", helpers.String("resource", res.Resource), helpers.Error(err))
	}
	for {
		event, chanActive := <-watcher.ResultChan()
		if !chanActive {
			watcher.Stop()
			break
		}
		if event.Type == watch.Error {
			logger.L().Error("watch event failed", helpers.String("resource", res.Resource), helpers.Interface("event", event))
			watcher.Stop()
			break
		}
		d, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		key := strings.Join([]string{d.GetNamespace(), d.GetName()}, "/")
		switch event.Type {
		case watch.Added:
			logger.L().Info("added resource", helpers.String("resource", res.Resource), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
			resources[key] = *d
		case watch.Modified:
			logger.L().Info("modified resource", helpers.String("resource", res.Resource), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
			if u, ok := resources[key]; ok {
				source, _ := u.MarshalJSON()
				target, _ := d.MarshalJSON()
				patch, err := jsondiff.CompareJSON(source, target)
				if err != nil {
					logger.L().Error("cannot create patch", helpers.String("resource", res.Resource), helpers.Error(err), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
				}
				msg, err := ctx.Value("nc").(*nats.Conn).Request(cfg.Nats.Subject, []byte(patch.String()), cfg.Nats.Timeout)
				switch {
				case err != nil:
					logger.L().Error("cannot send patch", helpers.String("resource", res.Resource), helpers.Error(err), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
				case string(msg.Data) != "OK":
					logger.L().Error("invalid response for patch", helpers.String("resource", res.Resource), helpers.String("msg", string(msg.Data)), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
				default:
					logger.L().Info("sent patch", helpers.String("resource", res.Resource), helpers.Int("size", len(patch.String())), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
				}
			}
		case watch.Deleted:
			logger.L().Info("deleted resource", helpers.String("resource", res.Resource), helpers.String("name", d.GetName()), helpers.String("namespace", d.GetNamespace()))
			delete(resources, key)
		}
	}
	ctx.Value("wg").(*sync.WaitGroup).Done()
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
	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "", Version: "v1", Resource: "pods"},
	}
	for _, res := range resources {
		wg.Add(1)
		go watchFor(ctx, res)
	}
	wg.Wait()
}
