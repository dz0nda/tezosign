package services

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"tezosign/models"
	"tezosign/types"
)

//TODO make as URL
const betterCallDevAccountAPI = "https://api.better-call.dev/v1/account/%s/%s/token_balances?offset=0&size=10"

var bcdNetworks = map[models.Network]string{
	models.NetworkMain:     "mainnet",
	models.NetworkIthaca:	"ithacanet",
	models.NetworkJakarta: 	"jakartanet",
}

func getAccountTokensBalance(account types.Address, network models.Network) (balances models.AssetBalances, err error) {
	resp, err := http.Get(fmt.Sprintf(betterCallDevAccountAPI, bcdNetworks[network], account.String()))
	if err != nil {
		return balances, err
	}

	if resp.StatusCode != http.StatusOK {
		return balances, fmt.Errorf("Not OK status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return balances, fmt.Errorf("ReadAll error: %s", err.Error())
	}

	err = json.Unmarshal(body, &balances)
	if err != nil {
		return balances, fmt.Errorf("Unmarshal into baseResponse: %s", err.Error())
	}

	return balances, nil
}
