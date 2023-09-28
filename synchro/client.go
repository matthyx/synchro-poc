package synchro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/domain"
	"github.com/matthyx/synchro-poc/utils"
	"github.com/nats-io/nats.go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

type Client struct {
	cfg       config.Config
	client    dynamic.Interface
	nc        *nats.Conn
	res       schema.GroupVersionResource
	resources map[string][]byte
	strategy  domain.Strategy
}

func NewClient(cfg config.Config, client dynamic.Interface, nc *nats.Conn, r config.Resource) *Client {
	res := schema.GroupVersionResource{Group: r.Group, Version: r.Version, Resource: r.Resource}
	return &Client{
		cfg:       cfg,
		client:    client,
		nc:        nc,
		res:       res,
		resources: map[string][]byte{},
		strategy:  r.Strategy,
	}
}

func (c *Client) handleAdded(key string, newObject []byte) error {
	if c.strategy == domain.Patch {
		// add to known resources
		c.resources[key] = newObject
	}
	// request checksum
	checksum, _ := c.requestChecksum(key)
	// compare with local checksum
	expected, _ := utils.CanonicalHash(newObject)
	// eventually send added event
	if !bytes.Equal(checksum, expected[:]) {
		return c.sendAdded(key, newObject)
	}
	return nil
}

func (c *Client) handleDeleted(key string) error {
	if c.strategy == domain.Patch {
		// remove from known resources
		delete(c.resources, key)
	}
	// send deleted event
	return c.sendDeleted(key)
}

func (c *Client) handleModified(key string, newObject []byte) error {
	if c.strategy == domain.Patch {
		// update in known resources
		defer func() {
			c.resources[key] = newObject
		}()
	}
	// check if resource is known
	if originalObject, knownResource := c.resources[key]; knownResource {
		// request checksum
		checksum, _ := c.requestChecksum(key)
		// compare with local checksum
		expected, _ := utils.CanonicalHash(originalObject)
		if !bytes.Equal(checksum, expected[:]) {
			// cannot create a patch, send added event
			return c.sendAdded(key, newObject)
		}
		// create patch
		patch, err := jsonpatch.CreateMergePatch(originalObject, newObject)
		if err != nil {
			return c.sendAdded(key, newObject)
		}
		// send modified event
		resp, err := c.sendModified(key, patch)
		if err != nil {
			return c.sendAdded(key, newObject)
		}
		// compare checksum
		newExpected, _ := utils.CanonicalHash(newObject)
		if !bytes.Equal(resp.Data, newExpected[:]) {
			// checksum mismatch, send added event
			return c.sendAdded(key, newObject)
		}
	} else {
		// cannot create a patch, send added event
		return c.sendAdded(key, newObject)
	}
	return nil
}

func (c *Client) requestChecksum(key string) ([]byte, error) {
	msg := domain.Message{
		Cluster: c.cfg.Cluster,
		Kind:    c.res,
		Key:     key,
		Type:    domain.Checksum,
	}
	resp, err := c.sendMessage(msg)
	if err != nil {
		logger.L().Error("error during checksum request", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
		return nil, err
	}
	logger.L().Info("got checksum", helpers.String("checksum", fmt.Sprintf("%x", resp.Data)), helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return resp.Data, nil
}

func (c *Client) sendAdded(key string, newObject []byte) error {
	msg := domain.Message{
		Cluster: c.cfg.Cluster,
		Kind:    c.res,
		Key:     key,
		Type:    domain.Added,
		Object:  newObject,
	}
	_, err := c.sendMessage(msg)
	if err != nil {
		return err
	}
	logger.L().Warning("sent added message", helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return nil
}

func (c *Client) sendDeleted(key string) error {
	msg := domain.Message{
		Cluster: c.cfg.Cluster,
		Kind:    c.res,
		Key:     key,
		Type:    domain.Deleted,
	}
	_, err := c.sendMessage(msg)
	if err != nil {
		return err
	}
	logger.L().Info("sent deleted message", helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return nil
}

func (c *Client) sendModified(key string, patch []byte) (*nats.Msg, error) {
	msg := domain.Message{
		Cluster: c.cfg.Cluster,
		Kind:    c.res,
		Key:     key,
		Type:    domain.Modified,
		Patch:   patch,
	}
	resp, err := c.sendMessage(msg)
	if err != nil {
		return nil, err
	}
	logger.L().Info("sent modified message", helpers.String("resource", c.res.Resource), helpers.String("key", key))
	return resp, nil
}

// https://github.com/mantil-io/mantil/blob/89d864c4ca601dc912cf3086fa459761c28d850f/cli/log/net/publisher.go
func (c *Client) sendMessage(msg domain.Message) (*nats.Msg, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	chunks, last := utils.SplitIntoMsgs(data, c.cfg.Nats.Subject, int(c.nc.MaxPayload())-100)
	for _, chunk := range chunks {
		if _, err := c.nc.RequestMsg(chunk, c.cfg.Nats.Timeout); err != nil {
			return nil, err
		}
	}
	return c.nc.RequestMsg(last, c.cfg.Nats.Timeout)
}

func (c *Client) Run(wg *sync.WaitGroup) {
	list, err := c.client.Resource(c.res).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, d := range list.Items {
		key := strings.Join([]string{d.GetNamespace(), d.GetName()}, "/")
		newObject, err := d.MarshalJSON()
		if err != nil {
			logger.L().Error("cannot marshal object", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			continue
		}
		err = c.handleAdded(key, newObject)
		if err != nil {
			logger.L().Error("cannot handle added resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			continue
		}
	}
	watcher, err := c.client.Resource(c.res).Namespace("").Watch(context.Background(), metav1.ListOptions{ResourceVersion: list.GetResourceVersion()})
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
		key := strings.Join([]string{d.GetNamespace(), d.GetName()}, "/")
		newObject, err := d.MarshalJSON()
		if err != nil {
			logger.L().Error("cannot marshal object", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			continue
		}
		switch {
		case event.Type == watch.Added:
			logger.L().Info("added resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleAdded(key, newObject)
			if err != nil {
				logger.L().Error("cannot handle added resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		case event.Type == watch.Deleted:
			logger.L().Info("deleted resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleDeleted(key)
			if err != nil {
				logger.L().Error("cannot handle deleted resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		case event.Type == watch.Modified:
			logger.L().Info("modified resource", helpers.String("resource", c.res.Resource), helpers.String("key", key))
			err := c.handleModified(key, newObject)
			if err != nil {
				logger.L().Error("cannot handle modified resource", helpers.Error(err), helpers.String("resource", c.res.Resource), helpers.String("key", key))
			}
		}
	}
	wg.Done()
}
