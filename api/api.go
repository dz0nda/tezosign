package api

import (
	"context"
	"fmt"
	"net/http"
	"tezosign/common/log"
	"tezosign/conf"
	"tezosign/infrustructure"
	"tezosign/models"
	"tezosign/services/auth"
	"tezosign/services/rpc_client"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/rs/cors"
	"github.com/urfave/negroni"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	gracefulTimeout  = time.Second * 10
	actionsAPIPrefix = ""
)

type (
	API struct {
		router       *mux.Router
		server       *http.Server
		cfg          conf.Config
		provider     NetworkContextProvider
		queryDecoder *schema.Decoder
	}

	// Route stores an API route data
	Route struct {
		Path       string
		Method     string
		Func       func(http.ResponseWriter, *http.Request)
		Middleware []negroni.HandlerFunc
	}
)

type NetworkContextProvider interface {
	GetDb(models.Network) (*gorm.DB, error)
	GetIndexerDb(net models.Network) (*gorm.DB, error)
	GetRPCClient(net models.Network) (*rpc_client.Tezos, error)
	GetAuthProvider(net models.Network) (*auth.Auth, error)
	GetNetworkContext(net models.Network) (infrustructure.NetworkContext, error)
}

func NewAPI(cfg conf.Config, provider NetworkContextProvider) *API {
	queryDecoder := schema.NewDecoder()
	queryDecoder.IgnoreUnknownKeys(true)
	api := &API{
		cfg:          cfg,
		provider:     provider,
		queryDecoder: queryDecoder,
	}
	api.initialize()
	return api
}

// Run starts the http server and binds the handlers.
func (api *API) Run() error {
	return api.startServe()
}

func (api *API) Stop() error {
	ctx, _ := context.WithTimeout(context.Background(), gracefulTimeout)
	return api.server.Shutdown(ctx)
}

func (api *API) Title() string {
	return "API"
}

func (api *API) initialize(handlerArr ...negroni.Handler) {
	api.router = mux.NewRouter().UseEncodedPath()

	wrapper := negroni.New()

	for _, handler := range handlerArr {
		wrapper.Use(handler)
	}

	wrapper.Use(cors.New(cors.Options{
		AllowedOrigins:   api.cfg.API.CORSAllowedOrigins,
		AllowCredentials: true,
		AllowedMethods:   []string{"POST", "GET", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "User-Env"},
		Debug: false,
	}))

	//Static file
	w := wrapper.With(negroni.Wrap(http.StripPrefix("/static", http.FileServer(http.Dir("./resources")))))
	api.router.PathPrefix("/static").Handler(w).Methods(http.MethodGet, http.MethodOptions)

	//public routes
	HandleActions(api.router, wrapper, actionsAPIPrefix, []*Route{
		{Path: "/", Method: http.MethodGet, Func: api.Index},
		{Path: "/health", Method: http.MethodGet, Func: api.Health},
	})

	mw := []negroni.HandlerFunc{
		api.CheckAndLoadNetwork,
	}

	//public routes
	HandleActions(api.router, wrapper, actionsAPIPrefix, []*Route{
		//Auth flow
		{Path: "/{network}/auth/request", Method: http.MethodPost, Func: api.AuthRequest, Middleware: mw},
		{Path: "/{network}/auth", Method: http.MethodPost, Func: api.Auth, Middleware: mw},
		{Path: "/{network}/auth/refresh", Method: http.MethodPost, Func: api.RefreshAuth, Middleware: mw},
		{Path: "/{network}/auth/restore", Method: http.MethodGet, Func: api.RestoreAuth, Middleware: mw},
		{Path: "/{network}/logout", Method: http.MethodGet, Func: api.Logout, Middleware: mw},
		{Path: "/{network}/exchange_rates", Method: http.MethodGet, Func: api.TezosExchangeRates, Middleware: mw},

		{Path: "/{network}/{address}/revealed", Method: http.MethodGet, Func: api.AddressIsRevealed, Middleware: mw},
		{Path: "/{network}/origination/{tx_id}", Method: http.MethodGet, Func: api.ContractOrigination, Middleware: mw},
		{Path: "/{network}/{address}/balance", Method: http.MethodGet, Func: api.AddressBalance, Middleware: mw},
	})

	mw = []negroni.HandlerFunc{
		api.CheckAndLoadNetwork,
		api.RequireJWT,
	}

	HandleActions(api.router, wrapper, actionsAPIPrefix, []*Route{
		//Get contracts by pubkey
		{Path: "/{network}/contracts", Method: http.MethodGet, Func: api.AddressContracts, Middleware: mw},
		//Init contract storage
		{Path: "/{network}/contract/storage/init", Method: http.MethodPost, Func: api.ContractStorageInit, Middleware: mw},
		//Get contract info
		{Path: "/{network}/contract/{contract_id}/info", Method: http.MethodGet, Func: api.ContractInfo, Middleware: mw},
		//Create operation
		{Path: "/{network}/contract/operation", Method: http.MethodPost, Func: api.ContractOperation, Middleware: mw},

		//Build payload by operation
		{Path: "/{network}/contract/operation/{operation_id}/payload", Method: http.MethodGet, Func: api.OperationSignPayload, Middleware: mw},
		//Save payload signature
		{Path: "/{network}/contract/operation/{operation_id}/signature", Method: http.MethodPost, Func: api.ContractOperationSignature, Middleware: mw},
		//Build final tx
		{Path: "/{network}/contract/operation/{operation_id}/build", Method: http.MethodGet, Func: api.ContractOperationBuild, Middleware: mw},
		//Operation list
		{Path: "/{network}/contract/{contract_id}/operations", Method: http.MethodGet, Func: api.ContractOperationsList, Middleware: mw},

		//Get contract assets list
		{Path: "/{network}/contract/{contract_id}/assets", Method: http.MethodGet, Func: api.AssetsList, Middleware: mw},
		//Get contract assets list
		{Path: "/{network}/contract/{contract_id}/assets_rates", Method: http.MethodGet, Func: api.AssetsExchangeRates, Middleware: mw},
		//Get asset metadata
		{Path: "/{network}/asset/{asset_id}/meta_data", Method: http.MethodGet, Func: api.AssetMetadata, Middleware: mw},

		//Vesting contract
		{Path: "/{network}/contract/vesting/storage/init", Method: http.MethodPost, Func: api.VestingContractStorageInit, Middleware: mw},
		//Vesting contract info
		{Path: "/{network}/contract/vesting/{vesting_contract_id}/info", Method: http.MethodGet, Func: api.VestingContractInfo, Middleware: mw},
		//Direct vesting contract call
		{Path: "/{network}/contract/vesting/operation", Method: http.MethodPost, Func: api.VestingContractOperation, Middleware: mw},
		//Get contract vestings list
		{Path: "/{network}/contract/{contract_id}/vestings", Method: http.MethodGet, Func: api.VestingsList, Middleware: mw},
	})

	mw = []negroni.HandlerFunc{
		api.CheckAndLoadNetwork,
		api.RequireJWT,
		api.OwnerAllowance,
	}

	//Endpoints with contract owner restriction
	HandleActions(api.router, wrapper, actionsAPIPrefix, []*Route{
		//Create update storage operation
		{Path: "/{network}/contract/{contract_id}/storage/update", Method: http.MethodPost, Func: api.ContractStorageUpdate, Middleware: mw},

		//Assets
		//Create contract asset
		{Path: "/{network}/contract/{contract_id}/asset", Method: http.MethodPost, Func: api.ContractAsset, Middleware: mw},
		//Edit contract asset
		{Path: "/{network}/contract/{contract_id}/asset/edit", Method: http.MethodPost, Func: api.ContractAssetEdit, Middleware: mw},
		//Remove contract asset
		{Path: "/{network}/contract/{contract_id}/asset/delete", Method: http.MethodPost, Func: api.RemoveContractAsset, Middleware: mw},

		//Vesting
		//Add vesting contract
		{Path: "/{network}/contract/{contract_id}/vesting", Method: http.MethodPost, Func: api.ContractVesting, Middleware: mw},
		//Edit contract asset
		{Path: "/{network}/contract/{contract_id}/vesting/edit", Method: http.MethodPost, Func: api.ContractVestingEdit, Middleware: mw},
		//Remove contract asset
		{Path: "/{network}/contract/{contract_id}/vesting/delete", Method: http.MethodPost, Func: api.RemoveContractVesting, Middleware: mw},
	})

	api.server = &http.Server{Addr: fmt.Sprintf(":%d", api.cfg.API.ListenOnPort), Handler: api.router}
}

func (api *API) startServe() error {
	log.Info("Start listening server on port", zap.Uint64("port", api.cfg.API.ListenOnPort))
	err := api.server.ListenAndServe()
	if err == http.ErrServerClosed {
		log.Warn("API server was closed")
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot run API service: %s", err.Error())
	}
	return nil
}

// HandleActions is used to handle all given routes
func HandleActions(router *mux.Router, wrapper *negroni.Negroni, prefix string, routes []*Route) {
	for _, r := range routes {
		w := wrapper.With()
		for _, m := range r.Middleware {
			w.Use(m)
		}

		w.Use(negroni.Wrap(http.HandlerFunc(r.Func)))
		router.Handle(prefix+r.Path, w).Methods(r.Method, "OPTIONS")
	}
}
