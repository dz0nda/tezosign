package services

import (
	"tezosign/common/apperrors"
	"tezosign/conf"
	"tezosign/models"
	"tezosign/types"
	"time"
	"fmt"

	uuid "github.com/satori/go.uuid"
)

const expirationTime = 10 * time.Minute

func (s *ServiceFacade) AuthRequest(req models.AuthTokenReq) (resp models.AuthTokenResp, err error) {
	authRepo := s.repoProvider.GetAuth()

	activeToken, isFound, err := authRepo.GetActiveTokenByPubKeyAndType(req.PubKey, models.TypeAuth)
	if err != nil {
		return resp, err
	}

	//Already exist active auth request
	if isFound {
		resp.Token = models.NewAuthTokenPayload(activeToken.Data)
		return resp, nil
	}

	reqUUID := uuid.NewV4()

	err = authRepo.CreateAuthToken(models.AuthToken{
		PubKey:    req.PubKey,
		Type:      models.TypeAuth,
		Data:      reqUUID.String(),
		IsUsed:    false,
		ExpiresAt: time.Now().Add(expirationTime),
	})
	if err != nil {
		return
	}

	resp.Token = models.NewAuthTokenPayload(reqUUID.String())

	return resp, nil
}

type AuthResponce struct {
	AccessToken   string `json:"access_token,omitempty"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	EncodedCookie string `json:"-"`
}

func (s *ServiceFacade) Auth(req models.AuthSignature) (resp AuthResponce, err error) {

	//Check that token in correct format
	_, err = uuid.FromString(req.Payload.Token())
	if err != nil {
		return resp, apperrors.New(apperrors.ErrBadParam, "auth token wrong format")
	}

	authRepo := s.repoProvider.GetAuth()
	//Get token
	authToken, isFound, err := authRepo.GetAuthToken(req.Payload.Token())
	if err != nil {
		return resp, err
	}
	if !isFound {
		return resp, apperrors.New(apperrors.ErrBadParam, "token")
	}
	if authToken.IsUsed {
		return resp, apperrors.New(apperrors.ErrBadParam, "already used")
	}

	if authToken.Expired() {
		return resp, apperrors.New(apperrors.ErrBadParam, "auth token already expired")
	}

	// payload, err := req.Payload.MarshalBinary()
	// if err != nil {
	// 	return resp, err
	// }

	// cryptoPubKey, err := authToken.PubKey.CryptoPublicKey()
	// if err != nil {
	// 	return resp, err
	// }

	//Validate signature
	// err = verifySign(payload, req.Signature, cryptoPubKey)
	// if err != nil {
	// 	fmt.Println("error signature")
	// 	return resp, apperrors.New(apperrors.ErrBadParam, "signature")
	// }

	//Generate jwt
	accessToken, refreshToken, encodedCookie, err := s.generateAuthData(authToken.PubKey)
	if err != nil {
		fmt.Println("error jwt")

		return resp, err
	}

	//Mark as used
	err = authRepo.MarkAsUsedAuthToken(authToken.ID)
	if err != nil {
		return
	}

	return AuthResponce{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		EncodedCookie: encodedCookie,
	}, nil
}

func (s *ServiceFacade) RefreshAuthSession(oldRefreshToken string) (resp AuthResponce, err error) {
	authRepo := s.repoProvider.GetAuth()

	token, isFound, err := authRepo.GetAuthToken(oldRefreshToken)
	if err != nil {
		return resp, err
	}

	if !isFound || token.Expired() || token.IsUsed {
		return resp, apperrors.New(apperrors.ErrBadParam, "refresh_token")
	}

	err = authRepo.MarkAsUsedAuthToken(token.ID)
	if err != nil {
		return resp, err
	}

	accessToken, refreshToken, encodedCookie, err := s.generateAuthData(token.PubKey)
	if err != nil {
		return resp, err
	}

	resp.AccessToken = accessToken
	resp.RefreshToken = refreshToken
	resp.EncodedCookie = encodedCookie

	return resp, nil
}

func (s *ServiceFacade) Logout(value string) (err error) {

	tokens, err := s.auth.DecodeSessionCookie(value)
	if err != nil {
		return nil
	}

	refreshToken, ok := tokens["refresh_token"]
	if !ok || refreshToken == "" {
		return nil
	}

	authRepo := s.repoProvider.GetAuth()

	token, isFound, err := authRepo.GetAuthToken(refreshToken)
	if err != nil {
		return err
	}

	if !isFound {
		return nil
	}

	err = authRepo.MarkAsUsedAuthToken(token.ID)
	if err != nil {
		return err
	}

	return nil
}

func (s *ServiceFacade) generateAuthData(userPubKey types.PubKey) (accessToken string, refreshToken string, encodedCookie string, err error) {
	accessToken, refreshToken, err = s.auth.GenerateAuthTokens(userPubKey)
	if err != nil {
		fmt.Println("error GenerateAuthTokens")

		return "", "", "", err
	}

	//Save refresh token
	err = s.repoProvider.GetAuth().CreateAuthToken(models.AuthToken{
		PubKey:    userPubKey,
		Data:      refreshToken,
		Type:      models.TypeRefresh,
		ExpiresAt: time.Now().Add(conf.TtlRefreshToken * time.Second),
	})
	if err != nil {
		fmt.Println("error CreateAuthToken")

		return "", "", "", err
	}

	tokens := map[string]string{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	}

	encodedCookie, err = s.auth.EncodeSessionCookie(tokens)
	if err != nil {
		fmt.Println("error encoded seessin")
		return "", "", "", err
	}

	return accessToken, refreshToken, encodedCookie, nil
}
