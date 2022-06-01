package models

import (
	"fmt"
	"strings"
	"tezosign/types"
	"time"
	"encoding/hex"
)

type TokenType string

const (
	TypeAuth    TokenType = "auth"
	TypeRefresh TokenType = "refresh"
)

type AuthToken struct {
	ID        uint64       `gorm:"column:atn_id;primaryKey"`
	PubKey    types.PubKey `gorm:"column:atn_pubkey"`
	Type      TokenType    `gorm:"column:atn_type"`
	Data      string       `gorm:"column:atn_data"` //token uuid
	IsUsed    bool         `gorm:"column:atn_is_used"`
	ExpiresAt time.Time    `gorm:"column:atn_expires_at"`
}

type AuthTokenReq struct {
	PubKey types.PubKey `json:"pub_key"`
}

func (r AuthTokenReq) Validate() (err error) {

	err = r.PubKey.Validate()
	if err != nil {
		return err
	}
	return nil
}

type AuthTokenResp struct {
	Token AuthTokenPayload `json:"token"`
}

func (r AuthToken) Expired() bool {
	return r.ExpiresAt.Before(time.Now())
}

type AuthSignature struct {
	Payload AuthTokenPayload `json:"payload"`
	SignatureReq
}

type AuthTokenPayload string

const (
	AuthTokenPayloadPrefix = "tzsignwallet-authpayload-"
	UUIDLength             = 36
)

func NewAuthTokenPayload(token string) AuthTokenPayload {
	return AuthTokenPayload(fmt.Sprintf("%s%s", AuthTokenPayloadPrefix, token))
}

func (t AuthTokenPayload) Token() string {
	tokens := strings.SplitAfter(string(t), AuthTokenPayloadPrefix)
	if len(tokens) != 2 {
		return ""
	}

	return tokens[1]
}

func (t AuthTokenPayload) Validate() (err error) {
	if !strings.HasPrefix(string(t), AuthTokenPayloadPrefix) {
		return fmt.Errorf("wrong payload format")
	}

	if len(strings.SplitAfter(string(t), AuthTokenPayloadPrefix)) != 2 {
		return fmt.Errorf("payload not presented")
	}

	return nil
}

//Convert UTF8 to bytes
func (t AuthTokenPayload) MarshalBinary() ([]byte, error) {
	bytes := hex.EncodeToString([]byte(hex.EncodeToString([]byte(string(t)))))
	rawPayload := "0501" + hex.EncodeToString([]byte(fmt.Sprint(len(bytes)))) + bytes

	return hex.DecodeString(rawPayload)
	// return []byte(string(t)), nil
}

func (s AuthSignature) Validate() (err error) {
	err = s.Payload.Validate()
	if err != nil {
		return err
	}

	err = s.SignatureReq.Validate()
	if err != nil {
		return err
	}
	return nil
}
