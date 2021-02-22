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

func (s *ServiceFacade) BuildContractStorageUpdateOperation(user, contractID types.Address, req models.ContractStorageRequest) (resp models.Request, err error) {
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

	isOwner, err := s.GetUserAllowance(user, contractID)
	if err != nil {
		return resp, err
	}

	if !isOwner {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	resp, err = s.ContractOperation(user, models.ContractOperationRequest{
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
	var pubKey string
	pubKeys = make([]types.PubKey, len(entities))

	for i := range entities {
		if entities[i].IsPubKey() {
			pubKeys[i] = entities[i].PubKey()
			continue
		}

		//TODO probably use indexed db
		pubKey, err = s.rpcClient.ManagerKey(context.Background(), entities[i].Address().String())
		if err != nil {
			return
		}

		if len(pubKey) == 0 {
			return nil, apperrors.New(apperrors.ErrBadParam, "address")
		}

		pubKeys[i] = types.PubKey(pubKey)
	}

	return pubKeys, err
}

func (s *ServiceFacade) ContractInfo(contractID types.Address) (resp models.ContractInfo, err error) {
	//Get contact
	storage, err := s.getContractStorage(contractID.String())
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

	balance, err := s.rpcClient.Balance(context.Background(), contractID.String())
	if err != nil {
		return resp, err
	}

	return models.ContractInfo{
		Address:   contractID,
		Balance:   balance,
		Threshold: storage.Threshold(),
		Counter:   storage.Counter(),
		Owners:    owners,
	}, nil
}

func (s *ServiceFacade) ContractOperation(user types.Address, req models.ContractOperationRequest) (resp models.Request, err error) {
	isOwner, err := s.GetUserAllowance(user, req.ContractID)
	if err != nil {
		return resp, err
	}

	if !isOwner {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	chainID, err := s.rpcClient.ChainID(context.Background())
	if err != nil {
		return resp, err
	}

	//Get contact
	storage, err := s.getContractStorage(req.ContractID.String())
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

	if req.Type == models.FATransfer {
		isFAContract, err := s.checkFAStandart(req.AssetID.String())
		if err != nil {
			return resp, err
		}

		if !isFAContract {
			return resp, apperrors.New(apperrors.ErrBadParam, "not FA asset contract")
		}
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

//TODO move to middleware
func (s *ServiceFacade) GetUserAllowance(userAddress, contractAddress types.Address) (isOwner bool, err error) {

	storage, err := s.getContractStorage(contractAddress.String())
	if err != nil {
		return false, err
	}

	pubKey, err := s.rpcClient.ManagerKey(context.Background(), userAddress.String())
	if err != nil {
		return false, err
	}

	_, isOwner = storage.Contains(types.PubKey(pubKey))

	return isOwner, nil
}

func (s *ServiceFacade) BuildContractOperationToSign(user types.Address, txID string, payloadType models.PayloadType) (resp models.OperationToSignResp, err error) {

	repo := s.repoProvider.GetContract()
	operationReq, isFound, err := repo.GetPayloadByHash(txID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotFound, "payload")
	}

	contractModel, err := repo.GetContractByID(operationReq.ContractID)
	if err != nil {
		return resp, err
	}

	isOwner, err := s.GetUserAllowance(user, contractModel.Address)
	if err != nil {
		return resp, err
	}

	if !isOwner {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	if operationReq.Counter == nil {
		return resp, errors.New("Empty operation counter")
	}

	counter := *operationReq.Counter
	var signPayload types.Payload
	if payloadType == models.TypeReject {
		signPayload, err = contract.BuildRejectSignPayload(operationReq.NetworkID, counter, contractModel.Address)
	} else {
		signPayload, err = contract.BuildContractSignPayload(operationReq.NetworkID, counter, operationReq.Info)
	}
	if err != nil {
		return resp, err
	}

	return models.OperationToSignResp{
		OperationID: operationReq.Hash,
		Payload:     signPayload,
	}, nil
}

func (s *ServiceFacade) BuildContractOperation(userAddress types.Address, txID string, payloadType models.PayloadType) (resp models.OperationParameter, err error) {
	//get payload by ID
	repo := s.repoProvider.GetContract()

	payload, isFound, err := repo.GetPayloadByHash(txID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotFound, "payload")
	}

	contr, err := repo.GetContractByID(payload.ContractID)
	if err != nil {
		return resp, err
	}

	//Get contact
	storage, err := s.getContractStorage(contr.Address.String())
	if err != nil {
		return resp, err
	}

	pubKey, err := s.rpcClient.ManagerKey(context.Background(), userAddress.String())
	if err != nil {
		return resp, err
	}

	//Check user allowance
	_, isOwner := storage.Contains(types.PubKey(pubKey))
	if !isOwner {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotAllowed, "pubkey not contains in storage")
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

	operationPayload, err := s.BuildContractOperationToSign(userAddress, txID, payloadType)
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

func (s *ServiceFacade) SaveContractOperationSignature(userAddress types.Address, operationID string, req models.OperationSignature) (resp models.OperationSignatureResp, err error) {

	storage, err := s.getContractStorage(req.ContractID.String())
	if err != nil {
		return resp, err
	}

	index, isFound := storage.Contains(req.PubKey)
	if !isFound {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotAllowed, "pubkey not contains in storage")
	}

	repo := s.repoProvider.GetContract()

	payload, isFound, err := repo.GetPayloadByHash(operationID)
	if err != nil {
		return resp, err
	}

	if !isFound {
		return resp, apperrors.NewWithDesc(apperrors.ErrNotFound, "operation")
	}

	//Check sign with pubkey
	pubKey, err := req.PubKey.CryptoPublicKey()
	if err != nil {
		return resp, err
	}

	operationPayload, err := s.BuildContractOperationToSign(userAddress, operationID, req.Type)
	if err != nil {
		return resp, err
	}

	bt, err := operationPayload.Payload.MarshalBinary()
	if err != nil {
		return resp, err
	}

	err = verifySign(bt, req.Signature.String(), pubKey)
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

func (s *ServiceFacade) getContractStorage(contractID string) (storage contract.ContractStorageContainer, err error) {
	rawStorage, err := s.rpcClient.Storage(context.Background(), contractID)
	if err != nil {
		return storage, err
	}

	storage, err = contract.NewContractStorageContainer(rawStorage)
	if err != nil {
		return storage, fmt.Errorf("%v; %w", err, apperrors.NewWithDesc(apperrors.ErrBadParam, "wrong contract type"))
	}

	return storage, err
}

func (s *ServiceFacade) checkFAStandart(contractID string) (isFAContract bool, err error) {
	script, err := s.rpcClient.Script(context.Background(), contractID)
	if err != nil {
		return false, err
	}

	return contract.CheckFATransferMethod(&script), nil
}

func operationID(payload string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(payload)))
}

//Verify signed payload
func verifySign(message []byte, signature string, publicKey crypto.PublicKey) error {
	// hash
	//TODO check Wallets sign with P256 and secp256k1 curves
	payloadHash := blake2b.Sum256(message)

	// verify signature over hash
	sigPrefix, sigBytes, err := tezosprotocol.Base58CheckDecode(signature)
	if err != nil {
		return errors.Errorf("failed to decode signature: %s: %s", signature, err)
	}
	var ok bool
	switch key := publicKey.(type) {
	case ed25519.PublicKey:
		if sigPrefix != tezosprotocol.PrefixEd25519Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
			return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		}
		ok = ed25519.Verify(key, payloadHash[:], sigBytes)
	//P256 curve
	case ecdsa.PublicKey:
		//if sigPrefix != tezosprotocol.PrefixP256Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
		//	log.Print(sigPrefix.PrefixBytes())
		//	return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		//}

		sig, err := deserializeSig(sigBytes)
		if err != nil {
			return err
		}

		ok = ecdsa.Verify(&key, payloadHash[:], sig.R, sig.S)

	// Secp256P1 curve
	case secp256k1.PubKey:
		if sigPrefix != tezosprotocol.PrefixSecp256k1Signature && sigPrefix != tezosprotocol.PrefixGenericSignature {
			return errors.Errorf("signature type %s does not match public key type %T", sigPrefix, publicKey)
		}

		ok = key.VerifySignature(payloadHash[:], sigBytes)
	default:
		return errors.Errorf("unsupported public key type: %T", publicKey)
	}
	if !ok {
		return errors.Errorf("invalid signature %s for public key %s", signature, publicKey)
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
