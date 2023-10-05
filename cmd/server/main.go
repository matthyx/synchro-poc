package main

import (
	"encoding/json"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/domain"
)

func main() {
	resources := map[string][]byte{}
	// websocket server
	http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			logger.L().Error("unable to upgrade connection", helpers.Error(err))
			return
		}

		go func() {
			defer conn.Close()

			for {
				var hash [32]byte
				data, op, err := wsutil.ReadClientData(conn)
				if err != nil {
					logger.L().Error("cannot read client data", helpers.Error(err))
					break
				}
				// unmarshal message
				var msg domain.Generic
				err = json.Unmarshal(data, &msg)
				if err != nil {
					logger.L().Error("cannot unmarshal message", helpers.Error(err))
					continue
				}
				logger.L().Info("received message", helpers.Interface("event", msg.Event.Value()))
				switch *msg.Event {
				case domain.EventAdd:
					var add domain.Add
					err = json.Unmarshal(data, &add)
					if err != nil {
						logger.L().Error("cannot unmarshal add", helpers.Error(err))
						continue
					}
					resources[add.Name] = []byte(add.Object)
				case domain.EventChecksum:
					var checksum domain.Checksum
					err = json.Unmarshal(data, &checksum)
					if err != nil {
						logger.L().Error("cannot unmarshal checksum", helpers.Error(err))
						continue
					}
					// TODO
				case domain.EventDelete:
					var del domain.Delete
					err = json.Unmarshal(data, &del)
					if err != nil {
						logger.L().Error("cannot unmarshal delete", helpers.Error(err))
						continue
					}
					delete(resources, del.Name)
				case domain.EventPatch:
					var patch domain.Patch
					err = json.Unmarshal(data, &patch)
					if err != nil {
						logger.L().Error("cannot unmarshal patch", helpers.Error(err))
						continue
					}
					// TODO
				}
				err = wsutil.WriteServerMessage(conn, op, hash[:])
				if err != nil {
					logger.L().Error("cannot write server message", helpers.Error(err))
					break
				}
			}
		}()
	}))
}
