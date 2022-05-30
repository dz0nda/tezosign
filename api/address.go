package api

import (
	"net/http"
	"tezosign/api/response"
	"tezosign/common/apperrors"
	"tezosign/repos"
	"tezosign/services"
	"tezosign/types"

	"github.com/gorilla/mux"
)

func (api *API) AddressIsRevealed(w http.ResponseWriter, r *http.Request) {
	net, networkContext, err := GetNetworkContext(r)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	address := types.Address(mux.Vars(r)["address"])
	if address == "" || address.Validate() != nil {
		response.JsonError(w, apperrors.New(apperrors.ErrBadParam, "address"))
		return
	}

	service := services.New(repos.New(networkContext.Db), repos.New(networkContext.IndexerDB), networkContext.Client, networkContext.Auth, net)

	isRevealed, err := service.AddressRevealed(address)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	isRevealed = true
	response.Json(w, map[string]interface{}{
		"revealed": isRevealed,
	})
}

func (api *API) AddressBalance(w http.ResponseWriter, r *http.Request) {
	net, networkContext, err := GetNetworkContext(r)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	address := types.Address(mux.Vars(r)["address"])
	if address == "" || address.Validate() != nil {
		response.JsonError(w, apperrors.New(apperrors.ErrBadParam, "address"))
		return
	}

	service := services.New(repos.New(networkContext.Db), repos.New(networkContext.IndexerDB), networkContext.Client, networkContext.Auth, net)

	balance, err := service.AddressBalance(address)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	response.Json(w, map[string]interface{}{
		"balance": balance,
	})
}

func (api *API) AddressContracts(w http.ResponseWriter, r *http.Request) {
	user, net, networkContext, err := GetUserNetworkContext(r)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	service := services.New(repos.New(networkContext.Db), repos.New(networkContext.IndexerDB), networkContext.Client, networkContext.Auth, net)

	contracts, err := service.GetAccountContracts(user)
	if err != nil {
		response.JsonError(w, err)
		return
	}

	response.Json(w, contracts)
}
