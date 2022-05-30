package api

import (
	"fmt"
	"strings"
	"tezosign/models"
)

func ToNetwork(net string) (models.Network, error) {
	switch strings.ToLower(net) {
	case "main", "mainnet":
		return models.NetworkMain, nil
	case "delphi", "delphinet":
		return models.NetworkDelphi, nil
	case "edo", "edonet":
		return models.NetworkEdo, nil
	case "florence", "florencenet":
		return models.NetworkFlorence, nil
	case "ithaca", "ithacanet":
		return models.NetworkIthaca, nil
	case "jakarta", "jakartanet":
		return models.NetworkJakarta, nil
	}

	return "", fmt.Errorf("not supported network")
}
