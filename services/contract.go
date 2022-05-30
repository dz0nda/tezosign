package services

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/md5"
	"fmt"
	"math/big"
	"tezosign/common/apperrors"
	"tezosign/models"
	"tezosign/services/contract"
	"tezosign/types"
	"time"

	"blockwatch.cc/tzindex/micheline"

	"github.com/anchorageoss/tezosprotocol/v2"
	"github.com/btcsuite/btcd/btcec"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/crypto/secp256k1"
	"golang.org/x/crypto/blake2b"
)

const maxEntitiesNum = 20

func (s *ServiceFacade) BuildContractInitStorage(req models.ContractStorageRequest) (resp []byte, err error) {

	if req.Threshold > uint(len(req.Entities)) {
		return nil, apperrors.New(apperrors.ErrBadParam, "threshold")
	}

	if len(req.Entities) > maxEntitiesNum {
		return nil, apperrors.New(apperrors.ErrBadParam, "addresses num")
	}

	//Add native support of pubkeys
	pubKeys, err := s.getPubKeys(req.Threshold, req.Entities)
	if err != nil {
		return nil, err
	}

	resp, err = contract.BuildContractStorage(req.Threshold, pubKeys)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (s *ServiceFacade) BuildContractStorageUpdateOperation(userPubKey types.PubKey, contractID types.Address, req models.ContractStorageRequest) (resp models.Request, err error) {
	if req.Threshold > uint(len(req.Entities)) {
		return resp, apperrors.New(apperrors.ErrBadParam, "threshold")
	}

	if len(req.Entities) > maxEntitiesNum {
		return resp, apperrors.New(apperrors.ErrBadParam, "addresses num")
	}

	pubKeys, err := s.getPubKeys(req.Threshold, req.Entities)
	if err != nil {
		return resp, err
	}

	resp, err = s.ContractOperation(userPubKey, models.ContractOperationRequest{
		ContractID: contractID,
		Type:       models.StorageUpdate,
		Threshold:  req.Threshold,
		Keys:       pubKeys,
	})
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (s *ServiceFacade) getPubKeys(threshold uint, entities []models.StorageEntity) (pubKeys []types.PubKey, err error) {
	var revealOp models.RevealOperation
	var isFound bool
	pubKeys = make([]types.PubKey, len(entities))

	for i := range entities {
		if entities[i].IsPubKey() {
			pubKeys[i] = entities[i].PubKey()
			continue
		}

		revealOp, isFound, err = s.indexerRepoProvider.GetIndexer().GetContractRevealOperation(entities[i].Address())
		if err != nil {
			return pubKeys, err
		}

		if !isFound {
			return pubKeys, apperrors.New(apperrors.ErrBadParam, fmt.Sprintf("address %s is not revealed", entities[i].Address().String()))
		}

		pubKeys[i] = revealOp.PublicKey
	}

	return pubKeys, err
}

func (s *ServiceFacade) ContractInfo(contractID types.Address) (resp models.ContractInfo, err error) {
	//Get contact
	storage, err := s.getMsigContractStorage(contractID)
	if err != nil {
		return resp, err
	}

	//Init contract if not created before
	_, err = s.repoProvider.GetContract().GetOrCreateContract(contractID)
	if err != nil {
		return resp, err
	}

	var address types.Address
	owners := make([]models.Owner, len(storage.PubKeys()))
	for i := range storage.PubKeys() {

		address, err = storage.PubKeys()[i].Address()
		if err != nil {
			return resp, err
		}

		owners[i] = models.Owner{
			PubKey:  storage.PubKeys()[i],
			Address: address,
		}
	}

	acc, isFound, err := s.indexerRepoProvider.GetIndexer().GetAccount(contractID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.New(apperrors.ErrNotFound, "contract")
	}

	return models.ContractInfo{
		Address:   contractID,
		Balance:   acc.Balance,
		Threshold: storage.Threshold(),
		Counter:   storage.Counter(),
		Owners:    owners,
	}, nil
}

func (s *ServiceFacade) ContractOperation(userPubKey types.PubKey, req models.ContractOperationRequest) (resp models.Request, err error) {
	isOwner, err := s.GetUserAllowance(userPubKey, req.ContractID)
	if err != nil {
		return resp, err
	}

	if !isOwner {
		return resp, apperrors.New(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	//Get contact
	storage, err := s.getMsigContractStorage(req.ContractID)
	if err != nil {
		return resp, err
	}

	chainID, err := s.rpcClient.ChainID(context.Background())
	if err != nil {
		return resp, err
	}

	//Specific checks
	err = s.checkOperation(req)
	if err != nil {
		return resp, err
	}

	repo := s.repoProvider.GetContract()
	contr, err := repo.GetOrCreateContract(req.ContractID)
	if err != nil {
		return resp, err
	}

	//Counter
	counter := storage.Counter()
	pendingNonce, err := repo.GetMaxContractPendingNone(contr.ID)
	if err != nil {
		return resp, err
	}

	//Increment counter only if pending nonce exist
	if pendingNonce.Valid && pendingNonce.Int64 >= counter {
		//Increment counter for next operation
		counter = pendingNonce.Int64 + 1
	}

	//TODO change format
	operationID := operationID(fmt.Sprintf("nonce%dnetwork%scontract%spayload%x", counter, chainID, req.ContractID, req))

	//Try to found already exists payload
	_, isFound, err := repo.GetPayloadByHash(operationID)
	if err != nil {
		return resp, err
	}

	request := models.Request{
		ContractID: contr.ID,
		Hash:       operationID,
		Counter:    &counter,
		Info:       req,
		NetworkID:  chainID,
		Status:     models.StatusPending,
		CreatedAt:  types.JSONTimestamp(time.Now()),
	}

	//Create new
	if !isFound {
		err = repo.SavePayload(request)
		if err != nil {
			return resp, err
		}
	}

	return request, nil
}

func (s *ServiceFacade) checkOperation(req models.ContractOperationRequest) (err error) {
	switch req.Type {
	//Check account balance
	case models.Transfer:
		acc, isFound, err := s.indexerRepoProvider.GetIndexer().GetAccount(req.ContractID)
		if err != nil {
			return err
		}

		if !isFound {
			return apperrors.New(apperrors.ErrNotFound, "account")
		}

		//TODO count fee
		if req.Amount > acc.Balance {
			return apperrors.New(apperrors.ErrNotAllowed, "not enough balance")
		}
	//Check FA standart and balance
	case models.FATransfer, models.FA2Transfer:
		assetType := models.TypeFA12
		if req.Type == models.FA2Transfer {
			assetType = models.TypeFA2
		}

		isFAContract, err := s.checkFAStandart(req.AssetID, assetType)
		if err != nil {
			return err
		}

		if !isFAContract {
			return apperrors.New(apperrors.ErrBadParam, "not FA asset contract")
		}

		//TODO check FA balance
	case models.VestingSetDelegate, models.VestingVest:
		//Сheck contract for vesting type
		vestingStorage, err := s.getVestingContractStorage(req.VestingID)
		if err != nil {
			return err
		}

		if req.Type == models.VestingSetDelegate && vestingStorage.DelegateAdmin != req.ContractID {
			return apperrors.New(apperrors.ErrNotAllowed, "msig is not delegate admin")
		}

		if req.Type == models.VestingVest && vestingStorage.OpenedTicks() < req.Ticks {
			return apperrors.New(apperrors.ErrNotAllowed, "not enough ticks")
		}

	}

	return nil
}

func (s *ServiceFacade) GetUserAllowance(userPubKey types.PubKey, contractAddress types.Address) (isOwner bool, err error) {

	storage, err := s.getMsigContractStorage(contractAddress)
	if err != nil {
		return false, err
	}

	_, isOwner = storage.Contains(userPubKey)

	return isOwner, nil
}

func (s *ServiceFacade) BuildContractOperationToSign(userPubKey types.PubKey, txID string, payloadType models.PayloadType) (resp models.OperationToSignResp, err error) {

	repo := s.repoProvider.GetContract()
	operationReq, isFound, err := repo.GetPayloadByHash(txID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.New(apperrors.ErrNotFound, "payload")
	}

	contractModel, err := repo.GetContractByID(operationReq.ContractID)
	if err != nil {
		return resp, err
	}

	isOwner, err := s.GetUserAllowance(userPubKey, contractModel.Address)
	if err != nil {
		return resp, err
	}

	if !isOwner {
		return resp, apperrors.New(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	if operationReq.Counter == nil {
		return resp, errors.New("Empty operation counter")
	}

	counter := *operationReq.Counter
	var signPayload types.Payload
	var payloadJson string
	if payloadType == models.TypeReject {
		signPayload, payloadJson, err = contract.BuildRejectSignPayload(operationReq.NetworkID, counter, contractModel.Address)
	} else {
		signPayload, payloadJson, err = contract.BuildContractSignPayload(operationReq.NetworkID, counter, operationReq.Info)
	}
	if err != nil {
		return resp, err
	}

	return models.OperationToSignResp{
		OperationID: operationReq.Hash,
		Payload:     signPayload,
		PayloadJSON: payloadJson,
	}, nil
}

func (s *ServiceFacade) BuildContractOperation(userPubKey types.PubKey, txID string, payloadType models.PayloadType) (resp models.OperationParameter, err error) {
	//get payload by ID
	repo := s.repoProvider.GetContract()

	payload, isFound, err := repo.GetPayloadByHash(txID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.New(apperrors.ErrNotFound, "payload")
	}

	contr, err := repo.GetContractByID(payload.ContractID)
	if err != nil {
		return resp, err
	}

	//Get contact
	storage, err := s.getMsigContractStorage(contr.Address)
	if err != nil {
		return resp, err
	}

	//Check user allowance
	_, isOwner := storage.Contains(userPubKey)
	if !isOwner {
		return resp, apperrors.New(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	//get signatures by payload ID
	sigs, err := repo.GetSignaturesByPayloadID(payload.ID, payloadType)
	if err != nil {
		return resp, err
	}

	//Make array with empty signatures
	signatures := make([]types.Signature, len(storage.PubKeys()))
	for i := range sigs {
		signatures[sigs[i].Index] = sigs[i].Signature
	}

	operationPayload, err := s.BuildContractOperationToSign(userPubKey, txID, payloadType)
	if err != nil {
		return resp, err
	}

	rawTx, entrypoint, err := contract.BuildFullTxPayload(operationPayload.Payload, signatures)
	if err != nil {
		return resp, err
	}

	return models.OperationParameter{
		Entrypoint: entrypoint,
		Value:      string(rawTx),
	}, nil
}

func (s *ServiceFacade) CheckContractOrigination(txID string) (contract types.Address, err error) {

	tx, isFound, err := s.indexerRepoProvider.GetIndexer().GetContractOriginationOperation(txID)
	if err != nil {
		return contract, nil
	}

	if !isFound {
		return contract, apperrors.New(apperrors.ErrNotFound, "tx")
	}

	return tx.ContractAddress, nil
}

func (s *ServiceFacade) SaveContractOperationSignature(userPubKey types.PubKey, operationID string, req models.OperationSignature) (resp models.OperationSignatureResp, err error) {

	storage, err := s.getMsigContractStorage(req.ContractID)
	if err != nil {
		return resp, err
	}

	index, isFound := storage.Contains(req.PubKey)
	if !isFound {
		return resp, apperrors.New(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	repo := s.repoProvider.GetContract()

	payload, isFound, err := repo.GetPayloadByHash(operationID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.New(apperrors.ErrNotFound, "operation")
	}

	//Check sign with pubkey
	pubKey, err := req.PubKey.CryptoPublicKey()
	if err != nil {
		return resp, err
	}

	operationPayload, err := s.BuildContractOperationToSign(userPubKey, operationID, req.Type)
	if err != nil {
		return resp, err
	}

	bt, err := operationPayload.Payload.MarshalBinary()
	if err != nil {
		return resp, err
	}

	err = verifySign(bt, req.Signature, pubKey)
	if err != nil {
		return resp, err
	}

	_, isFound, err = repo.GetPayloadSignature(req.Signature)
	if err != nil {
		return resp, err
	}

	if !isFound {
		//Save signature
		err = repo.SavePayloadSignature(models.Signature{
			RequestID: payload.ID,
			Index:     index,
			Signature: req.Signature,
			Type:      req.Type,
		})
		if err != nil {
			return resp, err
		}
	}

	count, err := repo.GetSignaturesCount(payload.ID)
	if err != nil {
		return resp, err
	}

	return models.OperationSignatureResp{
		SigCount:  count,
		Threshold: storage.Threshold(),
	}, nil
}

func (s *ServiceFacade) GetAccountContracts(userPubKey types.PubKey) ([]string, error) {

	//Request without limit
	contracts, err := s.repoProvider.GetContract().GetContractsList(0, 0)
	if err != nil {
		return nil, err
	}

	msigContractsList := make([]string, len(contracts))
	for i := range contracts {
		msigContractsList[i] = contracts[i].Address.String()
	}

	userContracts, err := s.indexerRepoProvider.GetIndexer().GetContractsStoragesContainsKey(msigContractsList, userPubKey.String())
	if err != nil {
		return nil, err
	}

	return userContracts, nil
}

func (s *ServiceFacade) getMsigContractStorage(contractID types.Address) (storageContainer contract.ContractStorageContainer, err error) {

	script, storage, err := s.getContractScriptAndStorage(contractID)
	if err != nil {
		return storageContainer, err
	}

	storageContainer, err = contract.NewContractStorageContainer(micheline.Script{
		Code: &micheline.Code{
			Param:   script.ParameterSchema.MichelinePrim(),
			Storage: script.StorageSchema.MichelinePrim(),
			Code:    script.CodeSchema.MichelinePrim(),
		},
		Storage: storage.RawValue.MichelinePrim(),
	})
	if err != nil {
		return storageContainer, fmt.Errorf("%v; %w", err, apperrors.New(apperrors.ErrBadParam, "wrong contract type"))
	}

	return storageContainer, err
}

func (s *ServiceFacade) checkFAStandart(contractID types.Address, assetType models.AssetType) (isFAContract bool, err error) {

	script, isFound, err := s.indexerRepoProvider.GetIndexer().GetContractScript(contractID)
	if err != nil {
		return false, err
	}

	if !isFound {
		return false, apperrors.New(apperrors.ErrNotFound, "contract")
	}

	return contract.CheckFATransferMethod(&micheline.Script{
		Code: &micheline.Code{
			Param:   script.ParameterSchema.MichelinePrim(),
			Storage: script.StorageSchema.MichelinePrim(),
			Code:    script.CodeSchema.MichelinePrim(),
		},
	}, assetType), nil
}

func (s *ServiceFacade) getVestingContractStorage(contractID types.Address) (storageContainer contract.VestingContractStorageContainer, err error) {

	script, storage, err := s.getContractScriptAndStorage(contractID)
	if err != nil {
		return storageContainer, err
	}

	storageContainer, err = contract.NewVestingContractStorageContainer(micheline.Script{
		Code: &micheline.Code{
			Param:   script.ParameterSchema.MichelinePrim(),
			Storage: script.StorageSchema.MichelinePrim(),
			Code:    script.CodeSchema.MichelinePrim(),
		},
		Storage: storage.RawValue.MichelinePrim()})
	if err != nil {
		return storageContainer, fmt.Errorf("%v; %w", err, apperrors.New(apperrors.ErrBadParam, "wrong contract type"))
	}

	return storageContainer, nil
}

func (s *ServiceFacade) checkVestingContract(contractID types.Address) (isVestingContract bool, err error) {

	script, isFound, err := s.indexerRepoProvider.GetIndexer().GetContractScript(contractID)
	if err != nil {
		return false, err
	}

	if !isFound {
		return false, apperrors.New(apperrors.ErrNotFound, "contract")
	}

	return contract.CheckVestingContractStorage(micheline.Script{
		Code: &micheline.Code{
			Param:   script.ParameterSchema.MichelinePrim(),
			Storage: script.StorageSchema.MichelinePrim(),
			Code:    script.CodeSchema.MichelinePrim(),
		}}), nil

	return true, nil
}

func (s *ServiceFacade) getContractScriptAndStorage(contractID types.Address) (script models.Script, storage models.Storage, err error) {

	var isFound bool

	indexerRepo := s.indexerRepoProvider.GetIndexer()
	script, isFound, err = indexerRepo.GetContractScript(contractID)
	if err != nil {
		return script, storage, err
	}

	if !isFound {
		return script, storage, apperrors.New(apperrors.ErrNotFound, "contract")
	}

	storage, isFound, err = indexerRepo.GetContractStorage(contractID)
	if err != nil {
		return script, storage, err
	}

	if !isFound {
		return script, storage, apperrors.New(apperrors.ErrNotFound, "contract")
	}

	return script, storage, nil
}

func operationID(payload string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(payload)))
}

//Verify signed payload
func verifySign(message []byte, signature types.Signature, publicKey crypto.PublicKey) error {
	// hash
	//TODO check Wallets sign with P256 and secp256k1 curves
	payloadHash := blake2b.Sum256(message)

	// verify signature over hash
	sigPrefix, sigBytes, err := tezosprotocol.Base58CheckDecode(signature.String())
	if err != nil {
		return errors.Errorf("failed to decode signature: %s: %s", signature, err)
	}
	var ok bool
	switch key := publicKey.(type) {
	case ed25519.PublicKey:
		fmt.Println("ed25519 pubkey");

		if sigPrefix != tezosprotocol.PrefixEd25519Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
			return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		}
		fmt.Println(key);
		fmt.Println(payloadHash[:]);
		fmt.Println(sigBytes);

		ok = ed25519.Verify(key, payloadHash[:], sigBytes)
	//P256 curve
	case ecdsa.PublicKey:
		if sigPrefix != tezosprotocol.PrefixP256Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
			fmt.Println(sigPrefix.PrefixBytes())
			return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		}

		sig, err := deserializeSig(sigBytes)
		if err != nil {
			return err
		}

		ok = ecdsa.Verify(&key, payloadHash[:], sig.R, sig.S)

	// Secp256P1 curve
	case secp256k1.PubKey:
		fmt.Println("secp256k1 pubkey");

		if sigPrefix != tezosprotocol.PrefixSecp256k1Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
			return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		}

		ok = key.VerifySignature(payloadHash[:], sigBytes)
	default:
		return errors.Errorf("unsupported public key type: %T", publicKey)
	}
	if !ok {
		fmt.Println("not ok");
		return errors.Errorf("invalid signature %s for public key %v", signature, publicKey)
	}
	return nil
}

// Serialize signature to R || S.
// R, S are padded to 32 bytes respectively.
func serializeSig(sig *btcec.Signature) []byte {
	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()
	sigBytes := make([]byte, 64)
	// 0 pad the byte arrays from the left if they aren't big enough.
	copy(sigBytes[32-len(rBytes):32], rBytes)
	copy(sigBytes[64-len(sBytes):64], sBytes)
	return sigBytes
}

func deserializeSig(serializedSig []byte) (sig btcec.Signature, err error) {
	if len(serializedSig) != 64 {
		return sig, fmt.Errorf("Wrong serialized sig len")
	}

	rBytes := &big.Int{}
	sBytes := &big.Int{}

	rBytes.FillBytes(serializedSig[:32])
	sBytes.FillBytes(serializedSig[32:])

	return btcec.Signature{
		R: rBytes,
		S: sBytes,
	}, nil
}
