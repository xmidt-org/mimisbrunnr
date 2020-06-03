package webhook

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/argus/webhookclient"
	"github.com/xmidt-org/webpa-common/webhook"
)

type Registry struct {
	hookStore *webhookclient.Client
	config    RegistryConfig
}

type RegistryConfig struct {
	Logger      log.Logger
	ArgusConfig webhookclient.ClientConfig
}

func NewRegistry(config RegistryConfig, listener webhookclient.Listener) (*Registry, error) {
	argus, err := webhookclient.CreateClient(config.ArgusConfig, webhookclient.WithLogger(config.Logger))
	if err != nil {
		return nil, err
	}
	if listener != nil {
		argus.SetListener(listener)
	}

	return &Registry{
		config:    config,
		hookStore: argus,
	}, nil
}

func jsonResponse(rw http.ResponseWriter, code int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)
	rw.Write([]byte(fmt.Sprintf(`{"message":"%s"}`, msg)))
}

// update is an api call to processes a listenener registration for adding and updating
func (r *Registry) UpdateRegistry(rw http.ResponseWriter, req *http.Request) {
	payload, err := ioutil.ReadAll(req.Body)
	req.Body.Close()

	w, err := webhook.NewW(payload, req.RemoteAddr)
	if err != nil {
		jsonResponse(rw, http.StatusBadRequest, err.Error())
		return
	}

	err = r.hookStore.Push(*w, "")
	if err != nil {
		jsonResponse(rw, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(rw, http.StatusOK, "Success")
}
