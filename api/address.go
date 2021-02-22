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

	response.Json(w, map[string]interface{}{
		"revealed": isRevealed,
	})
}
