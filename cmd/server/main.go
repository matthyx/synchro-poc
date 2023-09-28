package main

import (
	"encoding/json"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch/v5"
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
				var hash [32]byte
				data, op, err := wsutil.ReadClientData(conn)
				if err != nil {
					logger.L().Error("cannot read client data", helpers.Error(err))
					break
				}
				// unmarshal message
				var msg domain.Message
				err = json.Unmarshal(data, &msg)
				if err != nil {
					logger.L().Error("cannot unmarshal message", helpers.Error(err))
					continue
				}
				logger.L().Info("received message", helpers.Interface("type", msg.Type), helpers.String("kind", msg.Kind.Resource), helpers.String("key", msg.Key))
				switch msg.Type {
				case domain.Added:
					resources[msg.Key] = msg.Object
				case domain.Modified:
					modified, err := jsonpatch.MergePatch(resources[msg.Key], msg.Patch)
					if err != nil {
						logger.L().Error("cannot merge patch", helpers.Error(err))
						continue
					}
					resources[msg.Key] = modified
					// send checksum for validation
					hash, _ = utils.CanonicalHash(modified)
				case domain.Deleted:
					delete(resources, msg.Key)
				case domain.Checksum:
					hash, _ = utils.CanonicalHash(resources[msg.Key])
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
