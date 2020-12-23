package rpc_client

import (
	"context"
	"encoding/json"
	"msig/models"
	"msig/services/rpc_client/client"
	"msig/services/rpc_client/client/contracts"
)

const headBlock = "head"
const BlocksInCycle = 4096

type Tezos struct {
	client        *client.Tezosrpc
	network       models.Network
	isTestNetwork bool //we have to use a separate flag due to stupid nodes configs...
}

func New(cfg client.TransportConfig, network models.Network, isTestNetwork bool) *Tezos {
	cli := client.NewHTTPClientWithConfig(nil, &cfg)

	return &Tezos{
		client:        cli,
		network:       network,
		isTestNetwork: isTestNetwork,
	}
}

func (t *Tezos) Script(ctx context.Context, contractHash string) (bm models.BigMap, err error) {
	params := contracts.NewGetContractScriptParamsWithContext(ctx).WithContract(contractHash)
	resp, err := t.client.Contracts.GetContractScript(params)
	if err != nil {
		return bm, err
	}

	bytes, err := json.Marshal(resp.Payload)
	if err != nil {
		return bm, err
	}

	err = json.Unmarshal(bytes, &bm)
	if err != nil {
		return bm, err
	}

	return bm, nil
}

func (t *Tezos) ManagerKey(ctx context.Context, address string) (pubKey string, err error) {
	params := contracts.NewGetContractManagerKeyParamsWithContext(ctx).WithContract(address)
	resp, err := t.client.Contracts.GetContractManagerKey(params)
	if err != nil {
		return pubKey, err
	}

	return resp.Payload, nil
}
