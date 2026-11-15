package webpush

import (
	"encoding/asn1"
	"math/big"
)

// asn1Signature turns the JWS form (r and s side by side) back into the ASN.1
// form ecdsa.VerifyASN1 reads.
func asn1Signature(sig []byte) []byte {
	out, _ := asn1.Marshal(struct{ R, S *big.Int }{new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])})
	return out
}
