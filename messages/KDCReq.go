package messages

// Reference: https://www.ietf.org/rfc/rfc4120.txt
// Section: 5.4.1

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/jcmturner/gofork/encoding/asn1"
	"gopkg.in/jcmturner/gokrb5.v2/asn1tools"
	"gopkg.in/jcmturner/gokrb5.v2/config"
	"gopkg.in/jcmturner/gokrb5.v2/crypto"
	"gopkg.in/jcmturner/gokrb5.v2/iana"
	"gopkg.in/jcmturner/gokrb5.v2/iana/asnAppTag"
	"gopkg.in/jcmturner/gokrb5.v2/iana/flags"
	"gopkg.in/jcmturner/gokrb5.v2/iana/keyusage"
	"gopkg.in/jcmturner/gokrb5.v2/iana/msgtype"
	"gopkg.in/jcmturner/gokrb5.v2/iana/nametype"
	"gopkg.in/jcmturner/gokrb5.v2/iana/patype"
	"gopkg.in/jcmturner/gokrb5.v2/krberror"
	"gopkg.in/jcmturner/gokrb5.v2/types"
)

type marshalKDCReq struct {
	PVNO    int                  `asn1:"explicit,tag:1"`
	MsgType int                  `asn1:"explicit,tag:2"`
	PAData  types.PADataSequence `asn1:"explicit,optional,tag:3"`
	ReqBody asn1.RawValue        `asn1:"explicit,tag:4"`
}

// KDCReqFields represents the KRB_KDC_REQ fields.
type KDCReqFields struct {
	PVNO    int
	MsgType int
	PAData  types.PADataSequence
	ReqBody KDCReqBody
	Renewal bool
}

// ASReq implements RFC 4120 KRB_AS_REQ: https://tools.ietf.org/html/rfc4120#section-5.4.1.
type ASReq struct {
	KDCReqFields
}

// TGSReq implements RFC 4120 KRB_TGS_REQ: https://tools.ietf.org/html/rfc4120#section-5.4.1.
type TGSReq struct {
	KDCReqFields
}

type marshalKDCReqBody struct {
	KDCOptions  asn1.BitString      `asn1:"explicit,tag:0"`
	CName       types.PrincipalName `asn1:"explicit,optional,tag:1"`
	Realm       string              `asn1:"generalstring,explicit,tag:2"`
	SName       types.PrincipalName `asn1:"explicit,optional,tag:3"`
	From        time.Time           `asn1:"generalized,explicit,optional,tag:4"`
	Till        time.Time           `asn1:"generalized,explicit,tag:5"`
	RTime       time.Time           `asn1:"generalized,explicit,optional,tag:6"`
	Nonce       int                 `asn1:"explicit,tag:7"`
	EType       []int               `asn1:"explicit,tag:8"`
	Addresses   []types.HostAddress `asn1:"explicit,optional,tag:9"`
	EncAuthData types.EncryptedData `asn1:"explicit,optional,tag:10"`
	// Ticket needs to be a raw value as it is wrapped in an APPLICATION tag
	AdditionalTickets asn1.RawValue `asn1:"explicit,optional,tag:11"`
}

// KDCReqBody implements the KRB_KDC_REQ request body.
type KDCReqBody struct {
	KDCOptions        asn1.BitString      `asn1:"explicit,tag:0"`
	CName             types.PrincipalName `asn1:"explicit,optional,tag:1"`
	Realm             string              `asn1:"generalstring,explicit,tag:2"`
	SName             types.PrincipalName `asn1:"explicit,optional,tag:3"`
	From              time.Time           `asn1:"generalized,explicit,optional,tag:4"`
	Till              time.Time           `asn1:"generalized,explicit,tag:5"`
	RTime             time.Time           `asn1:"generalized,explicit,optional,tag:6"`
	Nonce             int                 `asn1:"explicit,tag:7"`
	EType             []int               `asn1:"explicit,tag:8"`
	Addresses         []types.HostAddress `asn1:"explicit,optional,tag:9"`
	EncAuthData       types.EncryptedData `asn1:"explicit,optional,tag:10"`
	AdditionalTickets []Ticket            `asn1:"explicit,optional,tag:11"`
}

// NewASReq generates a new KRB_AS_REQ struct.
func NewASReq(realm string, c *config.Config, cname types.PrincipalName) (ASReq, error) {
	nonce, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return ASReq{}, err
	}
	t := time.Now().UTC()
	// Copy the default options to make this thread safe
	var kopts asn1.BitString
	copy(kopts.Bytes, c.LibDefaults.KDCDefaultOptions.Bytes)
	kopts.BitLength = c.LibDefaults.KDCDefaultOptions.BitLength
	a := ASReq{
		KDCReqFields{
			PVNO:    iana.PVNO,
			MsgType: msgtype.KRB_AS_REQ,
			PAData:  types.PADataSequence{},
			ReqBody: KDCReqBody{
				KDCOptions: kopts,
				Realm:      realm,
				CName:      cname,
				SName: types.PrincipalName{
					NameType:   nametype.KRB_NT_SRV_INST,
					NameString: []string{"krbtgt", realm},
				},
				Till:  t.Add(c.LibDefaults.TicketLifetime),
				Nonce: int(nonce.Int64()),
				EType: c.LibDefaults.DefaultTktEnctypeIDs,
			},
		},
	}
	if c.LibDefaults.Forwardable {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Forwardable)
	}
	if c.LibDefaults.Canonicalize {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Canonicalize)
	}
	if c.LibDefaults.Proxiable {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Proxiable)
	}
	if c.LibDefaults.RenewLifetime != 0 {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Renewable)
		a.ReqBody.RTime = t.Add(c.LibDefaults.RenewLifetime)
		a.ReqBody.RTime = t.Add(time.Duration(48) * time.Hour)

	}
	return a, nil
}

// NewTGSReq generates a new KRB_TGS_REQ struct.
func NewTGSReq(cname types.PrincipalName, kdcRealm string, c *config.Config, tkt Ticket, sessionKey types.EncryptionKey, spn types.PrincipalName, renewal bool) (TGSReq, error) {
	nonce, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		return TGSReq{}, err
	}
	t := time.Now().UTC()
	a := TGSReq{
		KDCReqFields{
			PVNO:    iana.PVNO,
			MsgType: msgtype.KRB_TGS_REQ,
			ReqBody: KDCReqBody{
				KDCOptions: types.NewKrbFlags(),
				Realm:      kdcRealm,
				SName:      spn,
				Till:       t.Add(c.LibDefaults.TicketLifetime),
				Nonce:      int(nonce.Int64()),
				EType:      c.LibDefaults.DefaultTGSEnctypeIDs,
			},
			Renewal: renewal,
		},
	}
	if c.LibDefaults.Forwardable {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Forwardable)
	}
	if c.LibDefaults.Canonicalize {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Canonicalize)
	}
	if c.LibDefaults.Proxiable {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Proxiable)
	}
	if c.LibDefaults.RenewLifetime > time.Duration(0) {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Renewable)
		a.ReqBody.RTime = t.Add(c.LibDefaults.RenewLifetime)
	}
	if renewal {
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Renew)
		types.SetFlag(&a.ReqBody.KDCOptions, flags.Renewable)
	}
	auth, err := types.NewAuthenticator(tkt.Realm, cname)
	if err != nil {
		return a, krberror.Errorf(err, krberror.KRBMsgError, "error generating new authenticator")
	}
	// Add the CName to make validation of the reply easier
	a.ReqBody.CName = auth.CName
	b, err := a.ReqBody.Marshal()
	if err != nil {
		return a, krberror.Errorf(err, krberror.EncodingError, "error marshaling TGS_REQ body")
	}
	etype, err := crypto.GetEtype(sessionKey.KeyType)
	if err != nil {
		return a, krberror.Errorf(err, krberror.EncryptingError, "error getting etype to encrypt authenticator")
	}
	cb, err := etype.GetChecksumHash(sessionKey.KeyValue, b, keyusage.TGS_REQ_PA_TGS_REQ_AP_REQ_AUTHENTICATOR_CHKSUM)
	if err != nil {
		return a, krberror.Errorf(err, krberror.ChksumError, "error getting etype checksum hash")
	}
	auth.Cksum = types.Checksum{
		CksumType: etype.GetHashID(),
		Checksum:  cb,
	}
	apReq, err := NewAPReq(tkt, sessionKey, auth)
	if err != nil {
		return a, krberror.Errorf(err, krberror.KRBMsgError, "error generating new AP_REQ")
	}
	apb, err := apReq.Marshal()
	if err != nil {
		return a, krberror.Errorf(err, krberror.EncodingError, "error marshaling AP_REQ for pre-authentication data")
	}
	a.PAData = types.PADataSequence{
		types.PAData{
			PADataType:  patype.PA_TGS_REQ,
			PADataValue: apb,
		},
	}
	return a, nil
}

// Unmarshal bytes b into the ASReq struct.
func (k *ASReq) Unmarshal(b []byte) error {
	var m marshalKDCReq
	_, err := asn1.UnmarshalWithParams(b, &m, fmt.Sprintf("application,explicit,tag:%v", asnAppTag.ASREQ))
	if err != nil {
		return krberror.Errorf(err, krberror.EncodingError, "Error unmarshaling AS_REQ")
	}
	expectedMsgType := msgtype.KRB_AS_REQ
	if m.MsgType != expectedMsgType {
		return krberror.NewErrorf(krberror.KRBMsgError, "Message ID does not indicate a AS_REQ. Expected: %v; Actual: %v", expectedMsgType, m.MsgType)
	}
	var reqb KDCReqBody
	err = reqb.Unmarshal(m.ReqBody.Bytes)
	if err != nil {
		return krberror.Errorf(err, krberror.EncodingError, "error processing AS_REQ body")
	}
	k.MsgType = m.MsgType
	k.PAData = m.PAData
	k.PVNO = m.PVNO
	k.ReqBody = reqb
	return nil
}

// Unmarshal bytes b into the TGSReq struct.
func (k *TGSReq) Unmarshal(b []byte) error {
	var m marshalKDCReq
	_, err := asn1.UnmarshalWithParams(b, &m, fmt.Sprintf("application,explicit,tag:%v", asnAppTag.TGSREQ))
	if err != nil {
		return krberror.Errorf(err, krberror.EncodingError, "Error unmarshaling TGS_REQ")
	}
	expectedMsgType := msgtype.KRB_TGS_REQ
	if m.MsgType != expectedMsgType {
		return krberror.NewErrorf(krberror.KRBMsgError, "Message ID does not indicate a TGS_REQ. Expected: %v; Actual: %v", expectedMsgType, m.MsgType)
	}
	var reqb KDCReqBody
	err = reqb.Unmarshal(m.ReqBody.Bytes)
	if err != nil {
		return krberror.Errorf(err, krberror.EncodingError, "Error processing TGS_REQ body")
	}
	k.MsgType = m.MsgType
	k.PAData = m.PAData
	k.PVNO = m.PVNO
	k.ReqBody = reqb
	return nil
}

// Unmarshal bytes b into the KRB_KDC_REQ body struct.
func (k *KDCReqBody) Unmarshal(b []byte) error {
	var m marshalKDCReqBody
	_, err := asn1.Unmarshal(b, &m)
	if err != nil {
		return krberror.Errorf(err, krberror.EncodingError, "Error unmarshaling KDC_REQ body")
	}
	k.KDCOptions = m.KDCOptions
	if len(k.KDCOptions.Bytes) < 4 {
		tb := make([]byte, 4-len(k.KDCOptions.Bytes))
		k.KDCOptions.Bytes = append(tb, k.KDCOptions.Bytes...)
		k.KDCOptions.BitLength = len(k.KDCOptions.Bytes) * 8
	}
	k.CName = m.CName
	k.Realm = m.Realm
	k.SName = m.SName
	k.From = m.From
	k.Till = m.Till
	k.RTime = m.RTime
	k.Nonce = m.Nonce
	k.EType = m.EType
	k.Addresses = m.Addresses
	k.EncAuthData = m.EncAuthData
	if len(m.AdditionalTickets.Bytes) > 0 {
		k.AdditionalTickets, err = UnmarshalTicketsSequence(m.AdditionalTickets)
		if err != nil {
			return krberror.Errorf(err, krberror.EncodingError, "Error unmarshaling additional tickets")
		}
	}
	return nil
}

// Marshal ASReq struct.
func (k *ASReq) Marshal() ([]byte, error) {
	m := marshalKDCReq{
		PVNO:    k.PVNO,
		MsgType: k.MsgType,
		PAData:  k.PAData,
	}
	b, err := k.ReqBody.Marshal()
	if err != nil {
		var mk []byte
		return mk, err
	}
	m.ReqBody = asn1.RawValue{
		Class:      asn1.ClassContextSpecific,
		IsCompound: true,
		Tag:        4,
		Bytes:      b,
	}
	mk, err := asn1.Marshal(m)
	if err != nil {
		return mk, krberror.Errorf(err, krberror.EncodingError, "Error marshaling AS_REQ")
	}
	mk = asn1tools.AddASNAppTag(mk, asnAppTag.ASREQ)
	return mk, nil
}

// Marshal TGSReq struct.
func (k *TGSReq) Marshal() ([]byte, error) {
	m := marshalKDCReq{
		PVNO:    k.PVNO,
		MsgType: k.MsgType,
		PAData:  k.PAData,
	}
	b, err := k.ReqBody.Marshal()
	if err != nil {
		var mk []byte
		return mk, err
	}
	m.ReqBody = asn1.RawValue{
		Class:      asn1.ClassContextSpecific,
		IsCompound: true,
		Tag:        4,
		Bytes:      b,
	}
	mk, err := asn1.Marshal(m)
	if err != nil {
		return mk, krberror.Errorf(err, krberror.EncodingError, "Error marshaling AS_REQ")
	}
	mk = asn1tools.AddASNAppTag(mk, asnAppTag.TGSREQ)
	return mk, nil
}

// Marshal KRB_KDC_REQ body struct.
func (k *KDCReqBody) Marshal() ([]byte, error) {
	var b []byte
	m := marshalKDCReqBody{
		KDCOptions:  k.KDCOptions,
		CName:       k.CName,
		Realm:       k.Realm,
		SName:       k.SName,
		From:        k.From,
		Till:        k.Till,
		RTime:       k.RTime,
		Nonce:       k.Nonce,
		EType:       k.EType,
		Addresses:   k.Addresses,
		EncAuthData: k.EncAuthData,
	}
	rawtkts, err := MarshalTicketSequence(k.AdditionalTickets)
	if err != nil {
		return b, krberror.Errorf(err, krberror.EncodingError, "Error in marshaling KDC request body additional tickets")
	}
	//The asn1.rawValue needs the tag setting on it for where it is in the KDCReqBody
	rawtkts.Tag = 11
	if len(rawtkts.Bytes) > 0 {
		m.AdditionalTickets = rawtkts
	}
	b, err = asn1.Marshal(m)
	if err != nil {
		return b, krberror.Errorf(err, krberror.EncodingError, "Error in marshaling KDC request body")
	}
	return b, nil
}
