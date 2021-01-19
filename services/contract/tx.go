package contract

import (
	"blockwatch.cc/tzindex/micheline"
	"fmt"
	"msig/types"
)

func BuildFullTxPayload(payload types.Payload, signatures []types.Signature) (resp []byte, err error) {

	rawPayload, err := payload.MarshalBinary()
	if err != nil {
		return resp, err
	}

	if rawPayload[0] == TextWatermark {
		rawPayload = rawPayload[1:]
	}

	michelsonPayload := &micheline.Prim{}
	err = michelsonPayload.UnmarshalBinary(rawPayload)
	if err != nil {
		return resp, err
	}

	if michelsonPayload.OpCode != micheline.D_PAIR || len(michelsonPayload.Args) != 2 {
		return nil, fmt.Errorf("Wrong michelson payload")
	}

	signaturesParam := make([]*micheline.Prim, len(signatures))
	marshaledSig := make([]byte, 0, 33)
	for i := range signatures {
		if signatures[i].IsEmpty() {
			signaturesParam[i] = &micheline.Prim{
				Type:   micheline.PrimNullary,
				OpCode: micheline.D_NONE,
			}
			continue
		}

		marshaledSig, err = signatures[i].MarshalBinary()
		if err != nil {
			return resp, err
		}
		signaturesParam[i] = &micheline.Prim{
			Type:   micheline.PrimUnary,
			OpCode: micheline.D_SOME,
			Args: []*micheline.Prim{
				{
					Type:   micheline.PrimBytes,
					OpCode: micheline.T_BYTES,
					//Remove address byte
					Bytes: marshaledSig,
				},
			},
		}
	}

	actionParams := &micheline.Prim{
		Type:   micheline.PrimBinary,
		OpCode: micheline.D_PAIR,
		Args: []*micheline.Prim{
			//Take params without network consts
			michelsonPayload.Args[1],
			{
				Type:   micheline.PrimSequence,
				OpCode: micheline.T_LIST,
				Args:   signaturesParam,
			},
		},
	}

	resp, err = actionParams.MarshalJSON()
	if err != nil {
		return resp, err
	}

	return resp, nil
}