package main

import (
	"context"
	"encoding/json"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/domain"
	"github.com/matthyx/synchro-poc/synchro"
	"github.com/matthyx/synchro-poc/utils"
	"github.com/panjf2000/ants/v2"
)

func main() {
	// config
	cfg, err := config.LoadConfig("./configuration")
	if err != nil {
		logger.L().Fatal("unable to load configuration", helpers.Error(err))
	}
	// k8s client
	client, err := utils.NewClient()
	if err != nil {
		logger.L().Fatal("unable to create k8s client", helpers.Error(err))
	}
	// websocket client
	conn, _, _, err := ws.DefaultDialer.Dial(context.Background(), "ws://127.0.0.1:8080/")
	if err != nil {
		logger.L().Fatal("unable to create websocket connection", helpers.Error(err))
	}
	defer conn.Close()
	// outgoing message pool
	outPool, err := ants.NewPoolWithFunc(10, func(i interface{}) {
		data := i.([]byte)
		err = wsutil.WriteClientBinary(conn, data)
		if err != nil {
			logger.L().Error("cannot send message", helpers.Error(err))
			return
		}
	})
	if err != nil {
		logger.L().Fatal("unable to create outgoing message pool", helpers.Error(err))
	}
	// etcd watches
	clients := map[string]*synchro.Client{}
	for _, r := range cfg.Resources {
		syncClient := synchro.NewClient(cfg, client, outPool, r)
		clients[r.String()] = syncClient
		go syncClient.Run()
	}
	// incoming messages
	for {
		data, err := wsutil.ReadServerBinary(conn)
		if err != nil {
			logger.L().Error("cannot read server data", helpers.Error(err))
			break
		}
		// unmarshal message
		var msg domain.Generic
		err = json.Unmarshal(data, &msg)
		if err != nil {
			logger.L().Error("cannot unmarshal message", helpers.Error(err))
			continue
		}
		switch *msg.Event {
		case domain.EventAdd:
			logger.L().Info("received add message", helpers.Interface("event", msg.Event.Value()))
			var add domain.Add
			err = json.Unmarshal(data, &add)
			if err != nil {
				logger.L().Error("cannot unmarshal add message", helpers.Error(err))
				continue
			}
			err := clients[msg.Kind.String()].HandleSyncAdd(add.Name, []byte(add.Object))
			if err != nil {
				logger.L().Error("error handling add message", helpers.Error(err))
				continue
			}
		case domain.EventDelete:
			logger.L().Info("received delete message", helpers.Interface("event", msg.Event.Value()))
			var del domain.Delete
			err = json.Unmarshal(data, &del)
			if err != nil {
				logger.L().Error("cannot unmarshal delete message", helpers.Error(err))
				continue
			}
			err := clients[msg.Kind.String()].HandleSyncDelete(del.Name)
			if err != nil {
				logger.L().Error("error handling delete message", helpers.Error(err))
				continue
			}
		case domain.EventRetrieve:
			logger.L().Info("received retrieve message", helpers.Interface("event", msg.Event.Value()))
			var ret domain.Retrieve
			err = json.Unmarshal(data, &ret)
			if err != nil {
				logger.L().Error("cannot unmarshal retrieve message", helpers.Error(err))
				continue
			}
			err := clients[msg.Kind.String()].HandleSyncRetrieve(ret.Name)
			if err != nil {
				logger.L().Error("error handling retrieve message", helpers.Error(err))
				continue
			}
		case domain.EventUpdateShadow:
			logger.L().Info("received update shadow message", helpers.Interface("event", msg.Event.Value()))
			var upd domain.UpdateShadow
			err = json.Unmarshal(data, &upd)
			if err != nil {
				logger.L().Error("cannot unmarshal update shadow message", helpers.Error(err))
				continue
			}
			err := clients[msg.Kind.String()].HandleSyncUpdateShadow(upd.Name, []byte(upd.Object))
			if err != nil {
				logger.L().Error("error handling update shadow message", helpers.Error(err))
				continue
			}
		}
	}
}
