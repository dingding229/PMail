package http_server

import (
	"errors"
	"flag"
	"fmt"
	"github.com/Jinnrry/pmail/config"
	"github.com/Jinnrry/pmail/controllers"
	"github.com/Jinnrry/pmail/utils/ip"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cast"
	"net/http"
	"os"
	"sync"
	"time"
)

// 项目初始化引导用的服务，初始化引导结束后即退出
var setupServer *http.Server

func SetupStart() {
	mux := http.NewServeMux()
	var setupRequests sync.Mutex
	serializeSetup := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Legacy setup replaces the global config and DB; bootstrap reads them.
			setupRequests.Lock()
			defer setupRequests.Unlock()
			handler(w, r)
		}
	}
	mux.HandleFunc("/", staticWeb())
	mux.HandleFunc("POST /api/setup", serializeSetup(contextIterceptor(controllers.Setup)))
	mux.HandleFunc("GET /api/bootstrap", serializeSetup(contextIterceptor(controllers.Bootstrap)))
	mux.HandleFunc("GET "+acmePrefix+"{token}", controllers.AcmeChallenge)

	HttpPort := config.Instance.HttpPort
	if HttpPort <= 0 {
		HttpPort = cast.ToInt(os.Getenv("setup_port"))
	}
	flag.IntVar(&HttpPort, "p", HttpPort, "初始化阶段Http服务端口")
	flag.Parse()

	if HttpPort <= 0 || HttpPort > 65535 {
		HttpPort = 80
	}

	config.Instance.SetSetupPort(HttpPort)
	log.Infof("HttpServer Start On Port :%d", HttpPort)
	if HttpPort == 80 {
		log.Infof("Please click http://%s to continue.\n", ip.GetIp())
	} else {
		log.Infof("Please click http://%s:%d to continue.", ip.GetIp(), HttpPort)
	}

	setupServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", HttpPort),
		Handler:      webMiddleware(mux),
		ReadTimeout:  time.Second * 60,
		WriteTimeout: time.Second * 60,
	}
	err := setupServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func SetupStop() {
	err := setupServer.Close()
	log.Infof("Setup End!")
	if err != nil {
		panic(err)
	}
}
