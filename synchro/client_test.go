package synchro

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/kubescape/go-logger"
	"github.com/kubescape/go-logger/helpers"
	"github.com/matthyx/synchro-poc/config"
	"github.com/matthyx/synchro-poc/domain"
	"github.com/matthyx/synchro-poc/utils"
	"github.com/panjf2000/ants/v2"
	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	// config
	cfg, err := config.LoadConfig("../configuration")
	assert.NoError(t, err)
	// k8s client
	client, err := utils.NewClient()
	assert.NoError(t, err)
	// websocket client
	conn, _, _, err := ws.DefaultDialer.Dial(context.Background(), "ws://127.0.0.1:8080/")
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	syncClient := NewClient(cfg, client, outPool, cfg.Resources[1])
	toto := []byte("{}")
	err = syncClient.sendAdd("toto", toto)
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
	hash, err := utils.CanonicalHash(toto)
	assert.NoError(t, err)
	fmt.Println("hash", hash)
	err = syncClient.sendChecksum("toto", hash)
	assert.NoError(t, err)
	time.Sleep(1 * time.Second)
}

func TestSerializeChecksum(t *testing.T) {
	toto := []byte("{}")
	resources := map[string][]byte{
		"toto": toto,
	}
	hash, err := utils.CanonicalHash(toto)
	assert.NoError(t, err)
	event := domain.EventChecksum
	checksum := domain.Checksum{
		Event:    &event,
		Cluster:  "kind-kind",
		Name:     "toto",
		Checksum: hash,
	}
	localChecksum, _ := utils.CanonicalHash(resources[checksum.Name])
	if localChecksum == checksum.Checksum {
		fmt.Println("checksum ok")
	}
}
