package synchro

import (
	"context"
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/domain"
	"github.com/matthyx/synchro-poc/utils"
	"github.com/panjf2000/ants/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type Client struct {
	cfg       config.Config
	client    dynamic.Interface
	outPool   *ants.PoolWithFunc
	res       schema.GroupVersionResource
	resources map[string][]byte
	strategy  domain.Strategy
}

func NewClient(cfg config.Config, client dynamic.Interface, outPool *ants.PoolWithFunc, r config.Resource) *Client {
	res := schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Resource}
	return &Client{
		cfg:       cfg,
		client:    client,
		outPool:   outPool,
		res:       res,
		resources: map[string][]byte{},
		strategy:  r.Strategy,
	}
}

func (c *Client) handleEtcdAdded(key string, newObject []byte) error {
	return c.handleEtcdModified(key, newObject)
}

func (c *Client) handleEtcdDeleted(key string) error {
	// send deleted event
	err := c.sendDelete(key)
	if err != nil {
		return fmt.Errorf("send delete message: %w", err)
	}
	if c.strategy == domain.PatchStrategy {
		// remove from known resources
		delete(c.resources, key)
	}
	return nil
}

func (c *Client) handleEtcdModified(key string, newObject []byte) error {
	// calculate checksum
	checksum, _ := utils.CanonicalHash(newObject)
	// send checksum
	return c.sendChecksum(key, checksum)
}

func (c *Client) HandleSyncAdd(key string, newObject []byte) error {
	if c.strategy == domain.PatchStrategy {
		// add to known resources
		c.resources[key] = newObject
	}
	// add to etcd
	_, err := c.client.Resource(c.res).Namespace("").Create(context.Background(), &unstructured.Unstructured{Object: map[string]interface{}{}}, metav1.CreateOptions{})
	return err
}

func (c *Client) HandleSyncDelete(key string) error {
	if c.strategy == domain.PatchStrategy {
		// remove from known resources
		delete(c.resources, key)
	}
	// remove from etcd
	return c.client.Resource(c.res).Namespace("").Delete(context.Background(), key, metav1.DeleteOptions{})
}

func (c *Client) HandleSyncRetrieve(key string) error {
	ns, name := utils.KeyToNsName(key)
	obj, err := c.client.Resource(c.res).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get resource: %w", err)
	}
	newObject, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal resource: %w", err)
	}
	if c.strategy == domain.PatchStrategy {
		if oldObject, ok := c.resources[key]; ok {
			// calculate patch
			patch, err := jsonpatch.CreateMergePatch(oldObject, newObject)
			if err != nil {
				return fmt.Errorf("create merge patch: %w", err)
			}
			err = c.sendPatch(key, patch)
			if err != nil {
				return fmt.Errorf("send patch message: %w", err)
			}
		} else {
			err = c.sendAdd(key, newObject)
			if err != nil {
				return fmt.Errorf("send add message: %w", err)
			}
		}
		// add to known resources
		c.resources[key] = newObject
	} else {
		err = c.sendAdd(key, newObject)
		if err != nil {
			return fmt.Errorf("send add message: %w", err)
		}
	}
	return nil
}

func (c *Client) HandleSyncUpdateShadow(key string, newObject []byte) error {
	if c.strategy == domain.PatchStrategy {
		// update in known resources
		c.resources[key] = newObject
		// send again
		return c.HandleSyncRetrieve(key)
	}
	return nil
}

func (c *Client) sendAdd(key string, newObject []byte) error {
	event := domain.EventAdd
	msg := domain.Add{
		Cluster: c.cfg.Cluster,
		Kind: &domain.Kind{
			Group:    c.res.Group,
			Version:  c.res.Version,
			Resource: c.res.Resource,
		},
		Name:   key,
		Event:  &event,
		Object: string(newObject),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal add message: %w", err)
	}
	err = c.outPool.Invoke(data)
	if err != nil {
		return fmt.Errorf("invoke outPool on add message: %w", err)
	}
	logger.L().Warning("sent added message",
		helpers.String("kind", msg.Kind.Resource),
		helpers.String("resource", msg.Name),
		helpers.Int("size", len(msg.Object)))
	return nil
}

func (c *Client) sendChecksum(key string, checksum string) error {
	event := domain.EventChecksum
	msg := domain.Checksum{
		Cluster: c.cfg.Cluster,
		Kind: &domain.Kind{
			Group:    c.res.Group,
			Version:  c.res.Version,
			Resource: c.res.Resource,
		},
		Name:     key,
		Event:    &event,
		Checksum: checksum,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal checksum message: %w", err)
	}
	err = c.outPool.Invoke(data)
	if err != nil {
		return fmt.Errorf("invoke outPool on checksum message: %w", err)
	}
	logger.L().Info("sent checksum message",
		helpers.String("resource", msg.Kind.Resource),
		helpers.String("key", msg.Name),
		helpers.String("checksum", checksum))
	return nil
}

func (c *Client) sendDelete(key string) error {
	event := domain.EventDelete
	msg := domain.Delete{
		Cluster: c.cfg.Cluster,
		Kind: &domain.Kind{
			Group:    c.res.Group,
			Version:  c.res.Version,
			Resource: c.res.Resource,
		},
		Name:  key,
		Event: &event,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal delete message: %w", err)
	}
	err = c.outPool.Invoke(data)
	if err != nil {
		return fmt.Errorf("invoke outPool on delete message: %w", err)
	}
	logger.L().Info("sent deleted message", helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return nil
}

func (c *Client) sendPatch(key string, patch []byte) error {
	event := domain.EventPatch
	msg := domain.Patch{
		Cluster: c.cfg.Cluster,
		Kind: &domain.Kind{
			Group:    c.res.Group,
			Version:  c.res.Version,
			Resource: c.res.Resource,
		},
		Name:  key,
		Event: &event,
		Patch: string(patch),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal patch message: %w", err)
	}
	err = c.outPool.Invoke(data)
	if err != nil {
		return fmt.Errorf("invoke outPool on patch message: %w", err)
	}
	logger.L().Info("sent patch message", helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return nil
}

func (c *Client) Run() {
	watchOpts := metav1.ListOptions{}
	// for our storage, we need to list all resources and get them one by one
	// as list returns objects with empty spec
	// and watch does not return existing objects
	if c.res.Group == "spdx.softwarecomposition.kubescape.io" {
		list, err := c.client.Resource(c.res).Namespace("").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return
		}
		for _, d := range list.Items {
			key := utils.NsNameToKey(d.GetNamespace(), d.GetName())
			obj, err := c.client.Resource(c.res).Namespace(d.GetNamespace()).Get(context.Background(), d.GetName(), metav1.GetOptions{})
			if err != nil {
				logger.L().Error("cannot get object", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
				continue
			}
			newObject, err := obj.MarshalJSON()
			if err != nil {
				logger.L().Error("cannot marshal object", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
				continue
			}
			err = c.handleEtcdAdded(key, newObject)
			if err != nil {
				logger.L().Error("cannot handle added resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
				continue
			}
		}
		// set resource version to watch from
		watchOpts.ResourceVersion = list.GetResourceVersion()
	}
	// begin watch
	watcher, err := c.client.Resource(c.res).Namespace("").Watch(context.Background(), watchOpts)
	if err != nil {
		logger.L().Fatal("unable to watch for resources", helpers.String("resource", c.res.Resource), helpers.Error(err))
	}
	for {
		event, chanActive := <-watcher.ResultChan()
		if !chanActive {
			watcher.Stop()
			break
		}
		if event.Type == watch.Error {
			logger.L().Error("watch event failed", helpers.String("resource", c.res.Resource), helpers.Interface("event", event))
			watcher.Stop()
			break
		}
		d, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		key := utils.NsNameToKey(d.GetNamespace(), d.GetName())
		newObject, err := d.MarshalJSON()
		if err != nil {
			logger.L().Error("cannot marshal object", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			continue
		}
		switch {
		case event.Type == watch.Added:
			logger.L().Info("added resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleEtcdAdded(key, newObject)
			if err != nil {
				logger.L().Error("cannot handle added resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		case event.Type == watch.Deleted:
			logger.L().Info("deleted resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleEtcdDeleted(key)
			if err != nil {
				logger.L().Error("cannot handle deleted resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		case event.Type == watch.Modified:
			logger.L().Info("modified resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleEtcdModified(key, newObject)
			if err != nil {
				logger.L().Error("cannot handle modified resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		}
	}
}
