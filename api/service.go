package api

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/mcuadros/go-monitor.v1/aspects"

	"github.com/DeanThompson/ginpprof"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/szuecs/binary-patch/conf"
	"github.com/zalando/gin-glog"
	"github.com/zalando/gin-gomonitor"
	"github.com/zalando/gin-gomonitor/aspects"
	"github.com/zalando/gin-oauth2"
	"github.com/zalando/gin-oauth2/zalando"
	"golang.org/x/oauth2"
)

// ServiceConfig contains everything configurable for the service
// endpoint.
type ServiceConfig struct {
	Config          *conf.Config
	OAuth2Endpoints oauth2.Endpoint
	CertKeyPair     tls.Certificate
	Httponly        bool
}

var cfg *conf.Config

// Service is the main struct
type Service struct {
	Healthy bool
	sig     chan os.Signal
}

func NewService() *Service {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)

	return &Service{
		Healthy: false,
		sig:     sigs,
	}

}

func (svc *Service) RegisterShutdown() {
	go func() {
		<-svc.sig
		glog.Info("Shutdown..")
		os.Exit(0)
	}()
}

func (svc *Service) checkDependencies() bool {
	// TODO: you may want to check if you can connect to your dependencies here
	return true
}

// Run is the main function of the server. It bootstraps the service
// and creates the route endpoints.
func (svc *Service) Run(config *ServiceConfig) error {
	cfg = config.Config

	// init gin
	if !cfg.DebugEnabled {
		gin.SetMode(gin.ReleaseMode)
	}

	// initialize CounterAspect and reset every minute
	counterAspect := ginmon.NewCounterAspect()
	counterAspect.StartTimer(1 * time.Minute)
	asps := []aspects.Aspect{counterAspect}
	gomonitor.Start(cfg.MonitorPort, asps)

	// Middleware
	router := gin.New()
	router.Use(ginglog.Logger(cfg.LogFlushInterval))
	router.Use(ginmon.CounterHandler(counterAspect))
	router.Use(gin.Recovery())

	// OAuth2 secured if conf.Oauth2Enabled is set
	var private *gin.RouterGroup
	if cfg.Oauth2Enabled {
		private = router.Group("")

		if cfg.AuthorizedTeams != nil {
			glog.Infof("OAuth2 team authorization, grant to: %+v", cfg.AuthorizedTeams)
			private.Use(ginoauth2.Auth(zalando.GroupCheck(cfg.AuthorizedTeams), config.OAuth2Endpoints))
		}
		if cfg.AuthorizedUsers != nil {
			glog.Infof("OAuth2 user authorization, grant to: %+v", cfg.AuthorizedUsers)
			private.Use(ginoauth2.Auth(zalando.UidCheck(cfg.AuthorizedUsers), config.OAuth2Endpoints))
		}
	}

	//
	//  Handlers
	//
	router.GET("/healthz", svc.HealthHandler)
	if cfg.Oauth2Enabled {
		// authenticated and authorized routes
		private.GET("/", svc.RootHandler)
		private.GET("/update/:name", svc.UpdateHandler)
		private.GET("/patch-update/:name", svc.PatchUpdateHandler)
		private.GET("/signed-update/:name", svc.SignedUpdateHandler)
		private.GET("/signed-patch-update/:name", svc.SignedPatchUpdateHandler)
		private.PUT("/upload/:name", svc.UploadHandler)
	} else {
		// public routes
		router.GET("/", svc.RootHandler)
		router.GET("/update/:name", svc.UpdateHandler)
		router.GET("/patch-update/:name", svc.PatchUpdateHandler)
		router.GET("/signed-update/:name", svc.SignedUpdateHandler)
		router.GET("/signed-patch-update/:name", svc.SignedPatchUpdateHandler)
		router.PUT("/upload/:name", svc.UploadHandler)
	}

	// TLS config
	tlsConfig := tls.Config{}
	if !config.Httponly {
		tlsConfig.Certificates = []tls.Certificate{config.CertKeyPair}
		tlsConfig.Rand = rand.Reader // Strictly not necessary, should be default
	}

	// run api server
	serve := &http.Server{
		Addr:      fmt.Sprintf(":%d", cfg.Port),
		Handler:   router,
		TLSConfig: &tlsConfig,
	}

	if cfg.ProfilingEnabled {
		ginpprof.Wrapper(router)
	}

	if svc.checkDependencies() {
		svc.Healthy = true
	}
	svc.RegisterShutdown()

	// start server
	if config.Httponly {
		err := serve.ListenAndServe()
		if err != nil {
			glog.Exitf("Can not Serve HTTP, caused by: %s", err)
		}
	} else {
		conn, err := net.Listen("tcp", serve.Addr)
		if err != nil {
			glog.Exitf("Can not listen on %s, because some other process is already using it", serve.Addr)
		}
		tlsListener := tls.NewListener(conn, &tlsConfig)
		err = serve.Serve(tlsListener)
		if err != nil {
			glog.Exitf("Can not Serve TLS, caused by: %s", err)
		}
	}
	return nil
}
