package main

import (
	"encoding/json"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/domain"
	"github.com/matthyx/synchro-poc/utils"
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
				data, err := wsutil.ReadClientBinary(conn)
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
				logger.L().Debug("received message", helpers.Interface("event", msg.Event.Value()))
				switch *msg.Event {
				case domain.EventAdd:
					var add domain.Add
					err = json.Unmarshal(data, &add)
					if err != nil {
						logger.L().Error("cannot unmarshal add", helpers.Error(err))
						continue
					}
					logger.L().Info("adding object",
						helpers.String("kind", add.Kind.Resource),
						helpers.String("resource", add.Name),
						helpers.Int("size", len(add.Object)))
					if existingObj, ok := resources[add.Name]; ok {
						oldHash, _ := utils.CanonicalHash(existingObj)
						newHash, _ := utils.CanonicalHash([]byte(add.Object))
						logger.L().Info("object already exists",
							helpers.String("old checksum", oldHash),
							helpers.String("new checksum", newHash))
					}
					resources[add.Name] = []byte(add.Object)
				case domain.EventChecksum:
					var checksum domain.Checksum
					err = json.Unmarshal(data, &checksum)
					if err != nil {
						logger.L().Error("cannot unmarshal checksum", helpers.Error(err))
						continue
					}
					// check if checksum is correct
					localChecksum, _ := utils.CanonicalHash(resources[checksum.Name])
					if localChecksum == checksum.Checksum {
						logger.L().Info("checksum is correct",
							helpers.String("kind", checksum.Kind.Resource),
							helpers.String("resource", checksum.Name),
							helpers.String("checksum", checksum.Checksum))
						continue
					} else {
						logger.L().Warning("checksum is wrong",
							helpers.String("kind", checksum.Kind.Resource),
							helpers.String("resource", checksum.Name),
							helpers.String("local checksum", localChecksum),
							helpers.String("remote checksum", checksum.Checksum))
					}
					// wrong checksum, ask for retrieve
					event := domain.EventRetrieve
					resp := domain.Retrieve{
						Cluster: checksum.Cluster,
						Kind:    checksum.Kind,
						Name:    checksum.Name,
						Event:   &event,
					}
					respData, err := json.Marshal(resp)
					if err != nil {
						logger.L().Error("cannot marshal retrieve", helpers.Error(err))
						continue
					}
					err = wsutil.WriteServerBinary(conn, respData)
					if err != nil {
						logger.L().Error("cannot write retrieve", helpers.Error(err))
						continue
					}
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
					// apply patch
					modified, err := jsonpatch.MergePatch(resources[patch.Name], []byte(patch.Patch))
					if err != nil {
						logger.L().Error("cannot apply patch", helpers.Error(err))
						logger.L().Debug("send update shadow", helpers.String("key", patch.Name))
						event := domain.EventUpdateShadow
						resp := domain.UpdateShadow{
							Cluster: patch.Cluster,
							Kind:    patch.Kind,
							Name:    patch.Name,
							Event:   &event,
							Object:  string(resources[patch.Name]),
						}
						respData, err := json.Marshal(resp)
						if err != nil {
							logger.L().Error("cannot marshal updateShadow", helpers.Error(err))
							continue
						}
						err = wsutil.WriteServerBinary(conn, respData)
						if err != nil {
							logger.L().Error("cannot write updateShadow", helpers.Error(err))
							continue
						}
						continue
					}
					// update in known resources
					resources[patch.Name] = modified
				}
			}
		}()
	}))
}
