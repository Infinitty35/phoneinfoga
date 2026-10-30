package suppliers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	// NumverifyAPITypeClassic authenticates numverify.com / apilayer.net access keys
	// against https://apilayer.net/api/validate (access_key query parameter).
	NumverifyAPITypeClassic = "classic"
	// NumverifyAPITypeApilayer authenticates APILayer Marketplace keys against
	// https://api.apilayer.com/number_verification/validate (Apikey header).
	NumverifyAPITypeApilayer = "apilayer"

	numverifyClassicEndpoint = "https://apilayer.net/api/validate"
)

type NumverifySupplierInterface interface {
	Request() NumverifySupplierRequestInterface
}

type NumverifySupplierRequestInterface interface {
	SetApiKey(string) NumverifySupplierRequestInterface
	SetApiType(string) NumverifySupplierRequestInterface
	ValidateNumber(string) (*NumverifyValidateResponse, error)
}

type NumverifyErrorResponse struct {
	Message string `json:"message"`
}

// NumverifyValidateResponse REST API response
type NumverifyValidateResponse struct {
	Valid               bool   `json:"valid"`
	Number              string `json:"number"`
	LocalFormat         string `json:"local_format"`
	InternationalFormat string `json:"international_format"`
	CountryPrefix       string `json:"country_prefix"`
	CountryCode         string `json:"country_code"`
	CountryName         string `json:"country_name"`
	Location            string `json:"location"`
	Carrier             string `json:"carrier"`
	LineType            string `json:"line_type"`
}

type NumverifySupplier struct {
	Uri string
}

func NewNumverifySupplier() *NumverifySupplier {
	return &NumverifySupplier{
		Uri: "https://api.apilayer.com",
	}
}

type NumverifyRequest struct {
	apiKey  string
	apiType string
	uri     string
}

// numverifyWireResponse covers both API payloads. Classic errors are HTTP 200
// with success=false; marketplace errors use an HTTP error status and message.
type numverifyWireResponse struct {
	Success             *bool                  `json:"success"`
	Error               *numverifyClassicError `json:"error"`
	Message             string                 `json:"message"`
	Valid               bool                   `json:"valid"`
	Number              string                 `json:"number"`
	LocalFormat         string                 `json:"local_format"`
	InternationalFormat string                 `json:"international_format"`
	CountryPrefix       string                 `json:"country_prefix"`
	CountryCode         string                 `json:"country_code"`
	CountryName         string                 `json:"country_name"`
	Location            string                 `json:"location"`
	Carrier             string                 `json:"carrier"`
	LineType            string                 `json:"line_type"`
}

type numverifyClassicError struct {
	Code int    `json:"code"`
	Type string `json:"type"`
	Info string `json:"info"`
}

func (s *NumverifySupplier) Request() NumverifySupplierRequestInterface {
	return &NumverifyRequest{uri: s.Uri}
}

func (r *NumverifyRequest) SetApiKey(k string) NumverifySupplierRequestInterface {
	r.apiKey = k
	return r
}

func (r *NumverifyRequest) SetApiType(t string) NumverifySupplierRequestInterface {
	r.apiType = t
	return r
}

// ParseNumverifyAPIType normalizes NUMVERIFY_API_TYPE.
// Empty defaults to classic. "marketplace" is an alias for apilayer.
func ParseNumverifyAPIType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", NumverifyAPITypeClassic:
		return NumverifyAPITypeClassic, nil
	case NumverifyAPITypeApilayer, "marketplace":
		return NumverifyAPITypeApilayer, nil
	default:
		return "", errors.New("unsupported NUMVERIFY_API_TYPE: expected classic or apilayer")
	}
}

func (r *NumverifyRequest) ValidateNumber(internationalNumber string) (*NumverifyValidateResponse, error) {
	apiType, err := ParseNumverifyAPIType(r.apiType)
	if err != nil {
		return nil, err
	}

	// Log the number and endpoint kind only. The classic URL embeds the access key.
	logrus.
		WithField("number", internationalNumber).
		WithField("api_type", apiType).
		Debug("Running validate operation through Numverify API")

	endpoint, headerKey, err := r.endpoint(apiType, internationalNumber)
	if err != nil {
		return nil, r.redact(err)
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, r.redact(err)
	}
	if headerKey {
		req.Header.Set("Apikey", r.apiKey)
	}

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return nil, r.redact(err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, r.redact(err)
	}

	res, err := decodeNumverifyBody(response.StatusCode, body)
	if err != nil {
		return nil, r.redact(err)
	}
	return res, nil
}

func (r *NumverifyRequest) endpoint(apiType, internationalNumber string) (string, bool, error) {
	switch apiType {
	case NumverifyAPITypeClassic:
		u, err := url.Parse(numverifyClassicEndpoint)
		if err != nil {
			return "", false, err
		}
		q := u.Query()
		q.Set("access_key", r.apiKey)
		q.Set("number", internationalNumber)
		u.RawQuery = q.Encode()
		return u.String(), false, nil
	case NumverifyAPITypeApilayer:
		return fmt.Sprintf("%s/number_verification/validate?number=%s", r.uri, internationalNumber), true, nil
	default:
		return "", false, errors.New("unsupported NUMVERIFY_API_TYPE: expected classic or apilayer")
	}
}

func (r *NumverifyRequest) redact(err error) error {
	return redactSecret(err, r.apiKey)
}

func redactSecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := err.Error()
	escaped := url.QueryEscape(secret)
	if escaped != "" && escaped != secret {
		msg = strings.ReplaceAll(msg, escaped, "REDACTED")
	}
	msg = strings.ReplaceAll(msg, secret, "REDACTED")
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func decodeNumverifyBody(status int, data []byte) (*NumverifyValidateResponse, error) {
	var wire numverifyWireResponse
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire.Success != nil && !*wire.Success {
		return nil, classicAPIError(wire.Error)
	}
	if status >= 400 {
		if wire.Message != "" {
			return nil, errors.New(wire.Message)
		}
		return nil, fmt.Errorf("numverify request failed with status %d", status)
	}
	return &NumverifyValidateResponse{
		Valid:               wire.Valid,
		Number:              wire.Number,
		LocalFormat:         wire.LocalFormat,
		InternationalFormat: wire.InternationalFormat,
		CountryPrefix:       wire.CountryPrefix,
		CountryCode:         wire.CountryCode,
		CountryName:         wire.CountryName,
		Location:            wire.Location,
		Carrier:             wire.Carrier,
		LineType:            wire.LineType,
	}, nil
}

func classicAPIError(e *numverifyClassicError) error {
	if e == nil || (e.Info == "" && e.Type == "" && e.Code == 0) {
		return errors.New("numverify request failed")
	}
	if e.Info != "" {
		return errors.New(e.Info)
	}
	if e.Type != "" {
		return fmt.Errorf("numverify request failed: %s (%d)", e.Type, e.Code)
	}
	return fmt.Errorf("numverify request failed (%d)", e.Code)
}
