package main

import (
	"context"
	"errors"
	"flag"
	"path/filepath"

	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
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
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}
	// fallback to kubeconfig
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err == nil {
		return config, nil
	}
	// nothing works
	return nil, errors.New("unable to find config")
}

func newClient() (dynamic.Interface, error) {
	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return dynClient, nil
}

func main() {
	client, err := newClient()
	if err != nil {
		logger.L().Fatal("unable to create client", helpers.Error(err))
	}
	deploymentRes := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	ctx := context.Background()
	deployments := map[string]unstructured.Unstructured{}
	watcher, err := client.Resource(deploymentRes).Namespace("default").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		logger.L().Fatal("unable to watch for deployments", helpers.Error(err))
	}
	for {
		event, chanActive := <-watcher.ResultChan()
		if !chanActive {
			watcher.Stop()
			break
		}
		if event.Type == watch.Error {
			logger.L().Error("watch event failed", helpers.Interface("event", event))
			watcher.Stop()
			break
		}
		d, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		switch event.Type {
		case watch.Added:
			logger.L().Info("added deployment", helpers.String("name", d.GetName()))
			deployments[d.GetName()] = *d
		case watch.Modified:
			logger.L().Info("modified deployment", helpers.String("name", d.GetName()))
			if u, ok := deployments[d.GetName()]; ok {
				source, _ := u.MarshalJSON()
				target, _ := d.MarshalJSON()
				patch, err := jsondiff.CompareJSON(source, target)
				if err != nil {
					logger.L().Error("cannot create patch", helpers.Error(err))
				}
				logger.L().Info("patch", helpers.String("name", patch.String()))
			}
		case watch.Deleted:
			logger.L().Info("deleted deployment", helpers.String("name", d.GetName()))
			delete(deployments, d.GetName())
		}
	}
}
